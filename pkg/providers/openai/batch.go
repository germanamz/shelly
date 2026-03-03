package openai

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"

	"github.com/germanamz/shelly/pkg/modeladapter/batch"
	"github.com/germanamz/shelly/pkg/modeladapter/usage"
)

const (
	filesPath   = "/v1/files"
	batchesPath = "/v1/batches"
)

// BatchSubmitter implements batch.Submitter for the OpenAI Batch API.
// It uploads a JSONL file, creates a batch, polls for completion, and
// downloads the results.
type BatchSubmitter struct {
	adapter *Adapter
}

// NewBatchSubmitter creates a BatchSubmitter using the given Adapter.
func NewBatchSubmitter(adapter *Adapter) *BatchSubmitter {
	return &BatchSubmitter{adapter: adapter}
}

// --- Batch API types ---

// batchRequestLine is one line of the input JSONL file.
type batchRequestLine struct {
	CustomID string     `json:"custom_id"`
	Method   string     `json:"method"`
	URL      string     `json:"url"`
	Body     apiRequest `json:"body"`
}

type createBatchRequest struct {
	InputFileID      string `json:"input_file_id"`
	Endpoint         string `json:"endpoint"`
	CompletionWindow string `json:"completion_window"`
}

type batchStatus struct {
	ID            string `json:"id"`
	Status        string `json:"status"` // "validating", "in_progress", "completed", "failed", "expired", "cancelled"
	OutputFileID  string `json:"output_file_id"`
	ErrorFileID   string `json:"error_file_id"`
	RequestCounts *struct {
		Total     int `json:"total"`
		Completed int `json:"completed"`
		Failed    int `json:"failed"`
	} `json:"request_counts"`
}

type batchResultLine struct {
	CustomID string `json:"custom_id"`
	Response *struct {
		StatusCode int         `json:"status_code"`
		Body       apiResponse `json:"body"`
	} `json:"response"`
	Error *struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

// SubmitBatch uploads the requests as a JSONL file and creates a batch.
func (b *BatchSubmitter) SubmitBatch(ctx context.Context, reqs []batch.Request) (string, error) {
	// Build JSONL content.
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	for _, r := range reqs {
		line := batchRequestLine{
			CustomID: r.ID,
			Method:   "POST",
			URL:      completionsPath,
			Body:     b.adapter.buildRequest(r.Chat, r.Tools),
		}
		if err := enc.Encode(line); err != nil {
			return "", fmt.Errorf("openai batch: encode request: %w", err)
		}
	}

	// Upload the JSONL file.
	fileID, err := b.uploadFile(ctx, buf.Bytes())
	if err != nil {
		return "", err
	}

	// Create the batch.
	createReq := createBatchRequest{
		InputFileID:      fileID,
		Endpoint:         completionsPath,
		CompletionWindow: "24h",
	}

	var resp batchStatus
	if err := b.adapter.PostJSON(ctx, batchesPath, createReq, &resp); err != nil {
		return "", fmt.Errorf("openai batch: create: %w", err)
	}

	return resp.ID, nil
}

// PollBatch checks the status of a submitted batch.
func (b *BatchSubmitter) PollBatch(ctx context.Context, batchID string) (map[string]batch.Result, bool, error) {
	path := batchesPath + "/" + batchID

	req, err := b.adapter.NewRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, false, fmt.Errorf("openai batch: poll: %w", err)
	}

	resp, err := b.adapter.Do(req)
	if err != nil {
		return nil, false, fmt.Errorf("openai batch: poll: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, false, fmt.Errorf("openai batch: poll: status %d: %s", resp.StatusCode, string(body))
	}

	var status batchStatus
	if err := json.NewDecoder(resp.Body).Decode(&status); err != nil {
		return nil, false, fmt.Errorf("openai batch: poll: decode: %w", err)
	}

	switch status.Status {
	case "completed":
		results, dlErr := b.downloadResults(ctx, status.OutputFileID)
		if dlErr != nil {
			return nil, false, dlErr
		}
		return results, true, nil
	case "failed", "expired", "cancelled":
		return nil, false, fmt.Errorf("openai batch: batch %s terminal status %q — falling back to synchronous completion", batchID, status.Status)
	default:
		return nil, false, nil // Still in progress.
	}
}

// CancelBatch attempts to cancel an in-progress batch.
func (b *BatchSubmitter) CancelBatch(ctx context.Context, batchID string) error {
	path := batchesPath + "/" + batchID + "/cancel"
	return b.adapter.PostJSON(ctx, path, nil, nil)
}

// uploadFile uploads JSONL content as a file for batch processing.
func (b *BatchSubmitter) uploadFile(ctx context.Context, data []byte) (string, error) {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)

	if err := writer.WriteField("purpose", "batch"); err != nil {
		return "", fmt.Errorf("openai batch: write purpose field: %w", err)
	}

	part, err := writer.CreateFormFile("file", "batch_input.jsonl")
	if err != nil {
		return "", fmt.Errorf("openai batch: create form file: %w", err)
	}

	if _, err := part.Write(data); err != nil {
		return "", fmt.Errorf("openai batch: write file data: %w", err)
	}

	if err := writer.Close(); err != nil {
		return "", fmt.Errorf("openai batch: close writer: %w", err)
	}

	req, err := b.adapter.NewRequest(ctx, http.MethodPost, filesPath, &body)
	if err != nil {
		return "", fmt.Errorf("openai batch: upload: %w", err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := b.adapter.Do(req)
	if err != nil {
		return "", fmt.Errorf("openai batch: upload: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return "", fmt.Errorf("openai batch: upload: status %d: %s", resp.StatusCode, string(respBody))
	}

	var fileResp struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&fileResp); err != nil {
		return "", fmt.Errorf("openai batch: upload: decode: %w", err)
	}

	return fileResp.ID, nil
}

// downloadResults fetches and parses the output JSONL file.
func (b *BatchSubmitter) downloadResults(ctx context.Context, fileID string) (map[string]batch.Result, error) {
	path := filesPath + "/" + fileID + "/content"

	req, err := b.adapter.NewRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, fmt.Errorf("openai batch: download: %w", err)
	}

	resp, err := b.adapter.Do(req)
	if err != nil {
		return nil, fmt.Errorf("openai batch: download: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("openai batch: download: status %d: %s", resp.StatusCode, string(body))
	}

	return b.parseResultsJSONL(resp.Body)
}

// parseResultsJSONL parses the output JSONL stream into a map of results.
func (b *BatchSubmitter) parseResultsJSONL(r io.Reader) (map[string]batch.Result, error) {
	results := make(map[string]batch.Result)
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var item batchResultLine
		if err := json.Unmarshal(line, &item); err != nil {
			return nil, fmt.Errorf("openai batch: parse result: %w", err)
		}

		results[item.CustomID] = b.convertResult(item)
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("openai batch: scan results: %w", err)
	}

	return results, nil
}

// convertResult converts a single result line into a batch.Result.
func (b *BatchSubmitter) convertResult(line batchResultLine) batch.Result {
	if line.Error != nil {
		return batch.Result{
			Err: fmt.Errorf("openai batch: request %s: %s: %s", line.CustomID, line.Error.Code, line.Error.Message),
		}
	}

	if line.Response == nil {
		return batch.Result{
			Err: fmt.Errorf("openai batch: request %s: missing response", line.CustomID),
		}
	}

	if line.Response.StatusCode < 200 || line.Response.StatusCode >= 300 {
		return batch.Result{
			Err: fmt.Errorf("openai batch: request %s: non-success status %d", line.CustomID, line.Response.StatusCode),
		}
	}

	resp := line.Response.Body
	if len(resp.Choices) == 0 {
		return batch.Result{
			Err: fmt.Errorf("openai batch: request %s: empty choices", line.CustomID),
		}
	}

	msg := b.adapter.parseChoice(resp.Choices[0])
	tc := usage.TokenCount{
		InputTokens:  resp.Usage.PromptTokens,
		OutputTokens: resp.Usage.CompletionTokens,
	}

	return batch.Result{Message: msg, Usage: tc}
}
