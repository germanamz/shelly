package gemini

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/germanamz/shelly/pkg/modeladapter/batch"
	"github.com/germanamz/shelly/pkg/modeladapter/usage"
)

// BatchSubmitter implements batch.Submitter for the Gemini API.
// Gemini's batchGenerateContent is a synchronous inline batch endpoint —
// it accepts an array of requests and returns all results in one response.
// No polling is needed; SubmitBatch stores the results and PollBatch
// returns them immediately.
type BatchSubmitter struct {
	adapter *Adapter

	mu      sync.Mutex
	nextID  atomic.Int64
	results map[string]map[string]batch.Result // batchID → requestID → result
}

// NewBatchSubmitter creates a BatchSubmitter using the given Adapter.
func NewBatchSubmitter(adapter *Adapter) *BatchSubmitter {
	return &BatchSubmitter{
		adapter: adapter,
		results: make(map[string]map[string]batch.Result),
	}
}

// --- Batch API types ---

type batchGenerateRequest struct {
	Requests []inlineRequest `json:"requests"`
}

type inlineRequest struct {
	Model   string     `json:"model"`
	Request apiRequest `json:"request"` // Same as the single-request body.
}

type batchGenerateResponse struct {
	Responses []inlineResponse `json:"responses"`
}

type inlineResponse struct {
	Candidates    []apiCandidate `json:"candidates"`
	UsageMetadata apiUsageMeta   `json:"usageMetadata"`
	Error         *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

// SubmitBatch sends all requests as a single synchronous inline batch.
// Results are stored internally and returned by the next PollBatch call.
func (b *BatchSubmitter) SubmitBatch(ctx context.Context, reqs []batch.Request) (string, error) {
	inlineReqs := make([]inlineRequest, len(reqs))
	for i, r := range reqs {
		inlineReqs[i] = inlineRequest{
			Model:   fmt.Sprintf("models/%s", b.adapter.Config.Name),
			Request: b.adapter.buildRequest(r.Chat, r.Tools),
		}
	}

	payload := batchGenerateRequest{Requests: inlineReqs}
	path := fmt.Sprintf("/v1beta/models/%s:batchGenerateContent", b.adapter.Config.Name)

	var resp batchGenerateResponse
	if err := b.adapter.client.PostJSON(ctx, path, payload, &resp); err != nil {
		return "", fmt.Errorf("gemini batch: submit: %w", err)
	}

	// Correlate results by request order (Gemini returns results in the same order).
	results := make(map[string]batch.Result, len(reqs))
	for i, r := range reqs {
		if i >= len(resp.Responses) {
			results[r.ID] = batch.Result{
				Err: fmt.Errorf("gemini batch: missing response for request %s", r.ID),
			}
			continue
		}
		results[r.ID] = b.convertResponse(r.ID, resp.Responses[i])
	}

	// Generate a synthetic batch ID and store results.
	batchID := fmt.Sprintf("gemini-batch-%d", b.nextID.Add(1))
	b.mu.Lock()
	b.results[batchID] = results
	b.mu.Unlock()

	return batchID, nil
}

// PollBatch returns stored results immediately (Gemini batch is synchronous).
func (b *BatchSubmitter) PollBatch(_ context.Context, batchID string) (map[string]batch.Result, bool, error) {
	b.mu.Lock()
	results, ok := b.results[batchID]
	if ok {
		delete(b.results, batchID)
	}
	b.mu.Unlock()

	if !ok {
		return nil, false, fmt.Errorf("gemini batch: unknown batch %s", batchID)
	}

	return results, true, nil
}

// CancelBatch is a no-op for Gemini since the batch is synchronous.
func (b *BatchSubmitter) CancelBatch(_ context.Context, _ string) error {
	return nil
}

// convertResponse converts a single inline response into a batch.Result.
func (b *BatchSubmitter) convertResponse(reqID string, resp inlineResponse) batch.Result {
	if resp.Error != nil {
		return batch.Result{
			Err: fmt.Errorf("gemini batch: request %s: code %d: %s", reqID, resp.Error.Code, resp.Error.Message),
		}
	}

	if len(resp.Candidates) == 0 {
		return batch.Result{
			Err: fmt.Errorf("gemini batch: request %s: empty candidates", reqID),
		}
	}

	msg := b.adapter.parseCandidate(resp.Candidates[0])
	tc := usage.TokenCount{
		InputTokens:  resp.UsageMetadata.PromptTokenCount,
		OutputTokens: resp.UsageMetadata.CandidatesTokenCount,
	}

	return batch.Result{Message: msg, Usage: tc}
}
