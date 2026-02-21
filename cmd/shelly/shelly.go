// Shelly is an interactive terminal chat that connects to the Shelly engine.
// It loads a YAML configuration, creates a session with the configured entry
// agent, and runs a read-eval-print loop. While the agent processes a request
// the reasoning chain (tool calls, delegations, intermediate thinking) is
// streamed to the terminal in real time via the chat's Wait/Since API.
package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/germanamz/shelly/pkg/chats/chat"
	"github.com/germanamz/shelly/pkg/chats/content"
	"github.com/germanamz/shelly/pkg/chats/message"
	"github.com/germanamz/shelly/pkg/chats/role"
	"github.com/germanamz/shelly/pkg/engine"
)

// ANSI escape sequences for terminal formatting.
const (
	ansiReset  = "\033[0m"
	ansiBold   = "\033[1m"
	ansiDim    = "\033[2m"
	ansiGreen  = "\033[32m"
	ansiYellow = "\033[33m"
	ansiCyan   = "\033[36m"
	ansiRed    = "\033[31m"
)

func main() {
	configPath := flag.String("config", "shelly.yaml", "path to configuration file")
	agentName := flag.String("agent", "", "agent to start with (overrides entry_agent in config)")
	verbose := flag.Bool("verbose", false, "show tool arguments and results")
	flag.Parse()

	if err := run(*configPath, *agentName, *verbose); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

// run loads the engine from configPath, creates a session for agentName
// (or the configured entry agent), and enters the interactive chat loop.
func run(configPath, agentName string, verbose bool) error {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	cfg, err := engine.LoadConfig(configPath)
	if err != nil {
		return err
	}

	eng, err := engine.New(ctx, cfg)
	if err != nil {
		return err
	}
	defer func() { _ = eng.Close() }()

	sess, err := eng.NewSession(agentName)
	if err != nil {
		return err
	}

	fmt.Printf("%sshelly%s — interactive chat (session %s)\n", ansiBold, ansiReset, sess.ID())
	fmt.Printf("Type %s/help%s for commands, %s/quit%s to exit.\n\n", ansiDim, ansiReset, ansiDim, ansiReset)

	return chatLoop(ctx, sess, verbose)
}

// chatLoop reads user input line by line, dispatches it to the session, and
// streams the agent's reasoning chain until a final answer is produced.
// It handles /help and /quit commands, and exits cleanly on Ctrl+C or EOF.
func chatLoop(ctx context.Context, sess *engine.Session, verbose bool) error {
	scanner := bufio.NewScanner(os.Stdin)

	for {
		fmt.Printf("%syou>%s ", ansiGreen+ansiBold, ansiReset)

		if !scanner.Scan() {
			fmt.Println()
			return scanner.Err()
		}

		input := strings.TrimSpace(scanner.Text())
		if input == "" {
			continue
		}

		if input == "/quit" || input == "/exit" {
			fmt.Println("Goodbye!")
			return nil
		}

		if input == "/help" {
			printHelp()
			continue
		}

		if err := sendAndStream(ctx, sess, input, verbose); err != nil {
			if ctx.Err() != nil {
				fmt.Printf("\n%sInterrupted%s\n", ansiDim, ansiReset)
				return nil
			}
			fmt.Fprintf(os.Stderr, "%serror: %v%s\n", ansiRed, err, ansiReset)
		}

		fmt.Println()
	}
}

// sendAndStream sends a user message and streams the reasoning chain while
// the agent processes the request.
func sendAndStream(ctx context.Context, sess *engine.Session, input string, verbose bool) error {
	cursor := sess.Chat().Len()

	watchCtx, watchCancel := context.WithCancel(ctx)
	defer watchCancel()

	done := make(chan struct{})

	go func() {
		defer close(done)
		streamChat(watchCtx, sess.Chat(), cursor, verbose)
	}()

	_, err := sess.Send(ctx, input)
	watchCancel()
	<-done

	return err
}

// streamChat watches the chat for new messages and prints reasoning chain
// details as they appear. It exits when ctx is cancelled.
func streamChat(ctx context.Context, c *chat.Chat, cursor int, verbose bool) {
	for {
		if _, err := c.Wait(ctx, cursor); err != nil {
			return
		}

		for _, msg := range c.Since(cursor) {
			printMessage(msg, verbose)
			cursor++
		}
	}
}

// printMessage dispatches a single chat message to the appropriate renderer
// based on its role. System and user messages are skipped (already visible).
func printMessage(msg message.Message, verbose bool) {
	switch msg.Role {
	case role.System, role.User:
		return

	case role.Assistant:
		printAssistantMessage(msg, verbose)

	case role.Tool:
		if !verbose {
			return
		}
		printToolMessage(msg)
	}
}

// printAssistantMessage renders an assistant message. Messages with tool calls
// are displayed as reasoning steps (tool names, optional arguments). Messages
// without tool calls are treated as the agent's final answer.
func printAssistantMessage(msg message.Message, verbose bool) {
	calls := msg.ToolCalls()
	text := msg.TextContent()

	if len(calls) > 0 {
		if text != "" && verbose {
			fmt.Printf("  %s[thinking]%s %s\n", ansiDim, ansiReset, truncate(text, 200))
		}

		for _, tc := range calls {
			fmt.Printf("  %s[calling %s]%s", ansiYellow, tc.Name, ansiReset)
			if verbose && tc.Arguments != "" {
				fmt.Printf(" %s%s%s", ansiDim, truncate(tc.Arguments, 200), ansiReset)
			}
			fmt.Println()
		}

		return
	}

	if text != "" {
		fmt.Printf("\n%s%s>%s %s\n", ansiCyan+ansiBold, msg.Sender, ansiReset, text)
	}
}

// printToolMessage renders tool execution results. Errors are shown in red,
// successful results in dim text. Only displayed in verbose mode.
func printToolMessage(msg message.Message) {
	for _, p := range msg.Parts {
		tr, ok := p.(content.ToolResult)
		if !ok {
			continue
		}

		if tr.IsError {
			fmt.Printf("  %s[error] %s%s\n", ansiRed, truncate(tr.Content, 200), ansiReset)
		} else {
			fmt.Printf("  %s[result] %s%s\n", ansiDim, truncate(tr.Content, 200), ansiReset)
		}
	}
}

func printHelp() {
	fmt.Println("Commands:")
	fmt.Println("  /help   Show this help message")
	fmt.Println("  /quit   Exit the chat")
}

// truncate returns s shortened to at most n runes, with "..." appended if
// truncated. Newlines are replaced with spaces for single-line display.
func truncate(s string, n int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "..."
}
