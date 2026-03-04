package grok

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

// BatchSubmitter implements batch.Submitter for xAI's Grok API.
// Grok uses an OpenAI-compatible batch API with the same JSONL format.
type BatchSubmitter struct {
	adapter *GrokAdapter
}

// NewBatchSubmitter creates a BatchSubmitter using the given GrokAdapter.
func NewBatchSubmitter(adapter *GrokAdapter) *BatchSubmitter {
	return &BatchSubmitter{adapter: adapter}
}

// --- Batch API types (OpenAI-compatible) ---

type batchRequestLine struct {
	CustomID string      `json:"custom_id"`
	Method   string      `json:"method"`
	URL      string      `json:"url"`
	Body     chatRequest `json:"body"`
}

type createBatchRequest struct {
	InputFileID      string `json:"input_file_id"`
	Endpoint         string `json:"endpoint"`
	CompletionWindow string `json:"completion_window"`
}

type batchStatusResp struct {
	ID           string `json:"id"`
	Status       string `json:"status"`
	OutputFileID string `json:"output_file_id"`
}

type batchResultLine struct {
	CustomID string `json:"custom_id"`
	Response *struct {
		StatusCode int          `json:"status_code"`
		Body       chatResponse `json:"body"`
	} `json:"response"`
	Error *struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

// SubmitBatch uploads requests as JSONL and creates a batch.
func (b *BatchSubmitter) SubmitBatch(ctx context.Context, reqs []batch.Request) (string, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	for _, r := range reqs {
		line := batchRequestLine{
			CustomID: r.ID,
			Method:   "POST",
			URL:      "/v1/chat/completions",
			Body:     b.buildChatRequest(r),
		}
		if err := enc.Encode(line); err != nil {
			return "", fmt.Errorf("grok batch: encode request: %w", err)
		}
	}

	fileID, err := b.uploadFile(ctx, buf.Bytes())
	if err != nil {
		return "", err
	}

	createReq := createBatchRequest{
		InputFileID:      fileID,
		Endpoint:         "/v1/chat/completions",
		CompletionWindow: "24h",
	}

	var resp batchStatusResp
	if err := b.adapter.client.PostJSON(ctx, batchesPath, createReq, &resp); err != nil {
		return "", fmt.Errorf("grok batch: create: %w", err)
	}

	return resp.ID, nil
}

// PollBatch checks the status of a submitted batch.
func (b *BatchSubmitter) PollBatch(ctx context.Context, batchID string) (map[string]batch.Result, bool, error) {
	path := batchesPath + "/" + batchID

	req, err := b.adapter.client.NewRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, false, fmt.Errorf("grok batch: poll: %w", err)
	}

	resp, err := b.adapter.client.Do(req)
	if err != nil {
		return nil, false, fmt.Errorf("grok batch: poll: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, false, fmt.Errorf("grok batch: poll: status %d: %s", resp.StatusCode, string(body))
	}

	var status batchStatusResp
	if err := json.NewDecoder(resp.Body).Decode(&status); err != nil {
		return nil, false, fmt.Errorf("grok batch: poll: decode: %w", err)
	}

	switch status.Status {
	case "completed":
		results, dlErr := b.downloadResults(ctx, status.OutputFileID)
		if dlErr != nil {
			return nil, false, dlErr
		}
		return results, true, nil
	case "failed", "expired", "cancelled":
		return nil, false, fmt.Errorf("grok batch: batch %s terminal status %q — falling back to synchronous completion", batchID, status.Status)
	default:
		return nil, false, nil
	}
}

// CancelBatch attempts to cancel an in-progress batch.
func (b *BatchSubmitter) CancelBatch(ctx context.Context, batchID string) error {
	path := batchesPath + "/" + batchID + "/cancel"
	return b.adapter.client.PostJSON(ctx, path, nil, nil)
}

// buildChatRequest converts a batch.Request into a Grok chatRequest.
func (b *BatchSubmitter) buildChatRequest(r batch.Request) chatRequest {
	req := chatRequest{
		Model:       b.adapter.Config.Name,
		Messages:    convertMessages(r.Chat),
		Temperature: b.adapter.Config.Temperature,
		MaxTokens:   b.adapter.Config.MaxTokens,
	}

	for _, t := range r.Tools {
		schema := t.InputSchema
		if schema == nil {
			schema = json.RawMessage(`{"type":"object"}`)
		}
		req.Tools = append(req.Tools, MarshalToolDef(t.Name, t.Description, schema))
	}

	return req
}

// uploadFile uploads JSONL content as a file for batch processing.
func (b *BatchSubmitter) uploadFile(ctx context.Context, data []byte) (string, error) {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)

	if err := writer.WriteField("purpose", "batch"); err != nil {
		return "", fmt.Errorf("grok batch: write purpose field: %w", err)
	}

	part, err := writer.CreateFormFile("file", "batch_input.jsonl")
	if err != nil {
		return "", fmt.Errorf("grok batch: create form file: %w", err)
	}

	if _, err := part.Write(data); err != nil {
		return "", fmt.Errorf("grok batch: write file data: %w", err)
	}

	if err := writer.Close(); err != nil {
		return "", fmt.Errorf("grok batch: close writer: %w", err)
	}

	req, err := b.adapter.client.NewRequest(ctx, http.MethodPost, filesPath, &body)
	if err != nil {
		return "", fmt.Errorf("grok batch: upload: %w", err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := b.adapter.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("grok batch: upload: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return "", fmt.Errorf("grok batch: upload: status %d: %s", resp.StatusCode, string(respBody))
	}

	var fileResp struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&fileResp); err != nil {
		return "", fmt.Errorf("grok batch: upload: decode: %w", err)
	}

	return fileResp.ID, nil
}

// downloadResults fetches and parses the output JSONL file.
func (b *BatchSubmitter) downloadResults(ctx context.Context, fileID string) (map[string]batch.Result, error) {
	path := filesPath + "/" + fileID + "/content"

	req, err := b.adapter.client.NewRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, fmt.Errorf("grok batch: download: %w", err)
	}

	resp, err := b.adapter.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("grok batch: download: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("grok batch: download: status %d: %s", resp.StatusCode, string(body))
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
			return nil, fmt.Errorf("grok batch: parse result: %w", err)
		}

		results[item.CustomID] = b.convertResult(item)
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("grok batch: scan results: %w", err)
	}

	return results, nil
}

// convertResult converts a single result line into a batch.Result.
func (b *BatchSubmitter) convertResult(line batchResultLine) batch.Result {
	if line.Error != nil {
		return batch.Result{
			Err: fmt.Errorf("grok batch: request %s: %s: %s", line.CustomID, line.Error.Code, line.Error.Message),
		}
	}

	if line.Response == nil {
		return batch.Result{
			Err: fmt.Errorf("grok batch: request %s: missing response", line.CustomID),
		}
	}

	if line.Response.StatusCode < 200 || line.Response.StatusCode >= 300 {
		return batch.Result{
			Err: fmt.Errorf("grok batch: request %s: non-success status %d", line.CustomID, line.Response.StatusCode),
		}
	}

	resp := line.Response.Body
	if len(resp.Choices) == 0 {
		return batch.Result{
			Err: fmt.Errorf("grok batch: request %s: empty choices", line.CustomID),
		}
	}

	msg := convertResponse(resp.Choices[0].Message)
	tc := usage.TokenCount{
		InputTokens:  resp.Usage.PromptTokens,
		OutputTokens: resp.Usage.CompletionTokens,
	}

	return batch.Result{Message: msg, Usage: tc}
}
