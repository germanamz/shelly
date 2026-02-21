// Shelly is an interactive terminal chat that connects to the Shelly engine.
// It loads a YAML configuration, creates a session with the configured entry
// agent, and runs a read-eval-print loop. While the agent processes a request
// the reasoning chain (tool calls, delegations, intermediate thinking) is
// streamed to the terminal in real time via the chat's Wait/Since API.
package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"

	"github.com/joho/godotenv"

	"github.com/germanamz/shelly/pkg/chats/chat"
	"github.com/germanamz/shelly/pkg/chats/content"
	"github.com/germanamz/shelly/pkg/chats/message"
	"github.com/germanamz/shelly/pkg/chats/role"
	"github.com/germanamz/shelly/pkg/codingtoolbox/ask"
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
	envFile := flag.String("env", ".env", "path to .env file (ignored if missing)")
	agentName := flag.String("agent", "", "agent to start with (overrides entry_agent in config)")
	verbose := flag.Bool("verbose", false, "show tool arguments and results")
	flag.Parse()

	if err := loadDotEnv(*envFile); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	if err := run(*configPath, *agentName, *verbose); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

// loadDotEnv loads environment variables from path. If the file does not exist
// it is silently ignored so that .env files remain optional.
func loadDotEnv(path string) error {
	err := godotenv.Load(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
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

	return chatLoop(ctx, sess, eng.Events(), verbose)
}

// chatLoop reads user input line by line, dispatches it to the session, and
// streams the agent's reasoning chain until a final answer is produced.
// It handles /help and /quit commands, and exits cleanly on Ctrl+C or EOF.
func chatLoop(ctx context.Context, sess *engine.Session, events *engine.EventBus, verbose bool) error {
	reader := bufio.NewReader(os.Stdin)

	for {
		fmt.Printf("%syou>%s ", ansiGreen+ansiBold, ansiReset)

		line, err := reader.ReadString('\n')
		if err != nil {
			fmt.Println()
			if errors.Is(err, io.EOF) {
				return nil
			}
			return err
		}

		input := strings.TrimSpace(line)
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

		if err := sendAndStream(ctx, sess, events, input, verbose, reader); err != nil {
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
// the agent processes the request. It also subscribes to the event bus to
// handle ask_user prompts from permission-gated tools.
func sendAndStream(ctx context.Context, sess *engine.Session, events *engine.EventBus, input string, verbose bool, reader *bufio.Reader) error {
	cursor := sess.Chat().Len()

	watchCtx, watchCancel := context.WithCancel(ctx)
	defer watchCancel()

	chatDone := make(chan struct{})
	go func() {
		defer close(chatDone)
		streamChat(watchCtx, sess.Chat(), cursor, verbose)
	}()

	sub := events.Subscribe(64)
	askDone := make(chan struct{})
	go func() {
		defer close(askDone)
		handleAskEvents(watchCtx, sess, sub, reader)
	}()

	_, err := sess.Send(ctx, input)
	watchCancel()
	<-chatDone
	events.Unsubscribe(sub)
	<-askDone

	return err
}

// handleAskEvents watches for EventAskUser events and prompts the user for a
// response on the terminal. It runs until ctx is cancelled or the subscription
// channel is closed.
func handleAskEvents(ctx context.Context, sess *engine.Session, sub *engine.Subscription, reader *bufio.Reader) {
	for {
		select {
		case <-ctx.Done():
			return
		case ev, ok := <-sub.C:
			if !ok {
				return
			}
			if ev.Kind != engine.EventAskUser {
				continue
			}

			q, ok := ev.Data.(ask.Question)
			if !ok {
				continue
			}

			fmt.Printf("\n%s[question]%s %s\n", ansiYellow+ansiBold, ansiReset, q.Text)
			for i, opt := range q.Options {
				fmt.Printf("  %s%d)%s %s\n", ansiBold, i+1, ansiReset, opt)
			}
			fmt.Printf("%sanswer>%s ", ansiYellow+ansiBold, ansiReset)

			line, err := reader.ReadString('\n')
			if err != nil {
				return
			}

			response := strings.TrimSpace(line)

			// Map numeric input to the corresponding option.
			if len(q.Options) > 0 {
				if idx, parseErr := strconv.Atoi(response); parseErr == nil && idx >= 1 && idx <= len(q.Options) {
					response = q.Options[idx-1]
				}
			}

			if err := sess.Respond(q.ID, response); err != nil {
				fmt.Fprintf(os.Stderr, "%serror responding: %v%s\n", ansiRed, err, ansiReset)
			}
		}
	}
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
