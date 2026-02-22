package filesystem

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSessionTrust_DefaultUntrusted(t *testing.T) {
	st := &SessionTrust{}
	assert.False(t, st.IsTrusted())
}

func TestSessionTrust_Trust(t *testing.T) {
	st := &SessionTrust{}
	st.Trust()
	assert.True(t, st.IsTrusted())
}

func TestSessionTrust_ContextRoundTrip(t *testing.T) {
	st := &SessionTrust{}
	ctx := WithSessionTrust(context.Background(), st)

	got := sessionTrustFromContext(ctx)
	assert.Same(t, st, got)
}

func TestSessionTrust_ContextMissing(t *testing.T) {
	got := sessionTrustFromContext(context.Background())
	assert.Nil(t, got)
}
