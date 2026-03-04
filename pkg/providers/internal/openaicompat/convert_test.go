package openaicompat_test

import (
	"encoding/json"
	"testing"

	"github.com/germanamz/shelly/pkg/chats/chat"
	"github.com/germanamz/shelly/pkg/chats/content"
	"github.com/germanamz/shelly/pkg/chats/message"
	"github.com/germanamz/shelly/pkg/chats/role"
	"github.com/germanamz/shelly/pkg/providers/internal/openaicompat"
	"github.com/stretchr/testify/assert"
)

func strPtr(s string) *string { return &s }

func TestConvertMessages_AllRoles(t *testing.T) {
	c := chat.New(
		message.NewText("", role.System, "system prompt"),
		message.NewText("", role.User, "user msg"),
		message.New("", role.Assistant,
			content.Text{Text: "let me check"},
			content.ToolCall{ID: "tc-1", Name: "search", Arguments: `{"q":"test"}`},
		),
		message.New("", role.Tool,
			content.ToolResult{ToolCallID: "tc-1", Content: "result data"},
		),
		message.New("", role.Assistant, content.Text{Text: "here you go"}),
	)

	msgs := openaicompat.ConvertMessages(c.Messages())

	assert.Len(t, msgs, 5)

	assert.Equal(t, "system", msgs[0].Role)
	assert.Equal(t, "system prompt", *msgs[0].Content)

	assert.Equal(t, "user", msgs[1].Role)
	assert.Equal(t, "user msg", *msgs[1].Content)

	assert.Equal(t, "assistant", msgs[2].Role)
	assert.Equal(t, "let me check", *msgs[2].Content)
	assert.Len(t, msgs[2].ToolCalls, 1)
	assert.Equal(t, "tc-1", msgs[2].ToolCalls[0].ID)
	assert.Equal(t, "function", msgs[2].ToolCalls[0].Type)
	assert.Equal(t, "search", msgs[2].ToolCalls[0].Function.Name)
	assert.JSONEq(t, `{"q":"test"}`, msgs[2].ToolCalls[0].Function.Arguments)

	assert.Equal(t, "tool", msgs[3].Role)
	assert.Equal(t, "tc-1", msgs[3].ToolCallID)
	assert.Equal(t, "result data", *msgs[3].Content)

	assert.Equal(t, "assistant", msgs[4].Role)
	assert.Equal(t, "here you go", *msgs[4].Content)
}

func TestConvertMessages_MultipleToolResults(t *testing.T) {
	c := chat.New(
		message.New("", role.Tool,
			content.ToolResult{ToolCallID: "tc-1", Content: "result 1"},
			content.ToolResult{ToolCallID: "tc-2", Content: "result 2"},
		),
	)

	msgs := openaicompat.ConvertMessages(c.Messages())

	assert.Len(t, msgs, 2)
	assert.Equal(t, "tc-1", msgs[0].ToolCallID)
	assert.Equal(t, "result 1", *msgs[0].Content)
	assert.Equal(t, "tc-2", msgs[1].ToolCallID)
	assert.Equal(t, "result 2", *msgs[1].Content)
}

func TestParseMessage_TextOnly(t *testing.T) {
	am := openaicompat.Message{Role: "assistant", Content: strPtr("hello")}
	msg := openaicompat.ParseMessage(am)

	assert.Equal(t, role.Assistant, msg.Role)
	assert.Equal(t, "hello", msg.TextContent())
	assert.Empty(t, msg.ToolCalls())
}

func TestParseMessage_ToolCallsOnly(t *testing.T) {
	am := openaicompat.Message{
		Role: "assistant",
		ToolCalls: []openaicompat.ToolCall{
			{ID: "c1", Type: "function", Function: openaicompat.ToolFunction{Name: "fn1", Arguments: "{}"}},
			{ID: "c2", Type: "function", Function: openaicompat.ToolFunction{Name: "fn2", Arguments: `{"x":1}`}},
		},
	}
	msg := openaicompat.ParseMessage(am)

	assert.Equal(t, role.Assistant, msg.Role)
	assert.Empty(t, msg.TextContent())

	calls := msg.ToolCalls()
	assert.Len(t, calls, 2)
	assert.Equal(t, "c1", calls[0].ID)
	assert.Equal(t, "fn1", calls[0].Name)
	assert.Equal(t, "c2", calls[1].ID)
	assert.Equal(t, "fn2", calls[1].Name)
}

func TestParseMessage_TextAndToolCalls(t *testing.T) {
	am := openaicompat.Message{
		Role:    "assistant",
		Content: strPtr("thinking..."),
		ToolCalls: []openaicompat.ToolCall{
			{ID: "c1", Type: "function", Function: openaicompat.ToolFunction{Name: "fn", Arguments: "{}"}},
		},
	}
	msg := openaicompat.ParseMessage(am)

	assert.Equal(t, "thinking...", msg.TextContent())
	assert.Len(t, msg.ToolCalls(), 1)
}

func TestMarshalToolDef(t *testing.T) {
	schema := json.RawMessage(`{"type":"object","properties":{"q":{"type":"string"}}}`)
	tool := openaicompat.MarshalToolDef("search", "Search the web", schema)

	assert.Equal(t, "function", tool.Type)
	assert.Equal(t, "search", tool.Function.Name)
	assert.Equal(t, "Search the web", tool.Function.Description)
	assert.JSONEq(t, `{"type":"object","properties":{"q":{"type":"string"}}}`, string(tool.Function.Parameters))
}

func TestMarshalToolDef_NilSchema(t *testing.T) {
	tool := openaicompat.MarshalToolDef("test", "desc", nil)
	assert.JSONEq(t, `{"type":"object"}`, string(tool.Function.Parameters))
}

func TestParseUsage(t *testing.T) {
	u := openaicompat.Usage{
		PromptTokens:     100,
		CompletionTokens: 50,
		PromptTokensDetails: &openaicompat.PromptTokensDetails{
			CachedTokens: 80,
		},
	}
	tc := openaicompat.ParseUsage(u)

	assert.Equal(t, 100, tc.InputTokens)
	assert.Equal(t, 50, tc.OutputTokens)
	assert.Equal(t, 80, tc.CacheReadInputTokens)
}

func TestParseUsage_NoCaching(t *testing.T) {
	u := openaicompat.Usage{
		PromptTokens:     10,
		CompletionTokens: 5,
	}
	tc := openaicompat.ParseUsage(u)

	assert.Equal(t, 10, tc.InputTokens)
	assert.Equal(t, 5, tc.OutputTokens)
	assert.Equal(t, 0, tc.CacheReadInputTokens)
}
