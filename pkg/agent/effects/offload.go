package effects

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/germanamz/shelly/pkg/agent"
	"github.com/germanamz/shelly/pkg/chats/content"
	"github.com/germanamz/shelly/pkg/chats/role"
	"github.com/germanamz/shelly/pkg/modeladapter"
	"github.com/germanamz/shelly/pkg/tools/toolbox"
)

const (
	defaultOffloadThreshold    = 0.5
	defaultOffloadMinResultLen = 2000
	defaultOffloadRecentWindow = 6
	offloadMetaKey             = "offloaded"
	offloadPreviewLen          = 200
)

// OffloadConfig holds parameters for the OffloadEffect.
type OffloadConfig struct {
	ContextWindow int     // Provider's context window size.
	Threshold     float64 // Fraction triggering offload (default 0.5).
	MinResultLen  int     // Only offload results longer than this (default 2000 runes).
	RecentWindow  int     // Keep last N tool messages in-context (default 6).
	StorageDir    string  // Directory for offloaded data.
}

// OffloadEffect stores large tool results externally and replaces them with
// summary + reference in the chat. It provides a "recall" tool so the agent
// can reload offloaded content on demand.
//
// It runs at PhaseAfterComplete. When token estimate or usage exceeds the
// threshold, it scans non-recent tool result messages and offloads those
// exceeding MinResultLen to disk.
type OffloadEffect struct {
	cfg   OffloadConfig
	store sync.Map // toolCallID → filepath
}

// NewOffloadEffect creates an OffloadEffect with the given configuration,
// applying defaults for zero values.
func NewOffloadEffect(cfg OffloadConfig) *OffloadEffect {
	if cfg.Threshold <= 0 {
		cfg.Threshold = defaultOffloadThreshold
	}
	if cfg.MinResultLen <= 0 {
		cfg.MinResultLen = defaultOffloadMinResultLen
	}
	if cfg.RecentWindow < 0 {
		cfg.RecentWindow = defaultOffloadRecentWindow
	}
	return &OffloadEffect{cfg: cfg}
}

// Eval implements agent.Effect.
func (e *OffloadEffect) Eval(_ context.Context, ic agent.IterationContext) error {
	if ic.Phase != agent.PhaseAfterComplete {
		return nil
	}

	if !e.shouldOffload(ic.Completer, ic.EstimatedTokens) {
		return nil
	}

	return e.offload(ic)
}

// Reset implements agent.Resetter. It cleans up offloaded temp files.
func (e *OffloadEffect) Reset() {
	e.store.Range(func(key, value any) bool {
		if path, ok := value.(string); ok {
			os.Remove(path) //nolint:errcheck,gosec // best-effort cleanup
		}
		e.store.Delete(key)
		return true
	})
}

// Recall retrieves an offloaded tool result by its tool call ID.
func (e *OffloadEffect) Recall(id string) (string, error) {
	path, ok := e.store.Load(id)
	if !ok {
		return "", fmt.Errorf("offload: no offloaded content for ID %q", id)
	}

	data, err := os.ReadFile(path.(string)) //nolint:gosec // path comes from our own store
	if err != nil {
		return "", fmt.Errorf("offload: read %q: %w", id, err)
	}

	return string(data), nil
}

// ProvidedTools implements agent.ToolProvider, returning a toolbox containing
// the recall tool.
func (e *OffloadEffect) ProvidedTools() *toolbox.ToolBox {
	if e.cfg.StorageDir == "" {
		return nil
	}

	tb := toolbox.New()
	tb.Register(toolbox.Tool{
		Name:        "recall",
		Description: "Retrieve content that was offloaded from context to save tokens. Pass the tool call ID shown in the offload placeholder.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"id":{"type":"string","description":"The tool call ID of the offloaded content"}},"required":["id"]}`),
		Handler: func(_ context.Context, input json.RawMessage) (string, error) {
			var args struct {
				ID string `json:"id"`
			}
			if err := json.Unmarshal(input, &args); err != nil {
				return "", fmt.Errorf("recall: invalid input: %w", err)
			}
			return e.Recall(args.ID)
		},
	})
	return tb
}

// shouldOffload returns true when the context exceeds the threshold.
func (e *OffloadEffect) shouldOffload(completer modeladapter.Completer, estimatedTokens int) bool {
	if e.cfg.ContextWindow <= 0 || e.cfg.Threshold <= 0 || e.cfg.StorageDir == "" {
		return false
	}

	limit := int(float64(e.cfg.ContextWindow) * e.cfg.Threshold)

	if estimatedTokens > 0 && estimatedTokens >= limit {
		return true
	}

	reporter, ok := completer.(modeladapter.UsageReporter)
	if !ok {
		return false
	}

	last, ok := reporter.UsageTracker().Last()
	if !ok {
		return false
	}

	return last.InputTokens >= limit
}

// offload writes large tool results to disk and replaces them with placeholders.
func (e *OffloadEffect) offload(ic agent.IterationContext) error {
	msgs := ic.Chat.Messages()

	// Find tool message indices.
	var toolIndices []int
	for i, m := range msgs {
		if m.Role == role.Tool {
			toolIndices = append(toolIndices, i)
		}
	}

	// Determine which tool messages are recent (preserved).
	preserveSet := make(map[int]bool)
	preserveStart := max(len(toolIndices)-e.cfg.RecentWindow, 0)
	for _, idx := range toolIndices[preserveStart:] {
		preserveSet[idx] = true
	}

	// Ensure storage directory exists.
	if err := os.MkdirAll(e.cfg.StorageDir, 0o750); err != nil {
		return nil // best-effort; don't fail the loop
	}

	modified := false

	for i := range msgs {
		if msgs[i].Role != role.Tool || preserveSet[i] {
			continue
		}

		if _, ok := msgs[i].GetMeta(offloadMetaKey); ok {
			continue
		}

		offloaded := false

		for j, p := range msgs[i].Parts {
			tr, ok := p.(content.ToolResult)
			if !ok || tr.IsError {
				continue
			}

			if utf8.RuneCountInString(tr.Content) < e.cfg.MinResultLen {
				continue
			}

			// Write to disk.
			filename := sanitizeFilename(tr.ToolCallID) + ".txt"
			path := filepath.Join(e.cfg.StorageDir, filename)
			if err := os.WriteFile(path, []byte(tr.Content), 0o600); err != nil {
				continue // best-effort
			}

			e.store.Store(tr.ToolCallID, path)

			// Build preview.
			preview := truncate(tr.Content, offloadPreviewLen)
			tr.Content = fmt.Sprintf("[offloaded: use recall(%q) to retrieve. Summary: %s]", tr.ToolCallID, preview)
			msgs[i].Parts[j] = tr
			offloaded = true
		}

		if offloaded {
			msgs[i].SetMeta(offloadMetaKey, true)
			modified = true
		}
	}

	if modified {
		ic.Chat.Replace(msgs...)
	}

	return nil
}

// sanitizeFilename replaces non-alphanumeric characters with hyphens so a
// tool-call ID can be used safely as a filename component.
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
