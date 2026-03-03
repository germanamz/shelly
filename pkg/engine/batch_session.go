package engine

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/germanamz/shelly/pkg/chats/content"
	"github.com/germanamz/shelly/pkg/chats/message"
)

// DefaultBatchConcurrency is the default number of tasks to run in parallel.
const DefaultBatchConcurrency = 8

// BatchTask describes a single task in a batch run's input JSONL.
type BatchTask struct {
	ID      string `json:"id"`
	Agent   string `json:"agent"`
	Task    string `json:"task"`
	Context string `json:"context,omitempty"`
}

// BatchResult is written as one JSONL line per completed task.
type BatchResult struct {
	ID      string `json:"id"`
	Agent   string `json:"agent"`
	Status  string `json:"status"` // "completed" or "error"
	Reply   string `json:"reply,omitempty"`
	Error   string `json:"error,omitempty"`
	Elapsed string `json:"elapsed"`
}

// RunBatch reads tasks from tasksPath, runs them through the engine with bounded
// parallelism, and writes results to outputPath. Each task creates its own session
// with the specified agent, sends the task text, and collects the reply.
//
// Running tasks concurrently allows the batch Collector to accumulate multiple
// Complete() calls from independent agent trees into a single batch submission,
// maximizing cost savings from provider batch APIs.
func RunBatch(ctx context.Context, eng *Engine, tasksPath, outputPath string) error {
	tasks, err := readBatchTasks(tasksPath)
	if err != nil {
		return fmt.Errorf("batch: read tasks: %w", err)
	}

	if len(tasks) == 0 {
		return fmt.Errorf("batch: no tasks found in %s", tasksPath)
	}

	out, err := os.Create(outputPath) //nolint:gosec // output path from CLI flag
	if err != nil {
		return fmt.Errorf("batch: create output: %w", err)
	}
	defer func() { _ = out.Close() }()

	concurrency := min(DefaultBatchConcurrency, len(tasks))

	resultCh := make(chan BatchResult, concurrency)

	// Feed tasks into a channel consumed by a fixed pool of workers.
	taskCh := make(chan BatchTask, concurrency)

	var wg sync.WaitGroup
	for range concurrency {
		wg.Go(func() {
			for t := range taskCh {
				resultCh <- runSingleTask(ctx, eng, t)
			}
		})
	}

	// Send tasks to the worker pool.
	go func() {
		for _, t := range tasks {
			if ctx.Err() != nil {
				break
			}
			taskCh <- t
		}
		close(taskCh)
		wg.Wait()
		close(resultCh)
	}()

	enc := json.NewEncoder(out)
	for res := range resultCh {
		if err := enc.Encode(res); err != nil {
			return fmt.Errorf("batch: write result: %w", err)
		}
	}

	return nil
}

// runSingleTask creates a session, sends the task, and returns the result.
func runSingleTask(ctx context.Context, eng *Engine, task BatchTask) BatchResult {
	start := time.Now()

	agentName := task.Agent
	if agentName == "" {
		agentName = eng.cfg.EntryAgent
	}

	sess, err := eng.NewSession(agentName)
	if err != nil {
		return BatchResult{
			ID:      task.ID,
			Agent:   agentName,
			Status:  "error",
			Error:   err.Error(),
			Elapsed: time.Since(start).String(),
		}
	}
	defer eng.RemoveSession(sess.ID())

	// Build user message: prepend context if provided.
	userText := task.Task
	if task.Context != "" {
		userText = task.Context + "\n\n" + task.Task
	}

	reply, err := sess.SendParts(ctx, content.Text{Text: userText})
	if err != nil {
		return BatchResult{
			ID:      task.ID,
			Agent:   agentName,
			Status:  "error",
			Error:   err.Error(),
			Elapsed: time.Since(start).String(),
		}
	}

	return BatchResult{
		ID:      task.ID,
		Agent:   agentName,
		Status:  "completed",
		Reply:   replyText(reply),
		Elapsed: time.Since(start).String(),
	}
}

// replyText extracts the text content from a message.
func replyText(m message.Message) string {
	var b strings.Builder
	for _, p := range m.Parts {
		if t, ok := p.(content.Text); ok {
			b.WriteString(t.Text)
		}
	}
	return b.String()
}

// readBatchTasks reads tasks from a JSONL file.
func readBatchTasks(path string) ([]BatchTask, error) {
	f, err := os.Open(path) //nolint:gosec // path from CLI flag
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()

	return parseBatchTasks(f)
}

// parseBatchTasks reads BatchTask entries from a JSONL stream.
func parseBatchTasks(r io.Reader) ([]BatchTask, error) {
	var tasks []BatchTask
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024) // 1MB max line size for large context fields
	lineNum := 0

	for scanner.Scan() {
		lineNum++
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var task BatchTask
		if err := json.Unmarshal(line, &task); err != nil {
			return nil, fmt.Errorf("line %d: %w", lineNum, err)
		}

		if task.ID == "" {
			return nil, fmt.Errorf("line %d: task id is required", lineNum)
		}
		if task.Task == "" {
			return nil, fmt.Errorf("line %d: task field is required", lineNum)
		}

		tasks = append(tasks, task)
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return tasks, nil
}
