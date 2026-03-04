package openaicompat

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"

	"github.com/germanamz/shelly/pkg/modeladapter"
	"github.com/germanamz/shelly/pkg/modeladapter/batch"
)

const (
	// FilesPath is the standard OpenAI-compatible files endpoint.
	FilesPath = "/v1/files"
	// BatchesPath is the standard OpenAI-compatible batches endpoint.
	BatchesPath = "/v1/batches"
)

// BatchHelper provides shared batch operations for OpenAI-compatible APIs.
type BatchHelper struct {
	Client    *modeladapter.Client
	ErrPrefix string
}

// --- batch wire types ---

// BatchRequestLine is one line of the input JSONL file.
type BatchRequestLine struct {
	CustomID string  `json:"custom_id"`
	Method   string  `json:"method"`
	URL      string  `json:"url"`
	Body     Request `json:"body"`
}

// CreateBatchRequest is the request to create a batch.
type CreateBatchRequest struct {
	InputFileID      string `json:"input_file_id"`
	Endpoint         string `json:"endpoint"`
	CompletionWindow string `json:"completion_window"`
}

// BatchStatus is the response from checking batch status.
type BatchStatus struct {
	ID            string `json:"id"`
	Status        string `json:"status"`
	OutputFileID  string `json:"output_file_id"`
	ErrorFileID   string `json:"error_file_id,omitempty"`
	RequestCounts *struct {
		Total     int `json:"total"`
		Completed int `json:"completed"`
		Failed    int `json:"failed"`
	} `json:"request_counts,omitempty"`
}

// BatchResultLine is one line of the output JSONL file.
type BatchResultLine struct {
	CustomID string `json:"custom_id"`
	Response *struct {
		StatusCode int      `json:"status_code"`
		Body       Response `json:"body"`
	} `json:"response"`
	Error *struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

// SubmitBatch uploads requests as JSONL and creates a batch.
func (h *BatchHelper) SubmitBatch(ctx context.Context, cfg modeladapter.ModelConfig, reqs []batch.Request) (string, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)

	for _, r := range reqs {
		line := BatchRequestLine{
			CustomID: r.ID,
			Method:   "POST",
			URL:      CompletionsPath,
			Body:     BuildRequest(cfg, r.Chat, r.Tools),
		}

		if err := enc.Encode(line); err != nil {
			return "", fmt.Errorf("%s: encode request: %w", h.ErrPrefix, err)
		}
	}

	fileID, err := h.UploadFile(ctx, buf.Bytes())
	if err != nil {
		return "", err
	}

	createReq := CreateBatchRequest{
		InputFileID:      fileID,
		Endpoint:         CompletionsPath,
		CompletionWindow: "24h",
	}

	var resp BatchStatus
	if err := h.Client.PostJSON(ctx, BatchesPath, createReq, &resp); err != nil {
		return "", fmt.Errorf("%s: create: %w", h.ErrPrefix, err)
	}

	return resp.ID, nil
}

// PollBatch checks the status of a submitted batch.
func (h *BatchHelper) PollBatch(ctx context.Context, batchID string) (map[string]batch.Result, bool, error) {
	path := BatchesPath + "/" + batchID

	req, err := h.Client.NewRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, false, fmt.Errorf("%s: poll: %w", h.ErrPrefix, err)
	}

	resp, err := h.Client.Do(req)
	if err != nil {
		return nil, false, fmt.Errorf("%s: poll: %w", h.ErrPrefix, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, false, fmt.Errorf("%s: poll: status %d: %s", h.ErrPrefix, resp.StatusCode, string(body))
	}

	var status BatchStatus
	if err := json.NewDecoder(resp.Body).Decode(&status); err != nil {
		return nil, false, fmt.Errorf("%s: poll: decode: %w", h.ErrPrefix, err)
	}

	switch status.Status {
	case "completed":
		results, dlErr := h.DownloadResults(ctx, status.OutputFileID)
		if dlErr != nil {
			return nil, false, dlErr
		}

		return results, true, nil
	case "failed", "expired", "cancelled":
		return nil, false, fmt.Errorf("%s: batch %s terminal status %q — falling back to synchronous completion", h.ErrPrefix, batchID, status.Status)
	default:
		return nil, false, nil
	}
}

// CancelBatch attempts to cancel an in-progress batch.
func (h *BatchHelper) CancelBatch(ctx context.Context, batchID string) error {
	path := BatchesPath + "/" + batchID + "/cancel"
	return h.Client.PostJSON(ctx, path, nil, nil)
}

// UploadFile uploads JSONL content as a file for batch processing.
func (h *BatchHelper) UploadFile(ctx context.Context, data []byte) (string, error) {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)

	if err := writer.WriteField("purpose", "batch"); err != nil {
		return "", fmt.Errorf("%s: write purpose field: %w", h.ErrPrefix, err)
	}

	part, err := writer.CreateFormFile("file", "batch_input.jsonl")
	if err != nil {
		return "", fmt.Errorf("%s: create form file: %w", h.ErrPrefix, err)
	}

	if _, err := part.Write(data); err != nil {
		return "", fmt.Errorf("%s: write file data: %w", h.ErrPrefix, err)
	}

	if err := writer.Close(); err != nil {
		return "", fmt.Errorf("%s: close writer: %w", h.ErrPrefix, err)
	}

	req, err := h.Client.NewRequest(ctx, http.MethodPost, FilesPath, &body)
	if err != nil {
		return "", fmt.Errorf("%s: upload: %w", h.ErrPrefix, err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := h.Client.Do(req)
	if err != nil {
		return "", fmt.Errorf("%s: upload: %w", h.ErrPrefix, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return "", fmt.Errorf("%s: upload: status %d: %s", h.ErrPrefix, resp.StatusCode, string(respBody))
	}

	var fileResp struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&fileResp); err != nil {
		return "", fmt.Errorf("%s: upload: decode: %w", h.ErrPrefix, err)
	}

	return fileResp.ID, nil
}

// DownloadResults fetches and parses the output JSONL file.
func (h *BatchHelper) DownloadResults(ctx context.Context, fileID string) (map[string]batch.Result, error) {
	path := FilesPath + "/" + fileID + "/content"

	req, err := h.Client.NewRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, fmt.Errorf("%s: download: %w", h.ErrPrefix, err)
	}

	resp, err := h.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%s: download: %w", h.ErrPrefix, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("%s: download: status %d: %s", h.ErrPrefix, resp.StatusCode, string(body))
	}

	return h.ParseResultsJSONL(resp.Body)
}

// ParseResultsJSONL parses the output JSONL stream into a map of results.
func (h *BatchHelper) ParseResultsJSONL(r io.Reader) (map[string]batch.Result, error) {
	results := make(map[string]batch.Result)
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var item BatchResultLine
		if err := json.Unmarshal(line, &item); err != nil {
			return nil, fmt.Errorf("%s: parse result: %w", h.ErrPrefix, err)
		}

		results[item.CustomID] = h.ConvertResult(item)
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("%s: scan results: %w", h.ErrPrefix, err)
	}

	return results, nil
}

// ConvertResult converts a single result line into a batch.Result.
func (h *BatchHelper) ConvertResult(line BatchResultLine) batch.Result {
	if line.Error != nil {
		return batch.Result{
			Err: fmt.Errorf("%s: request %s: %s: %s", h.ErrPrefix, line.CustomID, line.Error.Code, line.Error.Message),
		}
	}

	if line.Response == nil {
		return batch.Result{
			Err: fmt.Errorf("%s: request %s: missing response", h.ErrPrefix, line.CustomID),
		}
	}

	if line.Response.StatusCode < 200 || line.Response.StatusCode >= 300 {
		return batch.Result{
			Err: fmt.Errorf("%s: request %s: non-success status %d", h.ErrPrefix, line.CustomID, line.Response.StatusCode),
		}
	}

	resp := line.Response.Body
	if len(resp.Choices) == 0 {
		return batch.Result{
			Err: fmt.Errorf("%s: request %s: empty choices", h.ErrPrefix, line.CustomID),
		}
	}

	return batch.Result{
		Message: ParseMessage(resp.Choices[0].Message),
		Usage:   ParseUsage(resp.Usage),
	}
}
