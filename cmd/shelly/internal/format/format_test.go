package format

import (
	"slices"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestFmtTokens(t *testing.T) {
	tests := []struct {
		input    int
		expected string
	}{
		{0, "0"},
		{500, "500"},
		{999, "999"},
		{1000, "1.0k"},
		{1200, "1.2k"},
		{15000, "15.0k"},
		{999_999, "1000.0k"},
		{1_000_000, "1.0M"},
		{3_400_000, "3.4M"},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.expected, FmtTokens(tt.input), "FmtTokens(%d)", tt.input)
	}
}

func TestFmtDuration(t *testing.T) {
	tests := []struct {
		input    time.Duration
		expected string
	}{
		{100 * time.Millisecond, "0.1s"},
		{2 * time.Second, "2.0s"},
		{30 * time.Second, "30.0s"},
		{65 * time.Second, "1m 5s"},
		{125 * time.Second, "2m 5s"},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.expected, FmtDuration(tt.input), "FmtDuration(%v)", tt.input)
	}
}

func TestTruncate(t *testing.T) {
	assert.Equal(t, "hello", Truncate("hello", 10))
	assert.Equal(t, "hel...", Truncate("hello world", 3))
	assert.Equal(t, "hello world", Truncate("hello\nworld", 20))
	assert.Empty(t, Truncate("", 5))
}

func TestRenderUserMessage(t *testing.T) {
	msg := RenderUserMessage("hello", 80)
	assert.Contains(t, msg, "User")
	assert.Contains(t, msg, "hello")
	assert.Contains(t, msg, "└ ")
}

func TestRenderUserMessageMultiLine(t *testing.T) {
	msg := RenderUserMessage("line1\nline2", 80)
	assert.Contains(t, msg, "line1")
	assert.Contains(t, msg, "line2")
}

func TestFmtCost(t *testing.T) {
	tests := []struct {
		input    float64
		expected string
	}{
		{0.0, "$0.0000"},
		{0.0023, "$0.0023"},
		{0.0099, "$0.0099"},
		{0.01, "$0.010"},
		{0.342, "$0.342"},
		{0.999, "$0.999"},
		{1.0, "$1.00"},
		{12.5, "$12.50"},
		{100.123, "$100.12"},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.expected, FmtCost(tt.input), "FmtCost(%v)", tt.input)
	}
}

func TestFmtBytes(t *testing.T) {
	tests := []struct {
		input    int
		expected string
	}{
		{0, "0 B"},
		{500, "500 B"},
		{1024, "1.0 KB"},
		{1536, "1.5 KB"},
		{1048576, "1.0 MB"},
		{2621440, "2.5 MB"},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.expected, FmtBytes(tt.input), "FmtBytes(%d)", tt.input)
	}
}

func TestRandomThinkingMessage(t *testing.T) {
	msg := RandomThinkingMessage()
	assert.NotEmpty(t, msg)

	// Verify it returns values from the list.
	assert.True(t, slices.Contains(ThinkingMessages, msg),
		"RandomThinkingMessage returned %q which is not in ThinkingMessages", msg)
}
