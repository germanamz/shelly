package main

import (
	"slices"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestTruncate(t *testing.T) {
	tests := []struct {
		name string
		s    string
		n    int
		want string
	}{
		{name: "short string", s: "hello", n: 10, want: "hello"},
		{name: "exact length", s: "hello", n: 5, want: "hello"},
		{name: "truncated", s: "hello world", n: 5, want: "hello..."},
		{name: "newlines replaced", s: "hello\nworld", n: 20, want: "hello world"},
		{name: "newlines and truncated", s: "hello\nworld\nfoo", n: 9, want: "hello wor..."},
		{name: "empty string", s: "", n: 10, want: ""},
		{name: "unicode preserved", s: "日本語テスト", n: 3, want: "日本語..."},
		{name: "zero length", s: "abc", n: 0, want: "..."},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, truncate(tt.s, tt.n))
		})
	}
}

func TestMoveCursorWordLeft(t *testing.T) {
	line := []rune("hello world")
	tests := []struct {
		name     string
		cursor   int
		expected int
	}{
		{name: "middle of word", cursor: 5, expected: 0},
		{name: "start of word", cursor: 6, expected: 0},
		{name: "middle of second word", cursor: 7, expected: 6},
		{name: "end of second word", cursor: 11, expected: 6},
		{name: "start of line", cursor: 0, expected: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, moveCursorWordLeft(line, tt.cursor))
		})
	}
}

func TestMoveCursorWordRight(t *testing.T) {
	line := []rune("hello world")
	tests := []struct {
		name     string
		cursor   int
		expected int
	}{
		{name: "middle of word", cursor: 5, expected: 6},
		{name: "start of word", cursor: 6, expected: 11},
		{name: "middle of second word", cursor: 7, expected: 11},
		{name: "end of line", cursor: 11, expected: 11},
		{name: "in space", cursor: 5, expected: 6},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, moveCursorWordRight(line, tt.cursor))
		})
	}
}

func TestFmtTokens(t *testing.T) {
	tests := []struct {
		name string
		n    int
		want string
	}{
		{name: "zero", n: 0, want: "0"},
		{name: "small", n: 42, want: "42"},
		{name: "thousand", n: 1200, want: "1.2k"},
		{name: "million", n: 1500000, want: "1.5M"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, fmtTokens(tt.n))
		})
	}
}

func TestFmtDuration(t *testing.T) {
	tests := []struct {
		name string
		d    time.Duration
		want string
	}{
		{name: "seconds", d: 2500 * time.Millisecond, want: "2.5s"},
		{name: "minutes", d: 75 * time.Second, want: "1m 15s"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, fmtDuration(tt.d))
		})
	}
}

func TestDeleteWordBackward(t *testing.T) {
	line := []rune("hello world")
	newLine, cursor := deleteWordBackward(line, 11)
	assert.Equal(t, "hello ", string(newLine))
	assert.Equal(t, 6, cursor)
}

func TestRandomThinkingMessage(t *testing.T) {
	msg := randomThinkingMessage()
	assert.NotEmpty(t, msg)
	assert.True(t, slices.Contains(thinkingMessages, msg), "randomThinkingMessage should return one of the predefined messages")
}
