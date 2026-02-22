package main

import (
	"testing"

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
		{name: "middle of word", cursor: 5, expected: 0},        // 'o' in hello -> start of hello
		{name: "start of word", cursor: 6, expected: 0},         // 'w' in world -> start of hello
		{name: "middle of second word", cursor: 7, expected: 6}, // 'o' in world -> start of world
		{name: "end of second word", cursor: 11, expected: 6},   // end -> start of world
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
		{name: "middle of word", cursor: 5, expected: 6},         // 'o' in hello -> start of world
		{name: "start of word", cursor: 6, expected: 11},         // 'w' in world -> end
		{name: "middle of second word", cursor: 7, expected: 11}, // 'o' in world -> end
		{name: "end of line", cursor: 11, expected: 11},
		{name: "in space", cursor: 5, expected: 6}, // but 5 is 'o', not space
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, moveCursorWordRight(line, tt.cursor))
		})
	}
}
