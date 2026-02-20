package modeladapter

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/coder/websocket"
	"github.com/germanamz/shelly/pkg/chatty/chat"
	"github.com/germanamz/shelly/pkg/chatty/message"
	"github.com/germanamz/shelly/pkg/modeladapter/usage"
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

// ModelAdapter holds shared state for LLM provider implementations. Embed it in
// concrete provider structs to get HTTP helpers, auth, custom headers, and
// usage tracking. Concrete types should define their own Complete method to
// shadow the default stub.
type ModelAdapter struct {
	Name        string            // Model identifier (e.g. "gpt-4").
	Temperature float64           // Sampling temperature.
	MaxTokens   int               // Maximum tokens in the response.
	Auth        Auth              // Authentication settings.
	BaseURL     string            // API base URL (no trailing slash).
	Client      *http.Client      // HTTP client; falls back to http.DefaultClient.
	Headers     map[string]string // Extra headers applied to every request.
	Usage       usage.Tracker     // Token usage tracker.
}

// New creates a ModelAdapter with the given settings.
// A nil client falls back to http.DefaultClient at call time.
func New(baseURL string, auth Auth, client *http.Client) ModelAdapter {
	return ModelAdapter{
		Auth:    auth,
		BaseURL: baseURL,
		Client:  client,
	}
}

// Complete is a stub that returns an error. Concrete providers that embed
// ModelAdapter should define their own Complete method to shadow this one.
func (a *ModelAdapter) Complete(_ context.Context, _ *chat.Chat) (message.Message, error) {
	return message.Message{}, errors.New("adapter: Complete not implemented")
}

// httpClient returns the configured client or http.DefaultClient.
func (a *ModelAdapter) httpClient() *http.Client {
	if a.Client != nil {
		return a.Client
	}

	return http.DefaultClient
}

// NewRequest builds an *http.Request with the base URL, auth, and custom
// headers already applied.
func (a *ModelAdapter) NewRequest(ctx context.Context, method, path string, body io.Reader) (*http.Request, error) {
	url := a.BaseURL + path

	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return nil, err
	}

	// Apply auth.
	if a.Auth.Key != "" {
		header := a.Auth.Header
		if header == "" {
			header = "Authorization"
		}

		value := a.Auth.Key
		if header == "Authorization" {
			scheme := a.Auth.Scheme
			if scheme == "" {
				scheme = "Bearer"
			}

			value = scheme + " " + value
		} else if a.Auth.Scheme != "" {
			value = a.Auth.Scheme + " " + value
		}

		req.Header.Set(header, value)
	}

	// Apply custom headers.
	for k, v := range a.Headers {
		req.Header.Set(k, v)
	}

	return req, nil
}

// Do sends the request using the configured HTTP client.
func (a *ModelAdapter) Do(req *http.Request) (*http.Response, error) {
	return a.httpClient().Do(req) //nolint:gosec // URL is built from trusted BaseURL config, not user input.
}

// PostJSON marshals payload as JSON, sends a POST to the given path,
// checks for a 2xx status, and unmarshals the response body into dest.
// If dest is nil the response body is discarded after the status check.
func (a *ModelAdapter) PostJSON(ctx context.Context, path string, payload any, dest any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal payload: %w", err)
	}

	req, err := a.NewRequest(ctx, http.MethodPost, path, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := a.Do(req)
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

// wsURL converts the BaseURL to a WebSocket URL and appends the path.
// https becomes wss, http becomes ws. URLs that already use ws/wss are
// left unchanged.
func (a *ModelAdapter) wsURL(path string) string {
	u := a.BaseURL + path

	if strings.HasPrefix(u, "https://") {
		return "wss://" + u[len("https://"):]
	}

	if strings.HasPrefix(u, "http://") {
		return "ws://" + u[len("http://"):]
	}

	return u
}

// wsHeaders returns an http.Header with auth and custom headers applied,
// for use with WebSocket dial options.
func (a *ModelAdapter) wsHeaders() http.Header {
	h := make(http.Header)

	if a.Auth.Key != "" {
		header := a.Auth.Header
		if header == "" {
			header = "Authorization"
		}

		value := a.Auth.Key
		if header == "Authorization" {
			scheme := a.Auth.Scheme
			if scheme == "" {
				scheme = "Bearer"
			}

			value = scheme + " " + value
		} else if a.Auth.Scheme != "" {
			value = a.Auth.Scheme + " " + value
		}

		h.Set(header, value)
	}

	for k, v := range a.Headers {
		h.Set(k, v)
	}

	return h
}

// DialWS establishes a WebSocket connection to the given path with auth and
// custom headers applied. The URL scheme is derived from BaseURL: https
// becomes wss, http becomes ws. It returns the WebSocket connection and the
// HTTP response from the handshake.
func (a *ModelAdapter) DialWS(ctx context.Context, path string) (*websocket.Conn, *http.Response, error) {
	u := a.wsURL(path)

	conn, resp, err := websocket.Dial(ctx, u, &websocket.DialOptions{
		HTTPClient: a.httpClient(),
		HTTPHeader: a.wsHeaders(),
	})
	if err != nil {
		return nil, resp, fmt.Errorf("dial websocket: %w", err)
	}

	return conn, resp, nil
}
