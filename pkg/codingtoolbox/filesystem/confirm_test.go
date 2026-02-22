package filesystem

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestComputeDiff_Changes(t *testing.T) {
	diff := computeDiff("test.txt", "hello\nworld\n", "hello\nearth\n")
	assert.Contains(t, diff, "-world")
	assert.Contains(t, diff, "+earth")
}

func TestComputeDiff_Identical(t *testing.T) {
	diff := computeDiff("test.txt", "same\n", "same\n")
	assert.Empty(t, diff)
}

func TestComputeDiff_NewFile(t *testing.T) {
	diff := computeDiff("test.txt", "", "new content\n")
	assert.Contains(t, diff, "+new content")
}

func TestConfirmChange_Approve(t *testing.T) {
	askFn := func(_ context.Context, _ string, _ []string) (string, error) {
		return "yes", nil
	}
	fs, _ := newTestFS(t, askFn)

	st := &SessionTrust{}
	ctx := WithSessionTrust(context.Background(), st)

	err := fs.confirmChange(ctx, "/tmp/test.txt", "some diff")
	require.NoError(t, err)
	assert.False(t, st.IsTrusted(), "should not trust after single approval")
}

func TestConfirmChange_Deny(t *testing.T) {
	askFn := func(_ context.Context, _ string, _ []string) (string, error) {
		return "no", nil
	}
	fs, _ := newTestFS(t, askFn)

	st := &SessionTrust{}
	ctx := WithSessionTrust(context.Background(), st)

	err := fs.confirmChange(ctx, "/tmp/test.txt", "some diff")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "denied")
}

func TestConfirmChange_TrustSession(t *testing.T) {
	askFn := func(_ context.Context, _ string, _ []string) (string, error) {
		return "trust this session", nil
	}
	fs, _ := newTestFS(t, askFn)

	st := &SessionTrust{}
	ctx := WithSessionTrust(context.Background(), st)

	err := fs.confirmChange(ctx, "/tmp/test.txt", "some diff")
	require.NoError(t, err)
	assert.True(t, st.IsTrusted(), "should trust after 'trust this session'")
}

func TestConfirmChange_AlreadyTrusted(t *testing.T) {
	askCalled := false
	askFn := func(_ context.Context, _ string, _ []string) (string, error) {
		askCalled = true
		return "yes", nil
	}

	var notified string
	notifyFn := func(_ context.Context, msg string) {
		notified = msg
	}

	fs, _ := newTestFSWithNotify(t, askFn, notifyFn)

	st := &SessionTrust{}
	st.Trust()
	ctx := WithSessionTrust(context.Background(), st)

	err := fs.confirmChange(ctx, "/tmp/test.txt", "some diff")
	require.NoError(t, err)
	assert.False(t, askCalled, "should not ask when session is trusted")
	assert.Contains(t, notified, "test.txt")
}

func TestConfirmChange_NoSessionTrust(t *testing.T) {
	askFn := func(_ context.Context, _ string, _ []string) (string, error) {
		return "yes", nil
	}
	fs, _ := newTestFS(t, askFn)

	// No SessionTrust in context — should still ask and work.
	err := fs.confirmChange(context.Background(), "/tmp/test.txt", "some diff")
	require.NoError(t, err)
}
