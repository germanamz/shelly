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

	"github.com/germanamz/shelly/pkg/codingtoolbox"
	"github.com/germanamz/shelly/pkg/codingtoolbox/permissions"
	"github.com/germanamz/shelly/pkg/tools/toolbox"
)

// maxBufferSize is the maximum number of bytes captured from stdout/stderr (1MB).
const maxBufferSize = 1 << 20

// OnExecFunc is called when a trusted command is about to execute, giving the
// frontend an opportunity to display what is being run without blocking.
type OnExecFunc func(ctx context.Context, display string)

// Exec provides command execution tools with permission gating.
type Exec struct {
	store    *permissions.Store
	ask      codingtoolbox.AskFunc
	onExec   OnExecFunc
	approver *codingtoolbox.Approver
}

// New creates an Exec that checks the given permissions store for trusted
// commands and prompts the user via askFn when a command is not yet trusted.
// The optional onExec callback is called for trusted commands so the frontend
// can display what is being executed.
func New(store *permissions.Store, askFn codingtoolbox.AskFunc, opts ...Option) *Exec {
	e := &Exec{store: store, ask: askFn, approver: codingtoolbox.NewApprover()}
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

	stdout := codingtoolbox.NewLimitedBuffer(maxBufferSize)
	stderr := codingtoolbox.NewLimitedBuffer(maxBufferSize)
	cmd.Stdout = stdout
	cmd.Stderr = stderr

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

	// Fast path: already trusted — no prompt needed.
	if e.store.IsCommandTrusted(command) {
		if e.onExec != nil {
			e.onExec(ctx, display)
		}

		return nil
	}

	return e.approver.Ensure(ctx, command,
		func() bool { return e.store.IsCommandTrusted(command) },
		func(ctx context.Context) codingtoolbox.ApprovalOutcome {
			trusted, err := e.askAndApproveCmd(ctx, command, display)
			return codingtoolbox.ApprovalOutcome{Err: err, Shared: trusted}
		},
		func(ctx context.Context) error {
			return e.promptPermission(ctx, command, display)
		},
	)
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
