package usage

import "sync"

// TokenCount holds input and output token counts for a single LLM call.
type TokenCount struct {
	InputTokens              int
	OutputTokens             int
	CacheCreationInputTokens int
	CacheReadInputTokens     int
}

// Total returns the sum of input and output tokens.
func (tc TokenCount) Total() int {
	return tc.InputTokens + tc.OutputTokens
}

// CacheSavings returns the ratio of cache-read tokens to total input tokens
// (cache_read + cache_creation + input). Returns 0 if there are no input tokens.
func (tc TokenCount) CacheSavings() float64 {
	total := tc.InputTokens + tc.CacheCreationInputTokens + tc.CacheReadInputTokens
	if total == 0 {
		return 0
	}
	return float64(tc.CacheReadInputTokens) / float64(total)
}

// Tracker accumulates token usage across multiple LLM calls.
// It is safe for concurrent use.
type Tracker struct {
	mu      sync.Mutex
	entries []TokenCount
}

// Add records a token count entry.
func (t *Tracker) Add(tc TokenCount) {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.entries = append(t.entries, tc)
}

// Last returns the most recent token count entry.
// The bool is false when the tracker has no entries.
func (t *Tracker) Last() (TokenCount, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if len(t.entries) == 0 {
		return TokenCount{}, false
	}

	return t.entries[len(t.entries)-1], true
}

// Total returns the aggregate token count across all entries.
func (t *Tracker) Total() TokenCount {
	t.mu.Lock()
	defer t.mu.Unlock()

	var total TokenCount
	for _, e := range t.entries {
		total.InputTokens += e.InputTokens
		total.OutputTokens += e.OutputTokens
		total.CacheCreationInputTokens += e.CacheCreationInputTokens
		total.CacheReadInputTokens += e.CacheReadInputTokens
	}

	return total
}

// Count returns the number of recorded entries.
func (t *Tracker) Count() int {
	t.mu.Lock()
	defer t.mu.Unlock()

	return len(t.entries)
}

// Reset clears all recorded entries.
func (t *Tracker) Reset() {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.entries = nil
}
