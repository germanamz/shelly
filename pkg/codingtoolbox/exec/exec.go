// Package exec provides a tool that gives agents the ability to run CLI
// commands. Every command execution is gated by explicit user permission.
// Users can "trust" a command (program name) to allow it for all future
// invocations without being prompted again. Trusted commands are persisted
// to the shared permissions file.
package exec

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	osexec "os/exec"
	"strings"

	"github.com/germanamz/shelly/pkg/codingtoolbox/permissions"
	"github.com/germanamz/shelly/pkg/tools/toolbox"
)

// AskFunc asks the user a question and blocks until a response is received.
type AskFunc func(ctx context.Context, question string, options []string) (string, error)

// Exec provides command execution tools with permission gating.
type Exec struct {
	store *permissions.Store
	ask   AskFunc
}

// New creates an Exec that checks the given permissions store for trusted
// commands and prompts the user via askFn when a command is not yet trusted.
func New(store *permissions.Store, askFn AskFunc) *Exec {
	return &Exec{store: store, ask: askFn}
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

	var stdout, stderr bytes.Buffer
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
	if e.store.IsCommandTrusted(command) {
		return nil
	}

	display := command
	if len(args) > 0 {
		display += " " + strings.Join(args, " ")
	}

	resp, err := e.ask(ctx, fmt.Sprintf("Allow running `%s`?", display), []string{"yes", "trust", "no"})
	if err != nil {
		return fmt.Errorf("exec_run: ask permission: %w", err)
	}

	switch strings.ToLower(resp) {
	case "trust":
		if err := e.store.TrustCommand(command); err != nil {
			return err
		}

		return nil
	case "yes":
		return nil
	default:
		return fmt.Errorf("exec_run: permission denied for %s", command)
	}
}
