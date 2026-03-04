// Package grok implements the modeladapter.Completer interface for xAI's Grok models
// using the OpenAI-compatible chat completions API.
package grok

import (
	"context"
	"fmt"
	"net/http"

	"github.com/germanamz/shelly/pkg/chats/chat"
	"github.com/germanamz/shelly/pkg/chats/message"
	"github.com/germanamz/shelly/pkg/modeladapter"
	"github.com/germanamz/shelly/pkg/modeladapter/usage"
	"github.com/germanamz/shelly/pkg/providers/internal/openaicompat"
	"github.com/germanamz/shelly/pkg/tools/toolbox"
)

// DefaultBaseURL is the base URL for the xAI API (without version prefix,
// consistent with the OpenAI and Anthropic providers).
const DefaultBaseURL = "https://api.x.ai"

var (
	_ modeladapter.Completer             = (*Adapter)(nil)
	_ modeladapter.UsageReporter         = (*Adapter)(nil)
	_ modeladapter.RateLimitInfoReporter = (*Adapter)(nil)
)

// Adapter sends chat completions to xAI's Grok API.
type Adapter struct {
	client *modeladapter.Client
	Config modeladapter.ModelConfig
	usage  usage.Tracker
}

// New creates a Adapter with the given base URL, API key, model, and HTTP client.
// A nil client falls back to a default HTTP client with a 10-minute timeout.
func New(baseURL, apiKey, model string, httpClient *http.Client) *Adapter {
	opts := []modeladapter.ClientOption{
		modeladapter.WithHeaderParser(modeladapter.ParseOpenAIRateLimitHeaders),
	}
	if httpClient != nil {
		opts = append(opts, modeladapter.WithHTTPClient(httpClient))
	}
	return &Adapter{
		client: modeladapter.NewClient(baseURL, modeladapter.Auth{Key: apiKey}, opts...),
		Config: modeladapter.ModelConfig{
			Name:      model,
			MaxTokens: 4096,
		},
	}
}

// UsageTracker returns the adapter's token usage tracker.
func (g *Adapter) UsageTracker() *usage.Tracker { return &g.usage }

// ModelMaxTokens returns the maximum tokens the model will generate per response.
func (g *Adapter) ModelMaxTokens() int { return g.Config.MaxTokens }

// LastRateLimitInfo returns the most recently observed rate limit info, or nil.
func (g *Adapter) LastRateLimitInfo() *modeladapter.RateLimitInfo {
	return g.client.LastRateLimitInfo()
}

// Complete sends a conversation to the Grok chat completions endpoint
// and returns the assistant's reply.
func (g *Adapter) Complete(ctx context.Context, c *chat.Chat, tools []toolbox.Tool) (message.Message, error) {
	req := openaicompat.BuildRequest(g.Config, c, tools)

	var resp openaicompat.Response
	if err := g.client.PostJSON(ctx, openaicompat.CompletionsPath, req, &resp); err != nil {
		return message.Message{}, fmt.Errorf("grok: %w", err)
	}

	if len(resp.Choices) == 0 {
		return message.Message{}, fmt.Errorf("grok: empty response")
	}

	g.usage.Add(openaicompat.ParseUsage(resp.Usage))

	return openaicompat.ParseMessage(resp.Choices[0].Message), nil
}
