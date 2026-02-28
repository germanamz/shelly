// Package exec provides a tool that gives agents the ability to run CLI
// commands. Every command execution is gated by explicit user permission.
// Users can "trust" a command (program name) to allow it for all future
// invocations without being prompted again. Trusted commands are persisted
// to the shared permissions file.
package exec

import (
	"context"
	"encoding/json"
	"fmt"
	osexec "os/exec"
	"strings"
	"sync"

	"github.com/germanamz/shelly/pkg/codingtoolbox/permissions"
	"github.com/germanamz/shelly/pkg/tools/toolbox"
)

// maxBufferSize is the maximum number of bytes captured from stdout/stderr (1MB).
const maxBufferSize = 1 << 20

// limitedBuffer is a bytes.Buffer that silently discards writes beyond maxBufferSize.
type limitedBuffer struct {
	buf []byte
}

func (b *limitedBuffer) Write(p []byte) (int, error) {
	remaining := maxBufferSize - len(b.buf)
	if remaining > 0 {
		if len(p) > remaining {
			b.buf = append(b.buf, p[:remaining]...)
		} else {
			b.buf = append(b.buf, p...)
		}
	}

	return len(p), nil
}

func (b *limitedBuffer) Len() int       { return len(b.buf) }
func (b *limitedBuffer) String() string { return string(b.buf) }

// AskFunc asks the user a question and blocks until a response is received.
type AskFunc func(ctx context.Context, question string, options []string) (string, error)

// OnExecFunc is called when a trusted command is about to execute, giving the
// frontend an opportunity to display what is being run without blocking.
type OnExecFunc func(ctx context.Context, display string)

// pendingResult holds the outcome of a single in-flight permission prompt so
// that concurrent callers waiting on the same command can share the result.
type pendingResult struct {
	done    chan struct{}
	err     error
	trusted bool // true when the user chose "trust" (safe to coalesce); false for one-time "yes".
}

// Exec provides command execution tools with permission gating.
type Exec struct {
	store      *permissions.Store
	ask        AskFunc
	onExec     OnExecFunc
	pendingMu  sync.Mutex
	pendingCmd map[string]*pendingResult
}

// New creates an Exec that checks the given permissions store for trusted
// commands and prompts the user via askFn when a command is not yet trusted.
// The optional onExec callback is called for trusted commands so the frontend
// can display what is being executed.
func New(store *permissions.Store, askFn AskFunc, opts ...Option) *Exec {
	e := &Exec{store: store, ask: askFn, pendingCmd: make(map[string]*pendingResult)}
	for _, opt := range opts {
		opt(e)
	}

	return e
}

// Option configures optional Exec behaviour.
type Option func(*Exec)

// WithOnExec sets a callback that is invoked when a trusted command is about
// to execute, allowing the frontend to display the full command line.
func WithOnExec(fn OnExecFunc) Option {
	return func(e *Exec) {
		e.onExec = fn
	}
}

// Tools returns a ToolBox containing the exec tools.
func (e *Exec) Tools() *toolbox.ToolBox {
	tb := toolbox.New()
	tb.Register(e.runTool())

	return tb
}

type runInput struct {
	Command string   `json:"command"`
	Args    []string `json:"args"`
}

func (e *Exec) runTool() toolbox.Tool {
	return toolbox.Tool{
		Name:        "exec_run",
		Description: "Run a program or CLI command. The user will be asked for permission before execution. They can choose to trust the command for all future calls. Use for arbitrary shell commands. For git operations, prefer the dedicated git tools (git_status, git_diff, git_log, git_commit) which provide structured output.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"command":{"type":"string","description":"The program or command to run (e.g. git, ls, npm)"},"args":{"type":"array","items":{"type":"string"},"description":"Arguments to pass to the command"}},"required":["command"]}`),
		Handler:     e.handleRun,
	}
}

func (e *Exec) handleRun(ctx context.Context, input json.RawMessage) (string, error) {
	var in runInput
	if err := json.Unmarshal(input, &in); err != nil {
		return "", fmt.Errorf("exec_run: invalid input: %w", err)
	}

	if in.Command == "" {
		return "", fmt.Errorf("exec_run: command is required")
	}

	if err := e.checkPermission(ctx, in.Command, in.Args); err != nil {
		return "", err
	}

	cmd := osexec.CommandContext(ctx, in.Command, in.Args...) //nolint:gosec // command is approved by user

	var stdout, stderr limitedBuffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()

	var result strings.Builder
	if stdout.Len() > 0 {
		result.WriteString(stdout.String())
	}

	if stderr.Len() > 0 {
		if result.Len() > 0 {
			result.WriteString("\n")
		}

		result.WriteString(stderr.String())
	}

	if err != nil {
		return "", fmt.Errorf("exec_run: %w\n%s", err, result.String())
	}

	return result.String(), nil
}

func (e *Exec) checkPermission(ctx context.Context, command string, args []string) error {
	display := command
	if len(args) > 0 {
		display += " " + strings.Join(args, " ")
	}

	// Fast path: already trusted (no lock contention).
	if e.store.IsCommandTrusted(command) {
		if e.onExec != nil {
			e.onExec(ctx, display)
		}

		return nil
	}

	e.pendingMu.Lock()
	// Re-check after acquiring lock.
	if e.store.IsCommandTrusted(command) {
		e.pendingMu.Unlock()
		if e.onExec != nil {
			e.onExec(ctx, display)
		}

		return nil
	}

	// If a prompt is already in-flight for this command, wait for its result.
	if pr, ok := e.pendingCmd[command]; ok {
		e.pendingMu.Unlock()
		<-pr.done

		if pr.err != nil {
			return pr.err
		}

		// Only coalesce if the user chose "trust". A one-time "yes"
		// approved specific arguments that may differ from ours, so we
		// must get our own prompt.
		if !pr.trusted {
			return e.promptPermission(ctx, command, display)
		}

		if e.onExec != nil {
			e.onExec(ctx, display)
		}

		return nil
	}

	// We are the first — create a pending entry and release the lock.
	pr := &pendingResult{done: make(chan struct{})}
	e.pendingCmd[command] = pr
	e.pendingMu.Unlock()

	// Ask the user (blocking).
	pr.trusted, pr.err = e.askAndApproveCmd(ctx, command, display)

	// Signal waiters and clean up.
	close(pr.done)
	e.pendingMu.Lock()
	delete(e.pendingCmd, command)
	e.pendingMu.Unlock()

	return pr.err
}

// promptPermission asks the user for permission without coalescing.
func (e *Exec) promptPermission(ctx context.Context, command, display string) error {
	trusted, err := e.askAndApproveCmd(ctx, command, display)
	if err != nil {
		return err
	}

	if trusted {
		if e.onExec != nil {
			e.onExec(ctx, display)
		}
	}

	return nil
}

// askAndApproveCmd prompts the user and trusts/approves the command.
// Returns (true, nil) for trust, (false, nil) for one-time yes.
func (e *Exec) askAndApproveCmd(ctx context.Context, command, display string) (bool, error) {
	resp, err := e.ask(ctx, fmt.Sprintf("Allow running `%s`?\n(\"trust\" will allow `%s` with ANY arguments without future prompts)", display, command), []string{"yes", "trust", "no"})
	if err != nil {
		return false, fmt.Errorf("exec_run: ask permission: %w", err)
	}

	switch strings.ToLower(resp) {
	case "trust":
		return true, e.store.TrustCommand(command)
	case "yes":
		return false, nil
	default:
		return false, fmt.Errorf("exec_run: permission denied for %s", command)
	}
}
