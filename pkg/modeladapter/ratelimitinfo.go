package modeladapter

import (
	"net/http"
	"strconv"
	"time"
)

// RateLimitInfo holds rate limit state parsed from provider response headers.
type RateLimitInfo struct {
	RemainingRequests int
	RemainingTokens   int
	RequestsReset     time.Time
	TokensReset       time.Time
}

// RateLimitInfoReporter provides the most recently observed rate limit info
// from a provider's response headers.
type RateLimitInfoReporter interface {
	LastRateLimitInfo() *RateLimitInfo
}

// RateLimitHeaderParser extracts rate limit info from HTTP response headers.
// It receives the current time so callers can control the clock in tests.
type RateLimitHeaderParser func(h http.Header, now time.Time) *RateLimitInfo

// ParseAnthropicRateLimitHeaders parses Anthropic-specific rate limit headers.
// Headers: anthropic-ratelimit-{requests,tokens}-{remaining,reset}.
func ParseAnthropicRateLimitHeaders(h http.Header, now time.Time) *RateLimitInfo {
	reqRemaining := h.Get("anthropic-ratelimit-requests-remaining")
	tokRemaining := h.Get("anthropic-ratelimit-tokens-remaining")
	reqReset := h.Get("anthropic-ratelimit-requests-reset")
	tokReset := h.Get("anthropic-ratelimit-tokens-reset")

	if reqRemaining == "" && tokRemaining == "" {
		return nil
	}

	info := &RateLimitInfo{}
	if v, err := strconv.Atoi(reqRemaining); err == nil {
		info.RemainingRequests = v
	}
	if v, err := strconv.Atoi(tokRemaining); err == nil {
		info.RemainingTokens = v
	}
	info.RequestsReset = parseResetTime(reqReset, now)
	info.TokensReset = parseResetTime(tokReset, now)

	return info
}

// ParseOpenAIRateLimitHeaders parses OpenAI-compatible rate limit headers.
// Also used by Grok since it follows the same convention.
// Headers: x-ratelimit-remaining-{requests,tokens}, x-ratelimit-reset-{requests,tokens}.
func ParseOpenAIRateLimitHeaders(h http.Header, now time.Time) *RateLimitInfo {
	reqRemaining := h.Get("x-ratelimit-remaining-requests")
	tokRemaining := h.Get("x-ratelimit-remaining-tokens")
	reqReset := h.Get("x-ratelimit-reset-requests")
	tokReset := h.Get("x-ratelimit-reset-tokens")

	if reqRemaining == "" && tokRemaining == "" {
		return nil
	}

	info := &RateLimitInfo{}
	if v, err := strconv.Atoi(reqRemaining); err == nil {
		info.RemainingRequests = v
	}
	if v, err := strconv.Atoi(tokRemaining); err == nil {
		info.RemainingTokens = v
	}
	info.RequestsReset = parseResetTime(reqReset, now)
	info.TokensReset = parseResetTime(tokReset, now)

	return info
}

// parseResetTime tries RFC3339 first, then a Go duration string (e.g. "6s", "1m30s")
// relative to now.
func parseResetTime(val string, now time.Time) time.Time {
	if val == "" {
		return time.Time{}
	}
	if t, err := time.Parse(time.RFC3339, val); err == nil {
		return t
	}
	if d, err := time.ParseDuration(val); err == nil {
		return now.Add(d)
	}
	return time.Time{}
}
