// Shelly is an interactive terminal chat that connects to the Shelly engine.
// It loads a YAML configuration, creates a session with the configured entry
// agent, and runs a read-eval-print loop. While the agent processes a request
// the reasoning chain (tool calls, delegations, intermediate thinking) is
// streamed to the terminal in real time via the chat's Wait/Since API.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"math/rand/v2"
	"os"
	"os/signal"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
	"unicode"

	"github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
	"github.com/joho/godotenv"
	"golang.org/x/term"

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

// termProtocol identifies which keyboard protocol the terminal supports for
// detecting modified keys like Shift+Enter.
type termProtocol int32

const (
	// protocolNone means no extended keyboard protocol is available.
	// Only Alt+Enter works for inserting newlines.
	protocolNone termProtocol = iota
	// protocolKitty is the kitty keyboard protocol (CSI u encoding).
	// Shift+Enter is reported as ESC[13;2u.
	protocolKitty
	// protocolModifyOtherKeys is xterm's modifyOtherKeys mode.
	// Shift+Enter is reported as ESC[27;2;13~.
	protocolModifyOtherKeys
)

// cachedProtocol stores the detected terminal protocol. Resolved once on first
// call to getTermProtocol and reused for the rest of the process.
var cachedProtocol atomic.Int32

func init() {
	// Sentinel value -1 means "not yet detected".
	cachedProtocol.Store(-1)
}

// getTermProtocol returns the cached terminal protocol, detecting it on first call.
func getTermProtocol() termProtocol {
	if v := cachedProtocol.Load(); v >= 0 {
		return termProtocol(v)
	}
	p := detectTermProtocol()
	cachedProtocol.Store(int32(p))
	return p
}

// detectTermProtocol identifies which keyboard protocol the terminal supports
// by inspecting environment variables set by known terminal emulators.
func detectTermProtocol() termProtocol {
	// Terminal-specific env vars that indicate kitty protocol support.
	if os.Getenv("KITTY_WINDOW_ID") != "" {
		return protocolKitty
	}
	if os.Getenv("WEZTERM_PANE") != "" {
		return protocolKitty
	}
	if os.Getenv("GHOSTTY_RESOURCES_DIR") != "" {
		return protocolKitty
	}

	// TERM_PROGRAM is set by most modern terminals.
	switch strings.ToLower(os.Getenv("TERM_PROGRAM")) {
	case "kitty", "wezterm", "ghostty", "foot", "rio":
		return protocolKitty
	case "iterm.app", "iterm2.app":
		return protocolModifyOtherKeys
	case "vscode":
		return protocolModifyOtherKeys
	case "apple_terminal":
		return protocolNone
	}

	// tmux supports modifyOtherKeys passthrough.
	if os.Getenv("TMUX") != "" {
		return protocolModifyOtherKeys
	}

	// xterm-compatible terminals generally support modifyOtherKeys.
	t := os.Getenv("TERM")
	if strings.HasPrefix(t, "xterm") {
		return protocolModifyOtherKeys
	}

	return protocolNone
}

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

// moveCursorWordLeft moves the cursor to the start of the previous word.
func moveCursorWordLeft(line []rune, cursor int) int {
	// Skip trailing spaces
	for cursor > 0 && unicode.IsSpace(line[cursor-1]) {
		cursor--
	}
	// Skip word
	for cursor > 0 && !unicode.IsSpace(line[cursor-1]) {
		cursor--
	}
	return cursor
}

// moveCursorWordRight moves the cursor to the start of the next word.
func moveCursorWordRight(line []rune, cursor int) int {
	lineLen := len(line)
	if cursor < lineLen && !unicode.IsSpace(line[cursor]) {
		// Skip to end of current word
		for cursor < lineLen && !unicode.IsSpace(line[cursor]) {
			cursor++
		}
	}
	// Skip spaces
	for cursor < lineLen && unicode.IsSpace(line[cursor]) {
		cursor++
	}
	return cursor
}

// deleteWordBackward deletes the word backward from the cursor.
func deleteWordBackward(line []rune, cursor int) ([]rune, int) {
	newCursor := moveCursorWordLeft(line, cursor)
	return append(line[:newCursor], line[cursor:]...), newCursor
}

func main() {
	configPath := flag.String("config", "shelly.yaml", "path to configuration file")
	envFile := flag.String("env", ".env", "path to .env file (ignored if missing)")
	agentName := flag.String("agent", "", "agent to start with (overrides entry_agent in config)")
	verbose := flag.Bool("verbose", false, "show tool results and thinking text")
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
	for {
		prompt := fmt.Sprintf("%syou>%s ", ansiGreen+ansiBold, ansiReset)
		line, err := readLine(prompt)
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

		// Display the total number of tokens used just below the user input
		if ur, ok := sess.Completer().(modeladapter.UsageReporter); ok {
			total := ur.UsageTracker().Total()
			fmt.Printf("Total tokens used: %d\n", total.InputTokens+total.OutputTokens)
		}

		if err := sendAndStream(ctx, sess, events, input, verbose); err != nil {
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
func sendAndStream(ctx context.Context, sess *engine.Session, events *engine.EventBus, input string, verbose bool) error {
	cursor := sess.Chat().Len()

	watchCtx, watchCancel := context.WithCancel(ctx)
	defer watchCancel()

	spin := newSpinner()

	// termMu coordinates terminal I/O between streamChat and handleAskEvents
	// so that tool output never interleaves with the answer prompt.
	var termMu sync.Mutex

	chatDone := make(chan struct{})
	go func() {
		defer close(chatDone)
		streamChat(watchCtx, sess.Chat(), cursor, verbose, spin, &termMu)
	}()

	sub := events.Subscribe(64)
	askDone := make(chan struct{})
	go func() {
		defer close(askDone)
		handleAskEvents(watchCtx, sess, sub, spin, &termMu)
	}()

	start := time.Now()
	spin.Start()
	_, err := sess.Send(ctx, input)
	duration := time.Since(start)
	watchCancel()
	<-chatDone
	spin.Stop()
	events.Unsubscribe(sub)
	<-askDone

	printUsage(sess, duration)

	return err
}

// handleAskEvents watches for EventAskUser events and prompts the user for a
// response on the terminal. It pauses the spinner during user interaction and
// holds termMu to prevent streamChat from interleaving output with the prompt.
func handleAskEvents(ctx context.Context, sess *engine.Session, sub *engine.Subscription, spin *spinner, termMu *sync.Mutex) {
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

			// Hold termMu for the entire question-answer interaction so that
			// streamChat cannot print tool output after the "answer>" prompt.
			termMu.Lock()
			spin.Pause()

			response := ""
			if len(q.Options) == 0 {
				// Free-form question
				fmt.Printf("\n%s[question]%s %s\n", ansiYellow+ansiBold, ansiReset, q.Text)
				prompt := fmt.Sprintf("%sanswer>%s ", ansiYellow+ansiBold, ansiReset)
				line, err := readLine(prompt)
				if err != nil {
					spin.Resume()
					termMu.Unlock()
					return
				}
				response = strings.TrimSpace(line)
			} else {
				// Multiple choice with selection
				options := make([]string, len(q.Options)+1)
				copy(options, q.Options)
				options[len(q.Options)] = "Other (custom input)"

				p := tea.NewProgram(askModel{question: q.Text, options: options})
				model, err := p.Run()
				if err != nil {
					fmt.Fprintf(os.Stderr, "%serror: %v%s\n", ansiRed, err, ansiReset)
					spin.Resume()
					termMu.Unlock()
					continue
				}

				am := model.(askModel)
				if am.choice == "Other (custom input)" {
					prompt := fmt.Sprintf("%s[custom input]%s ", ansiYellow+ansiBold, ansiReset)
					line, err := readLine(prompt)
					if err != nil {
						spin.Resume()
						termMu.Unlock()
						return
					}
					response = strings.TrimSpace(line)
				} else {
					response = am.choice
				}
			}

			if err := sess.Respond(q.ID, response); err != nil {
				fmt.Fprintf(os.Stderr, "%serror responding: %v%s\n", ansiRed, err, ansiReset)
			}

			spin.Resume()
			termMu.Unlock()
		}
	}
}

// streamChat watches the chat for new messages and prints reasoning chain
// details as they appear. It pauses the spinner while printing messages and
// holds termMu to avoid interleaving with the ask prompt.
func streamChat(ctx context.Context, c *chat.Chat, cursor int, verbose bool, spin *spinner, termMu *sync.Mutex) {
	for {
		_, err := c.Wait(ctx, cursor)

		// Always drain pending messages, even when the context is cancelled.
		// This prevents a race where watchCancel() fires at the same time as
		// a new message signal, causing Wait to pick ctx.Done and return
		// before the final agent response is printed.
		msgs := c.Since(cursor)
		if len(msgs) > 0 {
			termMu.Lock()
			spin.Pause()
			for _, msg := range msgs {
				printMessage(msg, verbose)
				cursor++
			}
			spin.Resume()
			termMu.Unlock()
		}

		if err != nil {
			return
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
// are displayed as reasoning steps (tool names and arguments). Messages
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
			if tc.Arguments != "" {
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

// printUsage displays token usage and timing information after each agent interaction.
func printUsage(sess *engine.Session, duration time.Duration) {
	ur, ok := sess.Completer().(modeladapter.UsageReporter)
	if !ok {
		fmt.Printf("  %s[%s]%s\n", ansiDim, fmtDuration(duration), ansiReset)
		return
	}

	last, hasLast := ur.UsageTracker().Last()
	total := ur.UsageTracker().Total()
	maxTok := ur.ModelMaxTokens()

	if hasLast {
		fmt.Printf("  %s[last: ↑%s ↓%s · total: ↑%s ↓%s · limit: %s · %s]%s\n",
			ansiDim,
			fmtTokens(last.InputTokens),
			fmtTokens(last.OutputTokens),
			fmtTokens(total.InputTokens),
			fmtTokens(total.OutputTokens),
			fmtTokens(maxTok),
			fmtDuration(duration),
			ansiReset,
		)
	} else {
		fmt.Printf("  %s[total: ↑%s ↓%s · limit: %s · %s]%s\n",
			ansiDim,
			fmtTokens(total.InputTokens),
			fmtTokens(total.OutputTokens),
			fmtTokens(maxTok),
			fmtDuration(duration),
			ansiReset,
		)
	}
}

func printHelp() {
	fmt.Println("Commands:")
	fmt.Println("  /help          Show this help message")
	fmt.Println("  /quit          Exit the chat")
	fmt.Println()
	fmt.Println("Shortcuts:")
	fmt.Println("  Enter          Submit message")
	switch getTermProtocol() {
	case protocolKitty, protocolModifyOtherKeys:
		fmt.Println("  Shift+Enter    New line")
	default:
		fmt.Println("  Shift+Enter    New line (not available — terminal lacks keyboard protocol)")
	}
	fmt.Println("  Alt+Enter      New line (works in all terminals)")
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

// fmtDuration formats a duration for display, showing seconds or minutes:seconds.
func fmtDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%.1fs", d.Seconds())
	}
	min := int(d.Minutes())
	sec := int(d.Seconds()) % 60
	return fmt.Sprintf("%dm %ds", min, sec)
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

// visibleWidth returns the number of visible columns in s, ignoring ANSI escape sequences.
func visibleWidth(s string) int {
	w := 0
	inEsc := false
	for _, r := range s {
		if r == '\033' {
			inEsc = true
			continue
		}
		if inEsc {
			if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
				inEsc = false
			}
			continue
		}
		w++
	}
	return w
}

// insertNewline splits the current line at the cursor and inserts a new line.
func insertNewline(lines [][]rune, row, col int) ([][]rune, int, int) {
	rest := make([]rune, len(lines[row][col:]))
	copy(rest, lines[row][col:])
	lines[row] = lines[row][:col]
	newLines := make([][]rune, len(lines)+1)
	copy(newLines, lines[:row+1])
	newLines[row+1] = rest
	copy(newLines[row+2:], lines[row+1:])
	return newLines, row + 1, 0
}

// readLine reads user input with support for multi-line editing (Shift+Enter),
// cursor movement, word jump, and word delete.
// It uses raw terminal mode to handle individual key presses.
// Shift+Enter inserts a newline; Enter submits the input.
// Supports both kitty keyboard protocol and xterm modifyOtherKeys for Shift+Enter.
// Alt+Enter works as a universal fallback in all terminals.
//
//nolint:gocyclo
func readLine(prompt string) (string, error) {
	oldState, err := term.MakeRaw(int(os.Stdin.Fd())) //nolint:gosec
	if err != nil {
		return "", err
	}
	defer func() {
		_ = term.Restore(int(os.Stdin.Fd()), oldState) //nolint:gosec
	}()

	// Enable keyboard protocol for Shift+Enter based on detected terminal capabilities.
	proto := getTermProtocol()
	switch proto {
	case protocolKitty:
		fmt.Print("\033[>1u")
		defer fmt.Print("\033[<u")
	case protocolModifyOtherKeys:
		fmt.Print("\033[>4;2m")
		defer fmt.Print("\033[>4;0m")
	}

	promptWidth := visibleWidth(prompt)
	cont := strings.Repeat(" ", promptWidth)

	fmt.Print(prompt)
	lines := [][]rune{{}}
	row := 0
	col := 0
	displayLines := 1
	for {
		var buf [1]byte
		n, err := os.Stdin.Read(buf[:])
		if err != nil {
			return "", err
		}
		if n == 0 {
			continue
		}
		b := buf[0]
		if b == 3 { // Ctrl+C
			return "", io.EOF
		}
		if b == 13 || b == 10 { // Enter (CR or LF) — submit
			// Move cursor to end of last line for clean output
			if row < len(lines)-1 {
				fmt.Printf("\033[%dB", len(lines)-1-row)
			}
			fmt.Println()
			parts := make([]string, len(lines))
			for i, l := range lines {
				parts[i] = string(l)
			}
			return strings.Join(parts, "\n"), nil
		}
		if b == 127 || b == 8 { // Backspace
			if col > 0 {
				lines[row] = append(lines[row][:col-1], lines[row][col:]...)
				col--
			} else if row > 0 {
				// Join current line with previous line
				prevLen := len(lines[row-1])
				lines[row-1] = append(lines[row-1], lines[row]...)
				lines = append(lines[:row], lines[row+1:]...)
				row--
				col = prevLen
			}
			displayLines = redrawLines(prompt, cont, lines, row, col, displayLines)
			continue
		}
		if b == 23 { // Ctrl+W (delete word backward)
			if col > 0 {
				lines[row], col = deleteWordBackward(lines[row], col)
			} else if row > 0 {
				// Join with previous line
				prevLen := len(lines[row-1])
				lines[row-1] = append(lines[row-1], lines[row]...)
				lines = append(lines[:row], lines[row+1:]...)
				row--
				col = prevLen
			}
			displayLines = redrawLines(prompt, cont, lines, row, col, displayLines)
			continue
		}
		if b == 27 { // Escape sequence
			n2, err2 := os.Stdin.Read(buf[:])
			if err2 != nil {
				return "", err2
			}
			if n2 == 0 {
				continue
			}
			b2 := buf[0]
			switch b2 {
			case 91: // [
				var seq []byte
				for {
					n3, err3 := os.Stdin.Read(buf[:])
					if err3 != nil {
						return "", err3
					}
					if n3 == 0 {
						continue
					}
					b3 := buf[0]
					if b3 >= 64 && b3 <= 126 { // Final character
						seq = append(seq, b3)
						break
					} else {
						seq = append(seq, b3)
					}
				}
				final := seq[len(seq)-1]
				seq = seq[:len(seq)-1]
				paramStr := string(seq)

				// Shift+Enter — insert newline
				// Kitty keyboard protocol: ESC[13;2u
				// xterm modifyOtherKeys: ESC[27;2;13~
				if (final == 'u' && paramStr == "13;2") || (final == '~' && paramStr == "27;2;13") {
					lines, row, col = insertNewline(lines, row, col)
					displayLines = redrawLines(prompt, cont, lines, row, col, displayLines)
					continue
				}

				switch paramStr {
				case "1;5":
					switch final {
					case 'D': // Ctrl+Left
						col = moveCursorWordLeft(lines[row], col)
					case 'C': // Ctrl+Right
						col = moveCursorWordRight(lines[row], col)
					}
				case "1;3":
					switch final {
					case 'D': // Alt+Left
						col = moveCursorWordLeft(lines[row], col)
					case 'C': // Alt+Right
						col = moveCursorWordRight(lines[row], col)
					}
				case "":
					switch final {
					case 'D': // Left arrow
						if col > 0 {
							col--
						} else if row > 0 {
							row--
							col = len(lines[row])
						}
					case 'C': // Right arrow
						if col < len(lines[row]) {
							col++
						} else if row < len(lines)-1 {
							row++
							col = 0
						}
					case 'A': // Up arrow
						if row > 0 {
							row--
							if col > len(lines[row]) {
								col = len(lines[row])
							}
						}
					case 'B': // Down arrow
						if row < len(lines)-1 {
							row++
							if col > len(lines[row]) {
								col = len(lines[row])
							}
						}
					}
				}
				displayLines = redrawLines(prompt, cont, lines, row, col, displayLines)
			case 13, 10: // Alt+Enter — insert newline (works in all terminals)
				lines, row, col = insertNewline(lines, row, col)
				displayLines = redrawLines(prompt, cont, lines, row, col, displayLines)
			case 'b': // Alt+Left (ESC b — word left, macOS Terminal)
				col = moveCursorWordLeft(lines[row], col)
				displayLines = redrawLines(prompt, cont, lines, row, col, displayLines)
			case 'f': // Alt+Right (ESC f — word right, macOS Terminal)
				col = moveCursorWordRight(lines[row], col)
				displayLines = redrawLines(prompt, cont, lines, row, col, displayLines)
			case 127: // Alt+Backspace (delete word backward)
				if col > 0 {
					lines[row], col = deleteWordBackward(lines[row], col)
				} else if row > 0 {
					prevLen := len(lines[row-1])
					lines[row-1] = append(lines[row-1], lines[row]...)
					lines = append(lines[:row], lines[row+1:]...)
					row--
					col = prevLen
				}
				displayLines = redrawLines(prompt, cont, lines, row, col, displayLines)
			}
			continue
		}
		if b >= 32 && b <= 126 { // Printable ASCII
			lines[row] = append(lines[row][:col], append([]rune{rune(b)}, lines[row][col:]...)...)
			col++
			displayLines = redrawLines(prompt, cont, lines, row, col, displayLines)
		}
	}
}

// redrawLines clears the displayed input area and redraws all lines with the cursor positioned.
// Returns the number of display lines rendered (for the next redraw).
func redrawLines(prompt, cont string, lines [][]rune, row, col, prevDisplayLines int) int {
	// Move cursor up to the first display line
	if prevDisplayLines > 1 {
		fmt.Printf("\033[%dA", prevDisplayLines-1)
	}
	// Clear from start of first line to end of screen
	fmt.Print("\r\033[J")

	// Render all lines
	for i, line := range lines {
		if i == 0 {
			fmt.Printf("%s%s", prompt, string(line))
		} else {
			fmt.Printf("\n%s%s", cont, string(line))
		}
	}

	// Position cursor at (row, col)
	lastRow := len(lines) - 1
	if row < lastRow {
		fmt.Printf("\033[%dA", lastRow-row)
	}
	var targetCol int
	if row == 0 {
		targetCol = visibleWidth(prompt) + col
	} else {
		targetCol = len(cont) + col
	}
	fmt.Print("\r")
	if targetCol > 0 {
		fmt.Printf("\033[%dC", targetCol)
	}
	return len(lines)
}

// --- ask model for interactive selection ---

type askModel struct {
	question string
	options  []string
	cursor   int
	selected bool
	choice   string
}

func (m askModel) Init() tea.Cmd { return nil }

func (m askModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if keyMsg, ok := msg.(tea.KeyMsg); ok {
		switch keyMsg.String() {
		case "up":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down":
			if m.cursor < len(m.options)-1 {
				m.cursor++
			}
		case "enter":
			m.selected = true
			m.choice = m.options[m.cursor]
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m askModel) View() string {
	var sb strings.Builder
	sb.WriteString(m.question)
	sb.WriteString("\n\n")
	for i, opt := range m.options {
		cursor := " "
		if m.cursor == i {
			cursor = ">"
		}
		sb.WriteString(cursor)
		sb.WriteString(" ")
		sb.WriteString(opt)
		sb.WriteString("\n")
	}
	sb.WriteString("\n")
	return sb.String()
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
