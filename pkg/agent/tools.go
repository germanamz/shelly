package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"

	"github.com/germanamz/shelly/pkg/chats/message"
	"github.com/germanamz/shelly/pkg/chats/role"
	"github.com/germanamz/shelly/pkg/tools/toolbox"
)

// AgentEventData carries metadata about an agent lifecycle event.
type AgentEventData struct {
	Prefix string // Display prefix (e.g. "🤖", "📝").
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
				if e.Name != a.name {
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
				if t.Agent == a.name {
					return "", fmt.Errorf("delegate: self-delegation is not allowed")
				}
			}

			if a.options.MaxDelegationDepth > 0 && a.depth >= a.options.MaxDelegationDepth {
				return "", fmt.Errorf("delegate: max delegation depth %d reached", a.options.MaxDelegationDepth)
			}

			results := make([]delegateResult, len(di.Tasks))

			var wg sync.WaitGroup

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
					child.options.EventNotifier = a.options.EventNotifier
					child.AddToolBoxes(a.toolboxes...)
					prependContext(child, t.Context)
					child.chat.Append(message.NewText("user", role.User, t.Task))

					// Auto-claim task if task_id is provided and TaskBoard is available.
					if t.TaskID != "" && a.options.TaskBoard != nil {
						_ = a.options.TaskBoard.ClaimTask(t.TaskID, child.name)
					}

					if a.options.EventNotifier != nil {
						a.options.EventNotifier(ctx, "agent_start", child.name, AgentEventData{Prefix: child.Prefix()})
					}

					reply, err := child.Run(ctx)

					if a.options.EventNotifier != nil {
						a.options.EventNotifier(ctx, "agent_end", child.name, AgentEventData{Prefix: child.Prefix()})
					}

					if err != nil {
						if errors.Is(err, ErrMaxIterations) {
							cr := &CompletionResult{
								Status:  "failed",
								Summary: fmt.Sprintf("Agent %q exhausted its iteration limit without completing the task.", t.Agent),
								Caveats: "Iteration limit reached. Check progress notes for partial work.",
							}
							if t.TaskID != "" && a.options.TaskBoard != nil {
								_ = a.options.TaskBoard.UpdateTaskStatus(t.TaskID, cr.Status)
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
					if t.TaskID != "" && a.options.TaskBoard != nil {
						if cr := child.CompletionResult(); cr != nil {
							_ = a.options.TaskBoard.UpdateTaskStatus(t.TaskID, cr.Status)
						}
					}

					results[i] = delegateResult{
						Agent:      t.Agent,
						Result:     reply.TextContent(),
						Completion: child.CompletionResult(),
					}
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

			a.completionResult = &CompletionResult{
				Status:        tci.Status,
				Summary:       tci.Summary,
				FilesModified: tci.FilesModified,
				TestsRun:      tci.TestsRun,
				Caveats:       tci.Caveats,
			}

			return fmt.Sprintf("Task marked as %s.", tci.Status), nil
		},
	}
}

// prependContext adds a context message before the task message
// in a child agent's chat. The context is wrapped in <delegation_context> tags.
func prependContext(child *Agent, ctx string) {
	child.chat.Append(message.NewText("user", role.User,
		"<delegation_context>\n"+ctx+"\n</delegation_context>"))
}
