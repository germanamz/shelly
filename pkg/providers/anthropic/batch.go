package anthropic

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/germanamz/shelly/pkg/modeladapter/batch"
	"github.com/germanamz/shelly/pkg/modeladapter/usage"
)

const batchesPath = "/v1/messages/batches"

// BatchSubmitter implements batch.Submitter for the Anthropic Messages Batches API.
type BatchSubmitter struct {
	adapter *Adapter
}

// NewBatchSubmitter creates a BatchSubmitter using the given Adapter for
// request building and HTTP communication.
func NewBatchSubmitter(adapter *Adapter) *BatchSubmitter {
	return &BatchSubmitter{adapter: adapter}
}

// --- Batch API request/response types ---

type batchRequest struct {
	Requests []batchRequestItem `json:"requests"`
}

type batchRequestItem struct {
	CustomID string     `json:"custom_id"`
	Params   apiRequest `json:"params"`
}

type batchResponse struct {
	ID               string `json:"id"`
	ProcessingStatus string `json:"processing_status"`
}

type batchResultLine struct {
	CustomID string            `json:"custom_id"`
	Result   batchResultDetail `json:"result"`
}

type batchResultDetail struct {
	Type    string      `json:"type"`    // "succeeded" or "errored"
	Message apiResponse `json:"message"` // Present when type == "succeeded".
	Error   *struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"error"` // Present when type == "errored".
}

// SubmitBatch sends a batch of requests to the Anthropic Messages Batches API.
func (b *BatchSubmitter) SubmitBatch(ctx context.Context, reqs []batch.Request) (string, error) {
	items := make([]batchRequestItem, len(reqs))
	for i, r := range reqs {
		items[i] = batchRequestItem{
			CustomID: r.ID,
			Params:   b.adapter.buildRequest(r.Chat, r.Tools),
		}
	}

	payload := batchRequest{Requests: items}

	var resp batchResponse
	if err := b.adapter.client.PostJSON(ctx, batchesPath, payload, &resp); err != nil {
		return "", fmt.Errorf("anthropic batch: submit: %w", err)
	}

	return resp.ID, nil
}

// PollBatch checks the status of a submitted batch.
func (b *BatchSubmitter) PollBatch(ctx context.Context, batchID string) (map[string]batch.Result, bool, error) {
	path := batchesPath + "/" + batchID

	req, err := b.adapter.client.NewRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, false, fmt.Errorf("anthropic batch: poll: %w", err)
	}

	resp, err := b.adapter.client.Do(req)
	if err != nil {
		return nil, false, fmt.Errorf("anthropic batch: poll: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, false, fmt.Errorf("anthropic batch: poll: status %d: %s", resp.StatusCode, string(body))
	}

	var status batchResponse
	if err := json.NewDecoder(resp.Body).Decode(&status); err != nil {
		return nil, false, fmt.Errorf("anthropic batch: poll: decode: %w", err)
	}

	if status.ProcessingStatus != "ended" {
		return nil, false, nil
	}

	results, err := b.fetchResults(ctx, batchID)
	if err != nil {
		return nil, false, err
	}

	return results, true, nil
}

// CancelBatch attempts to cancel an in-progress batch.
func (b *BatchSubmitter) CancelBatch(ctx context.Context, batchID string) error {
	path := batchesPath + "/" + batchID + "/cancel"
	return b.adapter.client.PostJSON(ctx, path, nil, nil)
}

// fetchResults downloads and parses the JSONL results for a completed batch.
func (b *BatchSubmitter) fetchResults(ctx context.Context, batchID string) (map[string]batch.Result, error) {
	path := batchesPath + "/" + batchID + "/results"

	req, err := b.adapter.client.NewRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, fmt.Errorf("anthropic batch: results: %w", err)
	}

	resp, err := b.adapter.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("anthropic batch: results: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("anthropic batch: results: status %d: %s", resp.StatusCode, string(body))
	}

	return b.parseResultsJSONL(resp.Body)
}

// parseResultsJSONL parses the JSONL stream of batch results.
func (b *BatchSubmitter) parseResultsJSONL(r io.Reader) (map[string]batch.Result, error) {
	results := make(map[string]batch.Result)
	scanner := bufio.NewScanner(r)

	// Allow up to 1MB per line for large responses.
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var item batchResultLine
		if err := json.Unmarshal(line, &item); err != nil {
			return nil, fmt.Errorf("anthropic batch: parse result line: %w", err)
		}

		result := b.convertResult(item)
		results[item.CustomID] = result
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("anthropic batch: scan results: %w", err)
	}

	return results, nil
}

// convertResult converts a single batch result line into a batch.Result.
func (b *BatchSubmitter) convertResult(item batchResultLine) batch.Result {
	if item.Result.Type == "errored" {
		errMsg := "unknown error"
		if item.Result.Error != nil {
			errMsg = item.Result.Error.Message
		}
		return batch.Result{
			Err: fmt.Errorf("anthropic batch: %s: %s", item.Result.Type, errMsg),
		}
	}

	msg := b.adapter.parseResponse(item.Result.Message)
	tc := usage.TokenCount{
		InputTokens:  item.Result.Message.Usage.InputTokens,
		OutputTokens: item.Result.Message.Usage.OutputTokens,
	}

	return batch.Result{
		Message: msg,
		Usage:   tc,
	}
}
