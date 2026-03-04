package grok

import (
	"context"

	"github.com/germanamz/shelly/pkg/modeladapter"
	"github.com/germanamz/shelly/pkg/modeladapter/batch"
	"github.com/germanamz/shelly/pkg/providers/internal/openaicompat"
)

// BatchSubmitter implements batch.Submitter for xAI's Grok API.
type BatchSubmitter struct {
	helper openaicompat.BatchHelper
	config modeladapter.ModelConfig
}

// NewBatchSubmitter creates a BatchSubmitter using the given GrokAdapter.
func NewBatchSubmitter(adapter *GrokAdapter) *BatchSubmitter {
	return &BatchSubmitter{
		helper: openaicompat.BatchHelper{Client: adapter.client, ErrPrefix: "grok batch"},
		config: adapter.Config,
	}
}

// SubmitBatch uploads requests as JSONL and creates a batch.
func (b *BatchSubmitter) SubmitBatch(ctx context.Context, reqs []batch.Request) (string, error) {
	return b.helper.SubmitBatch(ctx, b.config, reqs)
}

// PollBatch checks the status of a submitted batch.
func (b *BatchSubmitter) PollBatch(ctx context.Context, batchID string) (map[string]batch.Result, bool, error) {
	return b.helper.PollBatch(ctx, batchID)
}

// CancelBatch attempts to cancel an in-progress batch.
func (b *BatchSubmitter) CancelBatch(ctx context.Context, batchID string) error {
	return b.helper.CancelBatch(ctx, batchID)
}
