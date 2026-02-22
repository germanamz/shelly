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
	"math/rand/v2"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/charmbracelet/glamour"
	"github.com/joho/godotenv"

	"github.com/germanamz/shelly/pkg/chats/chat"
	"github.com/germanamz/shelly/pkg/chats/content"
	"github.com/germanamz/shelly/pkg/chats/message"
	"github.com/germanamz/shelly/pkg/chats/role"
	"github.com/germanamz/shelly/pkg/codingtoolbox/ask"
	"github.com/germanamz/shelly/pkg/engine"
	"github.com/germanamz/shelly/pkg/modeladapter"
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

// thinkingMessages are displayed while the agent is processing.
var thinkingMessages = []string{
	"Thinking...",
	"Pondering the cosmos...",
	"Consulting ancient scrolls...",
	"Brewing a response...",
	"Connecting synapses...",
	"Mining for wisdom...",
	"Summoning knowledge...",
	"Assembling words...",
	"Crunching tokens...",
	"Weaving thoughts...",
	"Channeling creativity...",
	"Exploring possibilities...",
	"Decoding the matrix...",
	"Warming up neurons...",
	"Traversing the knowledge graph...",
}

// spinnerFrames are braille characters for smooth animation.
var spinnerFrames = []string{"⣾", "⣽", "⣻", "⢿", "⡿", "⣟", "⣯", "⣷"}

// mdRenderer renders markdown to terminal-formatted output.
var mdRenderer *glamour.TermRenderer

func initMarkdownRenderer() {
	r, err := glamour.NewTermRenderer(
		glamour.WithAutoStyle(),
		glamour.WithWordWrap(100),
	)
	if err != nil {
		return
	}
	mdRenderer = r
}

func main() {
	configPath := flag.String("config", "shelly.yaml", "path to configuration file")
	envFile := flag.String("env", ".env", "path to .env file (ignored if missing)")
	agentName := flag.String("agent", "", "agent to start with (overrides entry_agent in config)")
	verbose := flag.Bool("verbose", false, "show tool arguments and results")
	flag.Parse()

	initMarkdownRenderer()

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
// handle ask_user prompts from permission-gated tools. A spinner with random
// phrases is displayed while the agent is working.
func sendAndStream(ctx context.Context, sess *engine.Session, events *engine.EventBus, input string, verbose bool, reader *bufio.Reader) error {
	cursor := sess.Chat().Len()

	watchCtx, watchCancel := context.WithCancel(ctx)
	defer watchCancel()

	spin := newSpinner()

	chatDone := make(chan struct{})
	go func() {
		defer close(chatDone)
		streamChat(watchCtx, sess.Chat(), cursor, verbose, spin)
	}()

	sub := events.Subscribe(64)
	askDone := make(chan struct{})
	go func() {
		defer close(askDone)
		handleAskEvents(watchCtx, sess, sub, reader, spin)
	}()

	spin.Start()
	_, err := sess.Send(ctx, input)
	watchCancel()
	<-chatDone
	spin.Stop()
	events.Unsubscribe(sub)
	<-askDone

	printUsage(sess)

	return err
}

// handleAskEvents watches for EventAskUser events and prompts the user for a
// response on the terminal. It pauses the spinner during user interaction.
func handleAskEvents(ctx context.Context, sess *engine.Session, sub *engine.Subscription, reader *bufio.Reader, spin *spinner) {
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

			spin.Pause()

			fmt.Printf("\n%s[question]%s %s\n", ansiYellow+ansiBold, ansiReset, q.Text)
			for i, opt := range q.Options {
				fmt.Printf("  %s%d)%s %s\n", ansiBold, i+1, ansiReset, opt)
			}
			fmt.Printf("%sanswer>%s ", ansiYellow+ansiBold, ansiReset)

			line, err := reader.ReadString('\n')
			if err != nil {
				spin.Resume()
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

			spin.Resume()
		}
	}
}

// streamChat watches the chat for new messages and prints reasoning chain
// details as they appear. It pauses the spinner while printing messages.
func streamChat(ctx context.Context, c *chat.Chat, cursor int, verbose bool, spin *spinner) {
	for {
		if _, err := c.Wait(ctx, cursor); err != nil {
			return
		}

		msgs := c.Since(cursor)
		if len(msgs) > 0 {
			spin.Pause()
			for _, msg := range msgs {
				printMessage(msg, verbose)
				cursor++
			}
			spin.Resume()
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
// without tool calls are treated as the agent's final answer and rendered with
// markdown formatting.
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
		fmt.Printf("\n%s%s>%s %s\n", ansiCyan+ansiBold, msg.Sender, ansiReset, renderMarkdown(text))
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

// printUsage displays token usage information after each agent interaction.
func printUsage(sess *engine.Session) {
	ur, ok := sess.Completer().(modeladapter.UsageReporter)
	if !ok {
		return
	}

	last, hasLast := ur.UsageTracker().Last()
	if !hasLast {
		return
	}

	total := ur.UsageTracker().Total()
	maxTok := ur.ModelMaxTokens()

	fmt.Printf("  %s[context: %s · limit: %s · total: ↑%s ↓%s]%s\n",
		ansiDim,
		fmtTokens(last.InputTokens),
		fmtTokens(maxTok),
		fmtTokens(total.InputTokens),
		fmtTokens(total.OutputTokens),
		ansiReset,
	)
}

func printHelp() {
	fmt.Println("Commands:")
	fmt.Println("  /help   Show this help message")
	fmt.Println("  /quit   Exit the chat")
}

// renderMarkdown converts markdown text to terminal-formatted output using
// glamour. Falls back to plain text if the renderer is unavailable.
func renderMarkdown(text string) string {
	if mdRenderer == nil {
		return text
	}
	out, err := mdRenderer.Render(text)
	if err != nil {
		return text
	}
	return strings.TrimRight(out, "\n")
}

// fmtTokens formats a token count for display, using k/M suffixes for
// readability.
func fmtTokens(n int) string {
	switch {
	case n >= 1_000_000:
		return fmt.Sprintf("%.1fM", float64(n)/1_000_000)
	case n >= 1_000:
		return fmt.Sprintf("%.1fk", float64(n)/1_000)
	default:
		return fmt.Sprintf("%d", n)
	}
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

// --- spinner ---

// spinner displays an animated indicator with rotating phrases while the agent
// is working. It is safe for concurrent use. Pause/Resume calls are
// reference-counted so multiple goroutines can pause independently.
type spinner struct {
	mu         sync.Mutex
	pauseCount int
	stopCh     chan struct{}
	doneCh     chan struct{}
}

func newSpinner() *spinner {
	return &spinner{}
}

// Start begins the spinner animation in a background goroutine.
func (s *spinner) Start() {
	s.mu.Lock()
	s.pauseCount = 0
	s.stopCh = make(chan struct{})
	s.doneCh = make(chan struct{})
	s.mu.Unlock()

	go s.run()
}

func (s *spinner) run() {
	defer close(s.doneCh)

	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	frame := 0
	msgIdx := rand.IntN(len(thinkingMessages)) //nolint:gosec // cosmetic randomness for spinner messages
	changeTick := 0

	for {
		select {
		case <-s.stopCh:
			s.clearLine()
			return
		case <-ticker.C:
			s.mu.Lock()
			paused := s.pauseCount > 0
			s.mu.Unlock()

			if paused {
				continue
			}

			f := spinnerFrames[frame%len(spinnerFrames)]
			msg := thinkingMessages[msgIdx]
			fmt.Printf("\r  %s%s %s%s\033[K", ansiDim, f, msg, ansiReset)

			frame++
			changeTick++
			if changeTick >= 30 { // change message every ~3 seconds
				msgIdx = (msgIdx + 1) % len(thinkingMessages)
				changeTick = 0
			}
		}
	}
}

// Pause temporarily hides the spinner and clears its line. Each call
// increments a counter; the spinner only resumes when all pausers have
// called Resume.
func (s *spinner) Pause() {
	s.mu.Lock()
	s.pauseCount++
	s.mu.Unlock()
	s.clearLine()
}

// Resume decrements the pause counter. The spinner animation only restarts
// once all outstanding Pause calls have been balanced by Resume calls.
func (s *spinner) Resume() {
	s.mu.Lock()
	if s.pauseCount > 0 {
		s.pauseCount--
	}
	s.mu.Unlock()
}

// Stop terminates the spinner goroutine and clears the line.
func (s *spinner) Stop() {
	select {
	case <-s.stopCh:
		return // already stopped
	default:
	}
	close(s.stopCh)
	<-s.doneCh
}

func (s *spinner) clearLine() {
	fmt.Print("\r\033[K")
}
