package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/germanamz/shelly/pkg/chatty/chat"
	"github.com/germanamz/shelly/pkg/chatty/message"
	"github.com/germanamz/shelly/pkg/providers/model"
	"github.com/germanamz/shelly/pkg/providers/usage"
)

// Completer sends a conversation to an LLM and returns the assistant's reply.
type Completer interface {
	Complete(ctx context.Context, c *chat.Chat) (message.Message, error)
}

// Auth holds authentication settings for an LLM provider API.
type Auth struct {
	Key    string // API key value.
	Header string // Header name (default: "Authorization").
	Scheme string // Scheme prefix (default: "Bearer" when Header is "Authorization").
}

// Provider holds shared state for LLM provider implementations. Embed it in
// concrete provider structs to get HTTP helpers, auth, custom headers, and
// usage tracking. Concrete types should define their own Complete method to
// shadow the default stub.
type Provider struct {
	model.Model                   // Embeds Name, Temperature, MaxTokens.
	Auth        Auth              // Authentication settings.
	BaseURL     string            // API base URL (no trailing slash).
	Client      *http.Client      // HTTP client; falls back to http.DefaultClient.
	Headers     map[string]string // Extra headers applied to every request.
	Usage       usage.Tracker     // Token usage tracker.
}

// NewProvider creates a Provider with the given settings.
// A nil client falls back to http.DefaultClient at call time.
func NewProvider(baseURL string, auth Auth, m model.Model, client *http.Client) Provider {
	return Provider{
		Model:   m,
		Auth:    auth,
		BaseURL: baseURL,
		Client:  client,
	}
}

// Complete is a stub that returns an error. Concrete providers that embed
// Provider should define their own Complete method to shadow this one.
func (p *Provider) Complete(_ context.Context, _ *chat.Chat) (message.Message, error) {
	return message.Message{}, errors.New("provider: Complete not implemented")
}

// httpClient returns the configured client or http.DefaultClient.
func (p *Provider) httpClient() *http.Client {
	if p.Client != nil {
		return p.Client
	}

	return http.DefaultClient
}

// NewRequest builds an *http.Request with the base URL, auth, and custom
// headers already applied.
func (p *Provider) NewRequest(ctx context.Context, method, path string, body io.Reader) (*http.Request, error) {
	url := p.BaseURL + path

	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return nil, err
	}

	// Apply auth.
	if p.Auth.Key != "" {
		header := p.Auth.Header
		if header == "" {
			header = "Authorization"
		}

		value := p.Auth.Key
		if header == "Authorization" {
			scheme := p.Auth.Scheme
			if scheme == "" {
				scheme = "Bearer"
			}

			value = scheme + " " + value
		} else if p.Auth.Scheme != "" {
			value = p.Auth.Scheme + " " + value
		}

		req.Header.Set(header, value)
	}

	// Apply custom headers.
	for k, v := range p.Headers {
		req.Header.Set(k, v)
	}

	return req, nil
}

// Do sends the request using the configured HTTP client.
func (p *Provider) Do(req *http.Request) (*http.Response, error) {
	return p.httpClient().Do(req) //nolint:gosec // URL is built from trusted BaseURL config, not user input.
}

// PostJSON marshals payload as JSON, sends a POST to the given path,
// checks for a 2xx status, and unmarshals the response body into dest.
// If dest is nil the response body is discarded after the status check.
func (p *Provider) PostJSON(ctx context.Context, path string, payload any, dest any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal payload: %w", err)
	}

	req, err := p.NewRequest(ctx, http.MethodPost, path, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := p.Do(req)
	if err != nil {
		return fmt.Errorf("do request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(respBody))
	}

	if dest == nil {
		return nil
	}

	if err := json.NewDecoder(resp.Body).Decode(dest); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}

	return nil
}
