package input

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestWordWrapLineCount(t *testing.T) {
	tests := []struct {
		name  string
		text  string
		width int
		want  int
	}{
		{name: "empty", text: "", width: 10, want: 1},
		{name: "short text", text: "hello", width: 10, want: 1},
		{name: "exact width", text: "aaaa bbbbb", width: 10, want: 2},
		{name: "one over", text: "aaaa bbbbbb", width: 10, want: 2},
		{name: "word wrap boundary", text: "aaaa bbbbb ccc", width: 10, want: 2},
		{
			name: "word wrap with trailing space triggers extra line",
			text: "aaaa bbbbb ccc ", width: 10, want: 3,
		},
		{
			name: "word wrap fills second line exactly",
			text: "aaaa bbbbb cccc", width: 10, want: 3,
		},
		{
			name: "long word broken across lines",
			text: "aaaa bbbbbbbbbbb", width: 10, want: 3,
		},
		{
			name: "text fills line exactly triggers extra line",
			text: "aaaaaaa bb", width: 10, want: 2,
		},
		{
			name: "single word exact width triggers extra line",
			text: "abcdefghij", width: 10, want: 2,
		},
		{
			name: "single word one over",
			text: "abcdefghijk", width: 10, want: 2,
		},
		{
			name: "multiple wraps",
			text: "aaa bbb ccc ddd eee fff ggg", width: 10, want: 4,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := wordWrapLineCount(tt.text, tt.width)
			assert.Equal(t, tt.want, got)
		})
	}
}
