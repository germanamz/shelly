// Package http provides a tool that gives agents the ability to make HTTP
// requests. Each domain is gated by explicit user permission. Users can
// "trust" a domain to allow all future requests without being prompted again.
// Trusted domains are persisted to the shared permissions file.
package http

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/germanamz/shelly/pkg/codingtoolbox/permissions"
	"github.com/germanamz/shelly/pkg/tools/toolbox"
)

// AskFunc asks the user a question and blocks until a response is received.
type AskFunc func(ctx context.Context, question string, options []string) (string, error)

// pendingResult holds the outcome of a single in-flight permission prompt so
// that concurrent callers waiting on the same domain can share the result.
type pendingResult struct {
	done chan struct{}
	err  error
}

// HTTP provides HTTP tools with permission gating.
type HTTP struct {
	store         *permissions.Store
	ask           AskFunc
	client        *http.Client
	pendingMu     sync.Mutex
	pendingDomain map[string]*pendingResult
}

// privateRanges are the CIDR blocks for private/loopback networks.
var privateRanges = func() []*net.IPNet {
	cidrs := []string{
		"127.0.0.0/8",
		"10.0.0.0/8",
		"172.16.0.0/12",
		"192.168.0.0/16",
		"169.254.0.0/16",
		"::1/128",
		"fc00::/7",
		"fe80::/10",
	}

	nets := make([]*net.IPNet, 0, len(cidrs))

	for _, cidr := range cidrs {
		_, ipNet, _ := net.ParseCIDR(cidr)
		nets = append(nets, ipNet)
	}

	return nets
}()

// isPrivateIP returns true if the given IP falls within any private/loopback range.
func isPrivateIP(ip net.IP) bool {
	for _, r := range privateRanges {
		if r.Contains(ip) {
			return true
		}
	}

	return false
}

// isPrivateHost returns true if the host resolves to a private or loopback IP.
func isPrivateHost(host string) bool {
	hostname := host
	if h, _, err := net.SplitHostPort(host); err == nil {
		hostname = h
	}

	ips, err := net.LookupIP(hostname)
	if err != nil {
		// If we can't resolve, err on the side of caution.
		return true
	}

	return slices.ContainsFunc(ips, isPrivateIP)
}

// safeTransport returns an *http.Transport with a custom DialContext that
// validates resolved IPs against private ranges at connection time. This
// prevents DNS rebinding attacks where a hostname resolves to a public IP
// during the permission check but to a private IP when the connection is made.
func safeTransport() *http.Transport {
	dialer := &net.Dialer{
		Timeout:   30 * time.Second,
		KeepAlive: 30 * time.Second,
	}

	return &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			host, port, err := net.SplitHostPort(addr)
			if err != nil {
				return nil, fmt.Errorf("http: invalid address %s: %w", addr, err)
			}

			ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
			if err != nil {
				return nil, fmt.Errorf("http: DNS lookup failed for %s: %w", host, err)
			}

			for _, ip := range ips {
				if isPrivateIP(ip.IP) {
					return nil, fmt.Errorf("http: connection to private address %s blocked", ip.IP)
				}
			}

			// Dial using the first resolved IP to prevent a second DNS lookup.
			return dialer.DialContext(ctx, network, net.JoinHostPort(ips[0].IP.String(), port))
		},
	}
}

// New creates an HTTP that checks the given permissions store for trusted
// domains and prompts the user via askFn when a domain is not yet trusted.
func New(store *permissions.Store, askFn AskFunc) *HTTP {
	h := &HTTP{
		store:         store,
		ask:           askFn,
		pendingDomain: make(map[string]*pendingResult),
	}

	h.client = &http.Client{
		Timeout:   60 * time.Second,
		Transport: safeTransport(),
		CheckRedirect: func(req *http.Request, _ []*http.Request) error {
			domain := req.URL.Hostname()
			if domain == "" {
				return fmt.Errorf("http: redirect target has no domain")
			}

			if isPrivateHost(req.URL.Host) {
				return fmt.Errorf("http: redirect to private/internal address %s is not allowed", req.URL.Host)
			}

			if !h.store.IsDomainTrusted(domain) {
				return fmt.Errorf("http: redirect to untrusted domain %s is not allowed", domain)
			}

			return nil
		},
	}

	return h
}

// Tools returns a ToolBox containing the HTTP tools.
func (h *HTTP) Tools() *toolbox.ToolBox {
	tb := toolbox.New()
	tb.Register(h.fetchTool())

	return tb
}

// checkPermission checks if a domain is trusted, prompting the user if not.
// Concurrent requests for the same domain coalesce into a single prompt.
func (h *HTTP) checkPermission(ctx context.Context, rawURL string) error {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("http: invalid URL: %w", err)
	}

	domain := parsed.Hostname()
	if domain == "" {
		return fmt.Errorf("http: could not extract domain from URL")
	}

	// Fast path: already trusted (no lock contention).
	if h.store.IsDomainTrusted(domain) {
		return nil
	}

	h.pendingMu.Lock()
	// Re-check after acquiring lock.
	if h.store.IsDomainTrusted(domain) {
		h.pendingMu.Unlock()

		return nil
	}

	// If a prompt is already in-flight for this domain, wait for its result.
	if pr, ok := h.pendingDomain[domain]; ok {
		h.pendingMu.Unlock()
		<-pr.done

		return pr.err
	}

	// We are the first — create a pending entry and release the lock.
	pr := &pendingResult{done: make(chan struct{})}
	h.pendingDomain[domain] = pr
	h.pendingMu.Unlock()

	// Ask the user (blocking).
	pr.err = h.askAndApproveDomain(ctx, domain)

	// Signal waiters and clean up.
	close(pr.done)
	h.pendingMu.Lock()
	delete(h.pendingDomain, domain)
	h.pendingMu.Unlock()

	return pr.err
}

// askAndApproveDomain prompts the user and trusts/approves the domain.
func (h *HTTP) askAndApproveDomain(ctx context.Context, domain string) error {
	resp, err := h.ask(ctx, fmt.Sprintf("Allow HTTP request to %s?", domain), []string{"yes", "trust", "no"})
	if err != nil {
		return fmt.Errorf("http: ask permission: %w", err)
	}

	switch strings.ToLower(resp) {
	case "trust":
		return h.store.TrustDomain(domain)
	case "yes":
		return nil
	default:
		return fmt.Errorf("http: access denied to %s", domain)
	}
}

// --- http_fetch ---

type fetchInput struct {
	URL     string            `json:"url"`
	Method  string            `json:"method"`
	Headers map[string]string `json:"headers"`
	Body    string            `json:"body"`
}

type fetchOutput struct {
	Status  int               `json:"status"`
	Headers map[string]string `json:"headers"`
	Body    string            `json:"body"`
}

// maxBodySize is the maximum response body size (1MB).
const maxBodySize = 1 << 20

func (h *HTTP) fetchTool() toolbox.Tool {
	return toolbox.Tool{
		Name:        "http_fetch",
		Description: "Make an HTTP request. Returns status, headers, and body (capped at 1MB).",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"url":{"type":"string","description":"The URL to fetch"},"method":{"type":"string","description":"HTTP method (default GET)"},"headers":{"type":"object","additionalProperties":{"type":"string"},"description":"Request headers"},"body":{"type":"string","description":"Request body"}},"required":["url"]}`),
		Handler:     h.handleFetch,
	}
}

func (h *HTTP) handleFetch(ctx context.Context, input json.RawMessage) (string, error) {
	var in fetchInput
	if err := json.Unmarshal(input, &in); err != nil {
		return "", fmt.Errorf("http_fetch: invalid input: %w", err)
	}

	if in.URL == "" {
		return "", fmt.Errorf("http_fetch: url is required")
	}

	if err := h.checkPermission(ctx, in.URL); err != nil {
		return "", err
	}

	method := in.Method
	if method == "" {
		method = http.MethodGet
	}

	var bodyReader io.Reader
	if in.Body != "" {
		bodyReader = strings.NewReader(in.Body)
	}

	req, err := http.NewRequestWithContext(ctx, method, in.URL, bodyReader)
	if err != nil {
		return "", fmt.Errorf("http_fetch: create request: %w", err)
	}

	for k, v := range in.Headers {
		req.Header.Set(k, v)
	}

	resp, err := h.client.Do(req) //nolint:gosec // URL is approved by user
	if err != nil {
		return "", fmt.Errorf("http_fetch: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck // best-effort close on read

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBodySize))
	if err != nil {
		return "", fmt.Errorf("http_fetch: read body: %w", err)
	}

	respHeaders := make(map[string]string, len(resp.Header))
	for k := range resp.Header {
		respHeaders[k] = resp.Header.Get(k)
	}

	out := fetchOutput{
		Status:  resp.StatusCode,
		Headers: respHeaders,
		Body:    string(body),
	}

	data, err := json.Marshal(out)
	if err != nil {
		return "", fmt.Errorf("http_fetch: marshal: %w", err)
	}

	return string(data), nil
}
