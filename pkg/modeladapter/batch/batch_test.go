package batch_test

import (
	"context"
	"testing"

	"github.com/germanamz/shelly/pkg/chats/chat"
	"github.com/germanamz/shelly/pkg/chats/message"
	"github.com/germanamz/shelly/pkg/chats/role"
	"github.com/germanamz/shelly/pkg/modeladapter/batch"
	"github.com/germanamz/shelly/pkg/modeladapter/usage"
	"github.com/germanamz/shelly/pkg/tools/toolbox"
	"github.com/stretchr/testify/assert"
)

func TestRequest_Fields(t *testing.T) {
	ch := chat.New(message.NewText("", role.User, "hello"))
	tools := []toolbox.Tool{{Name: "test_tool"}}

	req := batch.Request{
		ID:    "req-1",
		Chat:  ch,
		Tools: tools,
	}

	assert.Equal(t, "req-1", req.ID)
	assert.Equal(t, ch, req.Chat)
	assert.Len(t, req.Tools, 1)
}

func TestResult_WithError(t *testing.T) {
	res := batch.Result{
		Err: context.DeadlineExceeded,
	}
	assert.ErrorIs(t, res.Err, context.DeadlineExceeded)
}

func TestResult_WithMessage(t *testing.T) {
	msg := message.NewText("", role.Assistant, "response")
	res := batch.Result{
		Message: msg,
		Usage:   usage.TokenCount{InputTokens: 10, OutputTokens: 5},
	}

	assert.Equal(t, role.Assistant, res.Message.Role)
	assert.Equal(t, 10, res.Usage.InputTokens)
	assert.Equal(t, 5, res.Usage.OutputTokens)
}
