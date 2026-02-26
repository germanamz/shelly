package modeladapter_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/germanamz/shelly/pkg/chats/chat"
	"github.com/germanamz/shelly/pkg/chats/content"
	"github.com/germanamz/shelly/pkg/chats/message"
	"github.com/germanamz/shelly/pkg/chats/role"
	"github.com/germanamz/shelly/pkg/modeladapter"
	"github.com/germanamz/shelly/pkg/tools/toolbox"
	"github.com/stretchr/testify/assert"
)

func TestEstimateChat_Empty(t *testing.T) {
	e := &modeladapter.TokenEstimator{}
	c := chat.New()

	assert.Equal(t, 0, e.EstimateChat(c))
}

func TestEstimateChat_TextMessages(t *testing.T) {
	e := &modeladapter.TokenEstimator{}
	c := chat.New(
		message.NewText("user", role.User, "Hello, how are you?"),       // 19 chars
		message.NewText("bot", role.Assistant, "I am fine, thank you!"), // 21 chars
	)

	got := e.EstimateChat(c)
	assert.Positive(t, got)
	// At minimum: 2 messages * 4 overhead = 8.
	assert.GreaterOrEqual(t, got, 8)
}

func TestEstimateChat_WithToolCalls(t *testing.T) {
	e := &modeladapter.TokenEstimator{}

	c := chat.New(
		message.NewText("user", role.User, "Search for golang"),
		message.New("bot", role.Assistant,
			content.Text{Text: "Let me search."},
			content.ToolCall{ID: "c1", Name: "browser_search", Arguments: `{"query":"golang"}`},
		),
		message.New("tool", role.Tool,
			content.ToolResult{ToolCallID: "c1", Content: "Found results."},
		),
	)

	got := e.EstimateChat(c)
	assert.Greater(t, got, 12) // 3 messages * 4 overhead minimum
}

func TestEstimateTools_Empty(t *testing.T) {
	e := &modeladapter.TokenEstimator{}
	assert.Equal(t, 0, e.EstimateTools(nil))
}

func TestEstimateTools_SingleTool(t *testing.T) {
	e := &modeladapter.TokenEstimator{}
	schema := json.RawMessage(`{"type":"object","properties":{"query":{"type":"string"}}}`)

	tools := []toolbox.Tool{
		{Name: "browser_search", Description: "Search the web", InputSchema: schema},
	}

	got := e.EstimateTools(tools)
	assert.Greater(t, got, 10) // at least the overhead
}

func TestEstimateTotal_ChatPlusTools(t *testing.T) {
	e := &modeladapter.TokenEstimator{}

	c := chat.New(
		message.NewText("user", role.User, "What is the weather?"),
	)

	schema := json.RawMessage(`{"type":"object","properties":{"city":{"type":"string"}}}`)
	tools := []toolbox.Tool{
		{Name: "get_weather", Description: "Get current weather", InputSchema: schema},
	}

	chatOnly := e.EstimateChat(c)
	toolsOnly := e.EstimateTools(tools)
	total := e.EstimateTotal(c, tools)

	assert.Equal(t, chatOnly+toolsOnly, total)
}

func TestEstimateChat_LargeConversation(t *testing.T) {
	e := &modeladapter.TokenEstimator{}

	msgs := make([]message.Message, 100)
	text := strings.Repeat("a", 100)
	for i := range msgs {
		r := role.User
		if i%2 == 1 {
			r = role.Assistant
		}
		msgs[i] = message.NewText("agent", r, text)
	}

	c := chat.New(msgs...)

	got := e.EstimateChat(c)
	// Each message: 4 overhead + ceil(100/4)=25 = 29.
	assert.Equal(t, 2900, got)
}
