package modeladapter

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/coder/websocket"
	"github.com/germanamz/shelly/pkg/chats/chat"
	"github.com/germanamz/shelly/pkg/chats/message"
	"github.com/germanamz/shelly/pkg/modeladapter/usage"
	"github.com/germanamz/shelly/pkg/tools/toolbox"
)

// RateLimitError is returned when the API responds with HTTP 429 (Too Many Requests).
// It carries an optional RetryAfter duration parsed from the Retry-After header.
type RateLimitError struct {
	RetryAfter time.Duration
	Body       string
}

func (e *RateLimitError) Error() string {
	if e.RetryAfter > 0 {
		return fmt.Sprintf("rate limited (retry after %s): %s", e.RetryAfter, e.Body)
	}
	return fmt.Sprintf("rate limited: %s", e.Body)
}

// ParseRetryAfter parses the Retry-After header value as either seconds (integer)
// or an HTTP-date (RFC 7231). Returns zero if unparseable or if the date is in the past.
func ParseRetryAfter(val string) time.Duration {
	if val == "" {
		return 0
	}
	if secs, err := strconv.Atoi(val); err == nil {
		return time.Duration(secs) * time.Second
	}
	if t, err := http.ParseTime(val); err == nil {
		d := time.Until(t)
		if d > 0 {
			return d
		}
		return 0
	}
	return 0
}

// Completer sends a conversation to an LLM and returns the assistant's reply.
// The tools parameter declares which tools are available for this call.
type Completer interface {
	Complete(ctx context.Context, c *chat.Chat, tools []toolbox.Tool) (message.Message, error)
}

// UsageReporter provides token usage information from a completer.
// Completers that embed ModelAdapter implement this interface automatically.
type UsageReporter interface {
	UsageTracker() *usage.Tracker
	ModelMaxTokens() int
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
	Name         string                // Model identifier (e.g. "gpt-4").
	Temperature  float64               // Sampling temperature.
	MaxTokens    int                   // Maximum tokens in the response.
	Auth         Auth                  // Authentication settings.
	BaseURL      string                // API base URL (no trailing slash).
	Client       *http.Client          // HTTP client; falls back to http.DefaultClient.
	Headers      map[string]string     // Extra headers applied to every request.
	Usage        usage.Tracker         // Token usage tracker.
	HeaderParser RateLimitHeaderParser // Optional parser for rate limit response headers.

	rateLimitInfo atomic.Pointer[RateLimitInfo]
	clientOnce    sync.Once
	defaultClient *http.Client
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

// UsageTracker returns the adapter's token usage tracker.
func (a *ModelAdapter) UsageTracker() *usage.Tracker { return &a.Usage }

// ModelMaxTokens returns the maximum tokens the model will generate per response.
func (a *ModelAdapter) ModelMaxTokens() int { return a.MaxTokens }

// LastRateLimitInfo returns the most recently observed rate limit info, or nil.
func (a *ModelAdapter) LastRateLimitInfo() *RateLimitInfo { return a.rateLimitInfo.Load() }

// Complete is a stub that returns an error. Concrete providers that embed
// ModelAdapter should define their own Complete method to shadow this one.
func (a *ModelAdapter) Complete(_ context.Context, _ *chat.Chat, _ []toolbox.Tool) (message.Message, error) {
	return message.Message{}, errors.New("adapter: Complete not implemented")
}

// httpClient returns the configured client or a cached default client with a 10-minute timeout.
func (a *ModelAdapter) httpClient() *http.Client {
	if a.Client != nil {
		return a.Client
	}

	a.clientOnce.Do(func() {
		a.defaultClient = &http.Client{Timeout: 10 * time.Minute}
	})

	return a.defaultClient
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

	if resp.StatusCode == http.StatusTooManyRequests {
		respBody, _ := io.ReadAll(resp.Body)
		return &RateLimitError{
			RetryAfter: ParseRetryAfter(resp.Header.Get("Retry-After")),
			Body:       string(respBody),
		}
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(respBody))
	}

	// Parse and store rate limit info from response headers.
	if a.HeaderParser != nil {
		if info := a.HeaderParser(resp.Header, time.Now()); info != nil {
			a.rateLimitInfo.Store(info)
		}
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
