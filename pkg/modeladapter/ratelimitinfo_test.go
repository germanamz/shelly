package modeladapter_test

import (
	"net/http"
	"testing"
	"time"

	"github.com/germanamz/shelly/pkg/modeladapter"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseAnthropicRateLimitHeaders_AllHeaders(t *testing.T) {
	now := time.Date(2025, 1, 15, 12, 0, 0, 0, time.UTC)
	reset := now.Add(30 * time.Second)

	h := http.Header{}
	h.Set("anthropic-ratelimit-requests-remaining", "5")
	h.Set("anthropic-ratelimit-tokens-remaining", "1000")
	h.Set("anthropic-ratelimit-requests-reset", reset.Format(time.RFC3339))
	h.Set("anthropic-ratelimit-tokens-reset", reset.Format(time.RFC3339))

	info := modeladapter.ParseAnthropicRateLimitHeaders(h, now)
	require.NotNil(t, info)
	assert.Equal(t, 5, info.RemainingRequests)
	assert.Equal(t, 1000, info.RemainingTokens)
	assert.Equal(t, reset, info.RequestsReset)
	assert.Equal(t, reset, info.TokensReset)
}

func TestParseAnthropicRateLimitHeaders_PartialHeaders(t *testing.T) {
	now := time.Date(2025, 1, 15, 12, 0, 0, 0, time.UTC)

	h := http.Header{}
	h.Set("anthropic-ratelimit-requests-remaining", "3")

	info := modeladapter.ParseAnthropicRateLimitHeaders(h, now)
	require.NotNil(t, info)
	assert.Equal(t, 3, info.RemainingRequests)
	assert.Equal(t, 0, info.RemainingTokens)
	assert.True(t, info.TokensReset.IsZero())
}

func TestParseAnthropicRateLimitHeaders_NoHeaders(t *testing.T) {
	info := modeladapter.ParseAnthropicRateLimitHeaders(http.Header{}, time.Now())
	assert.Nil(t, info)
}

func TestParseAnthropicRateLimitHeaders_DurationReset(t *testing.T) {
	now := time.Date(2025, 1, 15, 12, 0, 0, 0, time.UTC)

	h := http.Header{}
	h.Set("anthropic-ratelimit-requests-remaining", "2")
	h.Set("anthropic-ratelimit-requests-reset", "30s")

	info := modeladapter.ParseAnthropicRateLimitHeaders(h, now)
	require.NotNil(t, info)
	assert.Equal(t, now.Add(30*time.Second), info.RequestsReset)
}

func TestParseOpenAIRateLimitHeaders_AllHeaders(t *testing.T) {
	now := time.Date(2025, 1, 15, 12, 0, 0, 0, time.UTC)
	reset := now.Add(60 * time.Second)

	h := http.Header{}
	h.Set("x-ratelimit-remaining-requests", "10")
	h.Set("x-ratelimit-remaining-tokens", "5000")
	h.Set("x-ratelimit-reset-requests", reset.Format(time.RFC3339))
	h.Set("x-ratelimit-reset-tokens", reset.Format(time.RFC3339))

	info := modeladapter.ParseOpenAIRateLimitHeaders(h, now)
	require.NotNil(t, info)
	assert.Equal(t, 10, info.RemainingRequests)
	assert.Equal(t, 5000, info.RemainingTokens)
	assert.Equal(t, reset, info.RequestsReset)
	assert.Equal(t, reset, info.TokensReset)
}

func TestParseOpenAIRateLimitHeaders_PartialHeaders(t *testing.T) {
	now := time.Date(2025, 1, 15, 12, 0, 0, 0, time.UTC)

	h := http.Header{}
	h.Set("x-ratelimit-remaining-tokens", "100")

	info := modeladapter.ParseOpenAIRateLimitHeaders(h, now)
	require.NotNil(t, info)
	assert.Equal(t, 0, info.RemainingRequests)
	assert.Equal(t, 100, info.RemainingTokens)
}

func TestParseOpenAIRateLimitHeaders_NoHeaders(t *testing.T) {
	info := modeladapter.ParseOpenAIRateLimitHeaders(http.Header{}, time.Now())
	assert.Nil(t, info)
}

func TestParseOpenAIRateLimitHeaders_DurationReset(t *testing.T) {
	now := time.Date(2025, 1, 15, 12, 0, 0, 0, time.UTC)

	h := http.Header{}
	h.Set("x-ratelimit-remaining-requests", "1")
	h.Set("x-ratelimit-reset-requests", "1m30s")

	info := modeladapter.ParseOpenAIRateLimitHeaders(h, now)
	require.NotNil(t, info)
	assert.Equal(t, now.Add(90*time.Second), info.RequestsReset)
}
