package agentctx

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestWithAgentNameRoundTrip(t *testing.T) {
	ctx := WithAgentName(context.Background(), "worker-1")
	assert.Equal(t, "worker-1", AgentNameFromContext(ctx))
}

func TestAgentNameFromContext_Empty(t *testing.T) {
	assert.Empty(t, AgentNameFromContext(context.Background()))
}

func TestWithAgentName_Overwrite(t *testing.T) {
	ctx := WithAgentName(context.Background(), "parent")
	ctx = WithAgentName(ctx, "child")
	assert.Equal(t, "child", AgentNameFromContext(ctx))
}
