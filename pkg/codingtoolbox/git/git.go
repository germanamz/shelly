// Package git provides tools that give agents controlled access to git
// operations. Command execution is gated by the shared permissions store
// using the command trust model (trusting "git").
package git

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	osexec "os/exec"
	"strconv"
	"strings"

	"github.com/germanamz/shelly/pkg/codingtoolbox/permissions"
	"github.com/germanamz/shelly/pkg/tools/toolbox"
)

// AskFunc asks the user a question and blocks until a response is received.
type AskFunc func(ctx context.Context, question string, options []string) (string, error)

// Git provides git tools with permission gating.
type Git struct {
	store   *permissions.Store
	ask     AskFunc
	workDir string
}

// New creates a Git that checks the given permissions store for trusted
// commands and prompts the user via askFn when git is not yet trusted.
// workDir sets the working directory for all git commands.
func New(store *permissions.Store, askFn AskFunc, workDir string) *Git {
	return &Git{store: store, ask: askFn, workDir: workDir}
}

// Tools returns a ToolBox containing the git tools.
func (g *Git) Tools() *toolbox.ToolBox {
	tb := toolbox.New()
	tb.Register(g.statusTool(), g.diffTool(), g.logTool(), g.commitTool())

	return tb
}

// checkPermission checks if git is trusted, prompting the user if not.
func (g *Git) checkPermission(ctx context.Context, description string) error {
	if g.store.IsCommandTrusted("git") {
		return nil
	}

	resp, err := g.ask(ctx, fmt.Sprintf("Allow running `%s`?", description), []string{"yes", "trust", "no"})
	if err != nil {
		return fmt.Errorf("git: ask permission: %w", err)
	}

	switch strings.ToLower(resp) {
	case "trust":
		return g.store.TrustCommand("git")
	case "yes":
		return nil
	default:
		return fmt.Errorf("git: permission denied")
	}
}

// runGit executes a git command and returns the combined output.
func (g *Git) runGit(ctx context.Context, args ...string) (string, error) {
	cmd := osexec.CommandContext(ctx, "git", args...) //nolint:gosec // command is approved by user
	if g.workDir != "" {
		cmd.Dir = g.workDir
	}

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
		return "", fmt.Errorf("git: %w\n%s", err, result.String())
	}

	return result.String(), nil
}

// --- git_status ---

type statusInput struct {
	Short bool `json:"short"`
}

func (g *Git) statusTool() toolbox.Tool {
	return toolbox.Tool{
		Name:        "git_status",
		Description: "Show the working tree status.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"short":{"type":"boolean","description":"Show short format"}}}`),
		Handler:     g.handleStatus,
	}
}

func (g *Git) handleStatus(ctx context.Context, input json.RawMessage) (string, error) {
	var in statusInput
	if err := json.Unmarshal(input, &in); err != nil {
		return "", fmt.Errorf("git_status: invalid input: %w", err)
	}

	args := []string{"status"}
	if in.Short {
		args = append(args, "--short")
	}

	if err := g.checkPermission(ctx, "git "+strings.Join(args, " ")); err != nil {
		return "", err
	}

	return g.runGit(ctx, args...)
}

// --- git_diff ---

type diffInput struct {
	Staged bool   `json:"staged"`
	Path   string `json:"path"`
}

func (g *Git) diffTool() toolbox.Tool {
	return toolbox.Tool{
		Name:        "git_diff",
		Description: "Show changes between commits, commit and working tree, etc.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"staged":{"type":"boolean","description":"Show staged changes (--cached)"},"path":{"type":"string","description":"Limit diff to a specific path"}}}`),
		Handler:     g.handleDiff,
	}
}

func (g *Git) handleDiff(ctx context.Context, input json.RawMessage) (string, error) {
	var in diffInput
	if err := json.Unmarshal(input, &in); err != nil {
		return "", fmt.Errorf("git_diff: invalid input: %w", err)
	}

	args := []string{"diff"}
	if in.Staged {
		args = append(args, "--cached")
	}

	if in.Path != "" {
		args = append(args, "--", in.Path)
	}

	if err := g.checkPermission(ctx, "git "+strings.Join(args, " ")); err != nil {
		return "", err
	}

	return g.runGit(ctx, args...)
}

// --- git_log ---

type logInput struct {
	Count  int    `json:"count"`
	Format string `json:"format"`
}

func (g *Git) logTool() toolbox.Tool {
	return toolbox.Tool{
		Name:        "git_log",
		Description: "Show commit logs.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"count":{"type":"integer","description":"Number of commits to show (default 10)"},"format":{"type":"string","description":"Pretty format (default oneline)"}}}`),
		Handler:     g.handleLog,
	}
}

func (g *Git) handleLog(ctx context.Context, input json.RawMessage) (string, error) {
	var in logInput
	if err := json.Unmarshal(input, &in); err != nil {
		return "", fmt.Errorf("git_log: invalid input: %w", err)
	}

	count := in.Count
	if count <= 0 {
		count = 10
	}

	format := in.Format
	if format == "" {
		format = "oneline"
	}

	args := []string{"log", "--pretty=" + format, "-n", strconv.Itoa(count)}

	if err := g.checkPermission(ctx, "git "+strings.Join(args, " ")); err != nil {
		return "", err
	}

	return g.runGit(ctx, args...)
}

// --- git_commit ---

type commitInput struct {
	Message string   `json:"message"`
	Files   []string `json:"files"`
	All     bool     `json:"all"`
}

func (g *Git) commitTool() toolbox.Tool {
	return toolbox.Tool{
		Name:        "git_commit",
		Description: "Create a git commit. Optionally stage specific files first.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"message":{"type":"string","description":"Commit message"},"files":{"type":"array","items":{"type":"string"},"description":"Files to stage before committing"},"all":{"type":"boolean","description":"Stage all tracked changes (-a)"}},"required":["message"]}`),
		Handler:     g.handleCommit,
	}
}

func (g *Git) handleCommit(ctx context.Context, input json.RawMessage) (string, error) {
	var in commitInput
	if err := json.Unmarshal(input, &in); err != nil {
		return "", fmt.Errorf("git_commit: invalid input: %w", err)
	}

	if in.Message == "" {
		return "", fmt.Errorf("git_commit: message is required")
	}

	if err := g.checkPermission(ctx, "git commit"); err != nil {
		return "", err
	}

	// Stage specific files if provided.
	if len(in.Files) > 0 {
		addArgs := append([]string{"add"}, in.Files...)
		if _, err := g.runGit(ctx, addArgs...); err != nil {
			return "", fmt.Errorf("git_commit: stage files: %w", err)
		}
	}

	commitArgs := []string{"commit", "-m", in.Message}
	if in.All {
		commitArgs = append(commitArgs, "-a")
	}

	return g.runGit(ctx, commitArgs...)
}
