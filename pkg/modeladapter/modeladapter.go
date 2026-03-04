package modeladapter

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/coder/websocket"
)

// Auth holds authentication settings for an LLM provider API.
type Auth struct {
	Key    string // API key value.
	Header string // Header name (default: "Authorization").
	Scheme string // Scheme prefix (default: "Bearer" when Header is "Authorization").
}

// ModelConfig holds model-specific settings.
type ModelConfig struct {
	Name        string  // Model identifier (e.g. "gpt-4").
	Temperature float64 // Sampling temperature.
	MaxTokens   int     // Maximum tokens in the response.
}

// Client provides HTTP and WebSocket transport with auth, custom headers,
// and rate limit info storage. It does NOT implement Completer — concrete
// providers compose a Client and implement Complete themselves.
type Client struct {
	baseURL       string
	auth          Auth
	headers       map[string]string
	httpClient    *http.Client
	headerParser  RateLimitHeaderParser
	rateLimitInfo atomic.Pointer[RateLimitInfo]
	clientOnce    sync.Once
	defaultClient *http.Client
}

// ClientOption configures a Client.
type ClientOption func(*Client)

// WithHTTPClient sets the HTTP client used for requests.
func WithHTTPClient(c *http.Client) ClientOption {
	return func(cl *Client) { cl.httpClient = c }
}

// WithHeaders sets extra headers applied to every request.
func WithHeaders(h map[string]string) ClientOption {
	return func(c *Client) { c.headers = h }
}

// WithHeaderParser sets the rate limit header parser.
func WithHeaderParser(p RateLimitHeaderParser) ClientOption {
	return func(c *Client) { c.headerParser = p }
}

// NewClient creates a Client with the given base URL, auth, and options.
func NewClient(baseURL string, auth Auth, opts ...ClientOption) *Client {
	c := &Client{
		baseURL: baseURL,
		auth:    auth,
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// LastRateLimitInfo returns the most recently observed rate limit info, or nil.
func (c *Client) LastRateLimitInfo() *RateLimitInfo { return c.rateLimitInfo.Load() }

// authHeader returns the header name and value for authentication.
// Returns empty strings if no auth key is configured.
func (c *Client) authHeader() (name, value string) {
	if c.auth.Key == "" {
		return "", ""
	}

	header := c.auth.Header
	if header == "" {
		header = "Authorization"
	}

	val := c.auth.Key
	if header == "Authorization" {
		scheme := c.auth.Scheme
		if scheme == "" {
			scheme = "Bearer"
		}
		val = scheme + " " + val
	} else if c.auth.Scheme != "" {
		val = c.auth.Scheme + " " + val
	}

	return header, val
}

// getHTTPClient returns the configured client or a cached default client with a 10-minute timeout.
func (c *Client) getHTTPClient() *http.Client {
	if c.httpClient != nil {
		return c.httpClient
	}

	c.clientOnce.Do(func() {
		c.defaultClient = &http.Client{Timeout: 10 * time.Minute}
	})

	return c.defaultClient
}

// NewRequest builds an *http.Request with the base URL, auth, and custom
// headers already applied.
func (c *Client) NewRequest(ctx context.Context, method, path string, body io.Reader) (*http.Request, error) {
	url := c.baseURL + path

	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return nil, err
	}

	if name, value := c.authHeader(); name != "" {
		req.Header.Set(name, value)
	}

	for k, v := range c.headers {
		req.Header.Set(k, v)
	}

	return req, nil
}

// Do sends the request using the configured HTTP client.
func (c *Client) Do(req *http.Request) (*http.Response, error) {
	return c.getHTTPClient().Do(req) //nolint:gosec // URL is built from trusted BaseURL config, not user input.
}

// PostJSON marshals payload as JSON, sends a POST to the given path,
// checks for a 2xx status, and unmarshals the response body into dest.
// If dest is nil the response body is discarded after the status check.
func (c *Client) PostJSON(ctx context.Context, path string, payload any, dest any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal payload: %w", err)
	}

	req, err := c.NewRequest(ctx, http.MethodPost, path, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := c.Do(req)
	if err != nil {
		return fmt.Errorf("do request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusTooManyRequests {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return &RateLimitError{
			RetryAfter: ParseRetryAfter(resp.Header.Get("Retry-After")),
			Body:       string(respBody),
		}
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(respBody))
	}

	// Parse and store rate limit info from response headers.
	if c.headerParser != nil {
		if info := c.headerParser(resp.Header, time.Now()); info != nil {
			c.rateLimitInfo.Store(info)
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
func (c *Client) wsURL(path string) string {
	u := c.baseURL + path

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
func (c *Client) wsHeaders() http.Header {
	h := make(http.Header)

	if name, value := c.authHeader(); name != "" {
		h.Set(name, value)
	}

	for k, v := range c.headers {
		h.Set(k, v)
	}

	return h
}

// DialWS establishes a WebSocket connection to the given path with auth and
// custom headers applied. The URL scheme is derived from BaseURL: https
// becomes wss, http becomes ws. It returns the WebSocket connection and the
// HTTP response from the handshake.
func (c *Client) DialWS(ctx context.Context, path string) (*websocket.Conn, *http.Response, error) {
	u := c.wsURL(path)

	conn, resp, err := websocket.Dial(ctx, u, &websocket.DialOptions{
		HTTPClient: c.getHTTPClient(),
		HTTPHeader: c.wsHeaders(),
	})
	if err != nil {
		return nil, resp, fmt.Errorf("dial websocket: %w", err)
	}

	return conn, resp, nil
}
