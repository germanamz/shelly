// Package openai provides a Completer implementation for the OpenAI Chat Completions API.
package openai

import (
	"context"
	"fmt"

	"github.com/germanamz/shelly/pkg/chats/chat"
	"github.com/germanamz/shelly/pkg/chats/message"
	"github.com/germanamz/shelly/pkg/modeladapter"
	"github.com/germanamz/shelly/pkg/modeladapter/usage"
	"github.com/germanamz/shelly/pkg/providers/internal/openaicompat"
	"github.com/germanamz/shelly/pkg/tools/toolbox"
)

var (
	_ modeladapter.Completer             = (*Adapter)(nil)
	_ modeladapter.UsageReporter         = (*Adapter)(nil)
	_ modeladapter.RateLimitInfoReporter = (*Adapter)(nil)
)

// Adapter implements modeladapter.Completer for the OpenAI Chat Completions API.
type Adapter struct {
	client *modeladapter.Client
	Config modeladapter.ModelConfig
	usage  usage.Tracker
}

// New creates an Adapter configured for the OpenAI API.
// The baseURL should be "https://api.openai.com" (no trailing slash).
func New(baseURL, apiKey, model string) *Adapter {
	return &Adapter{
		client: modeladapter.NewClient(baseURL, modeladapter.Auth{Key: apiKey},
			modeladapter.WithHeaderParser(modeladapter.ParseOpenAIRateLimitHeaders)),
		Config: modeladapter.ModelConfig{
			Name:      model,
			MaxTokens: 4096,
		},
	}
}

// UsageTracker returns the adapter's token usage tracker.
func (a *Adapter) UsageTracker() *usage.Tracker { return &a.usage }

// ModelMaxTokens returns the maximum tokens the model will generate per response.
func (a *Adapter) ModelMaxTokens() int { return a.Config.MaxTokens }

// LastRateLimitInfo returns the most recently observed rate limit info, or nil.
func (a *Adapter) LastRateLimitInfo() *modeladapter.RateLimitInfo {
	return a.client.LastRateLimitInfo()
}

// Complete sends a conversation to the OpenAI Chat Completions API and returns
// the assistant's reply.
func (a *Adapter) Complete(ctx context.Context, c *chat.Chat, tools []toolbox.Tool) (message.Message, error) {
	req := openaicompat.BuildRequest(a.Config, c, tools)

	var resp openaicompat.Response
	if err := a.client.PostJSON(ctx, openaicompat.CompletionsPath, req, &resp); err != nil {
		return message.Message{}, fmt.Errorf("openai: %w", err)
	}

	if len(resp.Choices) == 0 {
		return message.Message{}, fmt.Errorf("openai: empty choices in response")
	}

	a.usage.Add(openaicompat.ParseUsage(resp.Usage))

	return openaicompat.ParseMessage(resp.Choices[0].Message), nil
}
