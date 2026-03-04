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

// maxRecentEntries is the maximum number of recent entries kept in the ring
// buffer. Older entries are discarded but their totals are preserved in the
// running accumulator.
const maxRecentEntries = 100

// Tracker accumulates token usage across multiple LLM calls.
// It maintains a running total for O(1) aggregation and a bounded ring buffer
// of the most recent entries for Last(). It is safe for concurrent use.
type Tracker struct {
	mu      sync.Mutex
	total   TokenCount   // running accumulator
	entries []TokenCount // ring buffer, last maxRecentEntries entries
	head    int          // next write position in the ring buffer
	count   int          // total entries ever added
}

// Add records a token count entry.
func (t *Tracker) Add(tc TokenCount) {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.total.InputTokens += tc.InputTokens
	t.total.OutputTokens += tc.OutputTokens
	t.total.CacheCreationInputTokens += tc.CacheCreationInputTokens
	t.total.CacheReadInputTokens += tc.CacheReadInputTokens

	if len(t.entries) < maxRecentEntries {
		t.entries = append(t.entries, tc)
	} else {
		t.entries[t.head] = tc
	}
	t.head = (t.head + 1) % maxRecentEntries
	t.count++
}

// Last returns the most recent token count entry.
// The bool is false when the tracker has no entries.
func (t *Tracker) Last() (TokenCount, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.count == 0 {
		return TokenCount{}, false
	}

	idx := (t.head - 1 + maxRecentEntries) % maxRecentEntries
	return t.entries[idx], true
}

// Total returns the aggregate token count across all entries.
func (t *Tracker) Total() TokenCount {
	t.mu.Lock()
	defer t.mu.Unlock()

	return t.total
}

// Count returns the total number of entries ever added to the tracker.
func (t *Tracker) Count() int {
	t.mu.Lock()
	defer t.mu.Unlock()

	return t.count
}

// Reset clears all recorded entries and the running total.
func (t *Tracker) Reset() {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.entries = nil
	t.head = 0
	t.count = 0
	t.total = TokenCount{}
}
