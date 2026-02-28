package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/germanamz/shelly/pkg/chats/message"
	"github.com/germanamz/shelly/pkg/chats/role"
	"github.com/germanamz/shelly/pkg/tools/toolbox"
)

// AgentEventData carries metadata about an agent lifecycle event.
type AgentEventData struct {
	Prefix string // Display prefix (e.g. "🤖", "📝").
	Parent string // Name of the parent agent (empty for top-level).
}

// orchestrationToolBox builds a ToolBox containing the built-in orchestration
// tools (list_agents, delegate) for the given agent.
func orchestrationToolBox(a *Agent) *toolbox.ToolBox {
	tb := toolbox.New()
	tb.Register(
		listAgentsTool(a),
		delegateTool(a),
	)

	return tb
}

// --- list_agents ---

func listAgentsTool(a *Agent) toolbox.Tool {
	return toolbox.Tool{
		Name:        "list_agents",
		Description: "List all available agents that can be delegated to",
		InputSchema: json.RawMessage(`{"type":"object","properties":{}}`),
		Handler: func(_ context.Context, _ json.RawMessage) (string, error) {
			entries := a.registry.List()

			// Filter out self.
			var filtered []Entry
			for _, e := range entries {
				if !strings.EqualFold(e.Name, a.name) {
					filtered = append(filtered, e)
				}
			}

			data, err := json.Marshal(filtered)
			if err != nil {
				return "", fmt.Errorf("list_agents: %w", err)
			}

			return string(data), nil
		},
	}
}

// --- delegate ---

type delegateTask struct {
	Agent   string `json:"agent"`
	Task    string `json:"task"`
	Context string `json:"context"`
	TaskID  string `json:"task_id"`
}

type delegateInput struct {
	Tasks []delegateTask `json:"tasks"`
}

type delegateResult struct {
	Agent      string            `json:"agent"`
	Result     string            `json:"result,omitempty"`
	Completion *CompletionResult `json:"completion,omitempty"`
	Error      string            `json:"error,omitempty"`
}

func delegateTool(a *Agent) toolbox.Tool {
	return toolbox.Tool{
		Name:        "delegate",
		Description: "Delegate tasks to other agents. Accepts one or more tasks; all run concurrently. Use the context field to pass relevant background information so agents do not need to re-explore. Pass task_id on each task to automatically claim and update task board entries.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"tasks":{"type":"array","items":{"type":"object","properties":{"agent":{"type":"string","description":"Name of the agent"},"task":{"type":"string","description":"The task to delegate"},"context":{"type":"string","description":"Background context for the agent: relevant file contents, decisions, constraints, or any info the agent needs to complete the task without re-exploring."},"task_id":{"type":"string","description":"Optional task board ID. When provided, the task is auto-claimed for the child agent and its status is updated based on the completion result."}},"required":["agent","task","context"]},"description":"List of agent tasks to run concurrently"}},"required":["tasks"]}`),
		Handler: func(ctx context.Context, input json.RawMessage) (string, error) {
			var di delegateInput
			if err := json.Unmarshal(input, &di); err != nil {
				return "", fmt.Errorf("delegate: invalid input: %w", err)
			}

			if len(di.Tasks) == 0 {
				return "[]", nil
			}

			for _, t := range di.Tasks {
				if strings.EqualFold(t.Agent, a.name) {
					return "", fmt.Errorf("delegate: self-delegation is not allowed")
				}
			}

			if a.options.MaxDelegationDepth > 0 && a.depth >= a.options.MaxDelegationDepth {
				return "", fmt.Errorf("delegate: max delegation depth %d reached", a.options.MaxDelegationDepth)
			}

			results := make([]delegateResult, len(di.Tasks))

			var wg sync.WaitGroup

			// Snapshot toolboxes before spawning goroutines to avoid a data
			// race on the parent's a.toolboxes slice.
			toolboxSnapshot := make([]*toolbox.ToolBox, len(a.toolboxes))
			copy(toolboxSnapshot, a.toolboxes)

			eventNotifier := a.options.EventNotifier
			eventFunc := a.options.EventFunc
			reflectionDir := a.options.ReflectionDir
			taskBoard := a.options.TaskBoard
			maxDelegationDepth := a.options.MaxDelegationDepth

			for i, t := range di.Tasks {
				wg.Go(func() {
					child, ok := a.registry.Spawn(t.Agent, a.depth+1)
					if !ok {
						results[i] = delegateResult{
							Agent: t.Agent,
							Error: fmt.Sprintf("agent %q not found", t.Agent),
						}
						return
					}

					child.registry = a.registry
					child.options.EventNotifier = eventNotifier
					child.options.EventFunc = eventFunc
					child.options.ReflectionDir = reflectionDir
					child.options.TaskBoard = taskBoard
					child.options.MaxDelegationDepth = maxDelegationDepth
					child.AddToolBoxes(toolboxSnapshot...)
					prependContext(child, t.Context)

					if reflections := searchReflections(reflectionDir, t.Task); reflections != "" {
						child.chat.Append(message.NewText("user", role.User, reflections))
					}

					child.chat.Append(message.NewText("user", role.User, t.Task))

					// Auto-claim task if task_id is provided and TaskBoard is available.
					if t.TaskID != "" && taskBoard != nil {
						if claimErr := taskBoard.ClaimTask(t.TaskID, child.name); claimErr != nil {
							results[i] = delegateResult{
								Agent: t.Agent,
								Error: fmt.Sprintf("failed to claim task %q: %v", t.TaskID, claimErr),
							}
							return
						}
					}

					if eventNotifier != nil {
						eventNotifier(ctx, "agent_start", child.name, AgentEventData{Prefix: child.Prefix(), Parent: a.name})
					}

					reply, err := child.Run(ctx)

					if eventNotifier != nil {
						eventNotifier(ctx, "agent_end", child.name, AgentEventData{Prefix: child.Prefix(), Parent: a.name})
					}

					if err != nil {
						if errors.Is(err, ErrMaxIterations) {
							cr := &CompletionResult{
								Status:  "failed",
								Summary: fmt.Sprintf("Agent %q exhausted its iteration limit without completing the task.", t.Agent),
								Caveats: "Iteration limit reached. Check progress notes for partial work.",
							}
							writeReflection(reflectionDir, t.Agent, t.Task, cr)
							if t.TaskID != "" && taskBoard != nil {
								_ = taskBoard.UpdateTaskStatus(t.TaskID, cr.Status)
							}
							results[i] = delegateResult{
								Agent:      t.Agent,
								Completion: cr,
							}
							return
						}
						results[i] = delegateResult{
							Agent: t.Agent,
							Error: err.Error(),
						}
						return
					}

					// Auto-update task status based on completion result.
					if cr := child.CompletionResult(); cr != nil {
						if cr.Status == "failed" {
							writeReflection(reflectionDir, t.Agent, t.Task, cr)
						}
						if t.TaskID != "" && taskBoard != nil {
							_ = taskBoard.UpdateTaskStatus(t.TaskID, cr.Status)
						}
					}

					results[i] = buildDelegateResult(t.Agent, reply, child.CompletionResult())
				})
			}

			wg.Wait()

			data, err := json.Marshal(results)
			if err != nil {
				return "", fmt.Errorf("delegate: %w", err)
			}

			return string(data), nil
		},
	}
}

// --- task_complete ---

type taskCompleteInput struct {
	Status        string   `json:"status"`
	Summary       string   `json:"summary"`
	FilesModified []string `json:"files_modified"`
	TestsRun      []string `json:"tests_run"`
	Caveats       string   `json:"caveats"`
}

func taskCompleteTool(a *Agent) toolbox.Tool {
	return toolbox.Tool{
		Name:        "task_complete",
		Description: "Signal task completion with structured metadata. Call this when you have finished your delegated task.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"status":{"type":"string","enum":["completed","failed"],"description":"Whether the task was completed successfully or failed"},"summary":{"type":"string","description":"Concise description of what was done or why it failed"},"files_modified":{"type":"array","items":{"type":"string"},"description":"List of files that were modified"},"tests_run":{"type":"array","items":{"type":"string"},"description":"List of tests that were executed"},"caveats":{"type":"string","description":"Known limitations or follow-up work needed"}},"required":["status","summary"]}`),
		Handler: func(_ context.Context, input json.RawMessage) (string, error) {
			var tci taskCompleteInput
			if err := json.Unmarshal(input, &tci); err != nil {
				return "", fmt.Errorf("task_complete: invalid input: %w", err)
			}

			if tci.Status != "completed" && tci.Status != "failed" {
				return "", fmt.Errorf("task_complete: status must be \"completed\" or \"failed\", got %q", tci.Status)
			}

			alreadySet := true
			a.completionOnce.Do(func() {
				alreadySet = false
				a.completionResult = &CompletionResult{
					Status:        tci.Status,
					Summary:       tci.Summary,
					FilesModified: tci.FilesModified,
					TestsRun:      tci.TestsRun,
					Caveats:       tci.Caveats,
				}
			})

			if alreadySet {
				return "Task already marked — duplicate call ignored.", nil
			}

			return fmt.Sprintf("Task marked as %s.", tci.Status), nil
		},
	}
}

const maxDelegateResultLen = 2000

// buildDelegateResult constructs a delegateResult from a child agent's reply.
// When a CompletionResult is available, its Summary is used as the primary
// result to keep the parent's context concise. Otherwise, the reply text is
// truncated to maxDelegateResultLen.
func buildDelegateResult(agentName string, reply message.Message, cr *CompletionResult) delegateResult {
	result := delegateResult{
		Agent:      agentName,
		Completion: cr,
	}

	switch {
	case cr != nil && cr.Summary != "":
		result.Result = cr.Summary
	case cr != nil && cr.Status != "":
		parts := []string{cr.Status}
		if cr.Caveats != "" {
			parts = append(parts, cr.Caveats)
		}
		result.Result = strings.Join(parts, ": ")
	default:
		text := reply.TextContent()
		if utf8.RuneCountInString(text) > maxDelegateResultLen {
			text = string([]rune(text)[:maxDelegateResultLen]) + "… [truncated]"
		}
		result.Result = text
	}

	return result
}

// prependContext adds a context message before the task message
// in a child agent's chat. The context is wrapped in <delegation_context> tags.
// If ctx is empty, no message is appended.
func prependContext(child *Agent, ctx string) {
	if ctx == "" {
		return
	}
	child.chat.Append(message.NewText("user", role.User,
		"<delegation_context>\n"+ctx+"\n</delegation_context>"))
}

// --- reflection helpers ---

// writeReflection writes a reflection note when a sub-agent fails.
// It is best-effort: errors are silently ignored.
func writeReflection(dir string, agentName string, task string, cr *CompletionResult) {
	if dir == "" || cr == nil || cr.Status != "failed" {
		return
	}

	if err := os.MkdirAll(dir, 0o750); err != nil {
		return // best-effort
	}

	now := time.Now().UTC()
	timestamp := now.Format(time.RFC3339)
	// Use a sanitized filename based on agent name and timestamp.
	safeName := sanitizeFilename(agentName + "-" + now.Format("20060102-150405"))
	path := filepath.Join(dir, safeName+".md")

	var b strings.Builder
	fmt.Fprintf(&b, "# Reflection: %s\n\n", agentName)
	fmt.Fprintf(&b, "**Timestamp**: %s\n\n", timestamp)
	fmt.Fprintf(&b, "## Task\n%s\n\n", task)
	fmt.Fprintf(&b, "## Summary\n%s\n\n", cr.Summary)
	if cr.Caveats != "" {
		fmt.Fprintf(&b, "## Caveats\n%s\n\n", cr.Caveats)
	}
	if len(cr.FilesModified) > 0 {
		b.WriteString("## Files Modified\n")
		for _, f := range cr.FilesModified {
			fmt.Fprintf(&b, "- %s\n", f)
		}
		b.WriteString("\n")
	}

	os.WriteFile(path, []byte(b.String()), 0o600) //nolint:errcheck,gosec // best-effort reflection
}

// sanitizeFilename replaces any non-alphanumeric, non-hyphen, non-underscore
// characters with hyphens.
func sanitizeFilename(s string) string {
	var b strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			b.WriteRune(r)
		} else {
			b.WriteRune('-')
		}
	}
	return b.String()
}

const (
	maxReflectionFiles = 5
	maxReflectionBytes = 32 * 1024
)

// searchReflections searches for relevant reflections before delegating.
// Returns an empty string if no relevant reflections are found.
func searchReflections(dir string, task string) string {
	if dir == "" {
		return ""
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}

	sort.Slice(entries, func(i, j int) bool {
		fi, errI := entries[i].Info()
		fj, errJ := entries[j].Info()
		if errI != nil && errJ != nil {
			return false
		}
		if errI != nil {
			return false // errored entries sort to the end
		}
		if errJ != nil {
			return true
		}
		return fi.ModTime().After(fj.ModTime())
	})

	var reflections []string
	var totalBytes int
	taskLower := strings.ToLower(task)

	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}

		data, err := os.ReadFile(filepath.Join(dir, e.Name())) //nolint:gosec // dir is controlled by config, entry names from ReadDir
		if err != nil {
			continue
		}

		content := string(data)
		// Simple relevance: check if any words from the task appear in the reflection.
		if containsRelevantKeywords(taskLower, strings.ToLower(content)) {
			reflections = append(reflections, content)
			totalBytes += len(data)
			if len(reflections) >= maxReflectionFiles || totalBytes >= maxReflectionBytes {
				break
			}
		}
	}

	if len(reflections) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString("\n<prior_reflections>\nPrevious attempts at similar tasks failed. Learn from these reflections:\n\n")
	for _, r := range reflections {
		b.WriteString(r)
		b.WriteString("\n---\n")
	}
	b.WriteString("</prior_reflections>")
	return b.String()
}

// containsRelevantKeywords checks if the content shares significant keywords with the task.
func containsRelevantKeywords(task, content string) bool {
	words := strings.Fields(task)
	matches := 0
	for _, w := range words {
		if len(w) < 4 { // skip short words like "the", "for", etc.
			continue
		}
		if strings.Contains(content, w) {
			matches++
		}
	}
	return matches >= 2 // at least 2 significant word matches
}
