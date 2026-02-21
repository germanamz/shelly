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
