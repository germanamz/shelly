package main

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
		assert.Equal(t, tt.expected, fmtTokens(tt.input), "fmtTokens(%d)", tt.input)
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
		assert.Equal(t, tt.expected, fmtDuration(tt.input), "fmtDuration(%v)", tt.input)
	}
}

func TestTruncate(t *testing.T) {
	assert.Equal(t, "hello", truncate("hello", 10))
	assert.Equal(t, "hel...", truncate("hello world", 3))
	assert.Equal(t, "hello world", truncate("hello\nworld", 20))
	assert.Empty(t, truncate("", 5))
}

func TestRenderUserMessage(t *testing.T) {
	msg := renderUserMessage("hello")
	assert.Contains(t, msg, "User")
	assert.Contains(t, msg, "hello")
	assert.Contains(t, msg, treeCorner)
}

func TestRenderUserMessageMultiLine(t *testing.T) {
	msg := renderUserMessage("line1\nline2")
	assert.Contains(t, msg, "line1")
	assert.Contains(t, msg, "line2")
}

func TestRandomThinkingMessage(t *testing.T) {
	msg := randomThinkingMessage()
	assert.NotEmpty(t, msg)

	// Verify it returns values from the list.
	assert.True(t, slices.Contains(thinkingMessages, msg),
		"randomThinkingMessage returned %q which is not in thinkingMessages", msg)
}
