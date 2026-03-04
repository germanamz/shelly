package effects

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/germanamz/shelly/pkg/chats/content"
	"github.com/germanamz/shelly/pkg/chats/message"
	"github.com/germanamz/shelly/pkg/chats/role"
)

const (
	maxToolArgs   = 200
	maxToolResult = 500
)

// renderMessages converts a slice of messages into a compact text transcript,
// skipping system messages. Tool call arguments are truncated to 200 runes,
// tool results to 500 runes.
func renderMessages(msgs []message.Message) string {
	var b strings.Builder

	for _, m := range msgs {
		if m.Role == role.System {
			continue
		}

		for _, p := range m.Parts {
			switch v := p.(type) {
			case content.Text:
				fmt.Fprintf(&b, "[%s] %s\n", m.Role, v.Text)
			case content.ToolCall:
				args := truncate(v.Arguments, maxToolArgs)
				fmt.Fprintf(&b, "[%s] Called tool %s(%s)\n", m.Role, v.Name, args)
			case content.ToolResult:
				body := truncate(v.Content, maxToolResult)
				if v.IsError {
					fmt.Fprintf(&b, "[tool error] %s\n", body)
				} else {
					fmt.Fprintf(&b, "[tool result] %s\n", body)
				}
			}
		}
	}

	return b.String()
}

// truncate returns s truncated to maxLen runes with "…" appended if needed.
func truncate(s string, maxLen int) string {
	if utf8.RuneCountInString(s) <= maxLen {
		return s
	}

	runes := []rune(s)

	return string(runes[:maxLen]) + "\u2026"
}
