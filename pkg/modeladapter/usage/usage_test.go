package usage_test

import (
	"sync"
	"testing"

	"github.com/germanamz/shelly/pkg/modeladapter/usage"
	"github.com/stretchr/testify/assert"
)

func TestTokenCount_Total(t *testing.T) {
	tc := usage.TokenCount{InputTokens: 100, OutputTokens: 50}
	assert.Equal(t, 150, tc.Total())
}

func TestTokenCount_Total_Zero(t *testing.T) {
	tc := usage.TokenCount{}
	assert.Equal(t, 0, tc.Total())
}

func TestTracker_Add_And_Count(t *testing.T) {
	var tr usage.Tracker

	assert.Equal(t, 0, tr.Count())

	tr.Add(usage.TokenCount{InputTokens: 10, OutputTokens: 5})
	assert.Equal(t, 1, tr.Count())

	tr.Add(usage.TokenCount{InputTokens: 20, OutputTokens: 10})
	assert.Equal(t, 2, tr.Count())
}

func TestTracker_Last_Empty(t *testing.T) {
	var tr usage.Tracker

	tc, ok := tr.Last()
	assert.False(t, ok)
	assert.Equal(t, usage.TokenCount{}, tc)
}

func TestTracker_Last(t *testing.T) {
	var tr usage.Tracker

	tr.Add(usage.TokenCount{InputTokens: 10, OutputTokens: 5})
	tr.Add(usage.TokenCount{InputTokens: 20, OutputTokens: 10})

	tc, ok := tr.Last()
	assert.True(t, ok)
	assert.Equal(t, usage.TokenCount{InputTokens: 20, OutputTokens: 10}, tc)
}

func TestTracker_Total(t *testing.T) {
	var tr usage.Tracker

	tr.Add(usage.TokenCount{InputTokens: 10, OutputTokens: 5})
	tr.Add(usage.TokenCount{InputTokens: 20, OutputTokens: 10})

	total := tr.Total()
	assert.Equal(t, 30, total.InputTokens)
	assert.Equal(t, 15, total.OutputTokens)
	assert.Equal(t, 45, total.Total())
}

func TestTracker_Total_Empty(t *testing.T) {
	var tr usage.Tracker

	total := tr.Total()
	assert.Equal(t, usage.TokenCount{}, total)
}

func TestTracker_Reset(t *testing.T) {
	var tr usage.Tracker

	tr.Add(usage.TokenCount{InputTokens: 10, OutputTokens: 5})
	tr.Add(usage.TokenCount{InputTokens: 20, OutputTokens: 10})
	assert.Equal(t, 2, tr.Count())

	tr.Reset()
	assert.Equal(t, 0, tr.Count())

	_, ok := tr.Last()
	assert.False(t, ok)

	assert.Equal(t, usage.TokenCount{}, tr.Total())
}

func TestTokenCount_CacheSavings(t *testing.T) {
	tc := usage.TokenCount{
		InputTokens:              100,
		OutputTokens:             50,
		CacheCreationInputTokens: 50,
		CacheReadInputTokens:     200,
	}
	// 200 / (100 + 50 + 200) = 200/350 ≈ 0.5714
	assert.InDelta(t, 0.5714, tc.CacheSavings(), 0.001)
}

func TestTokenCount_CacheSavings_Zero(t *testing.T) {
	tc := usage.TokenCount{}
	assert.InDelta(t, 0, tc.CacheSavings(), 0.001)
}

func TestTokenCount_CacheSavings_NoCacheReads(t *testing.T) {
	tc := usage.TokenCount{
		InputTokens:              100,
		CacheCreationInputTokens: 50,
	}
	assert.InDelta(t, 0, tc.CacheSavings(), 0.001)
}

func TestTracker_Total_CacheFields(t *testing.T) {
	var tr usage.Tracker

	tr.Add(usage.TokenCount{
		InputTokens:              10,
		OutputTokens:             5,
		CacheCreationInputTokens: 20,
		CacheReadInputTokens:     30,
	})
	tr.Add(usage.TokenCount{
		InputTokens:              15,
		OutputTokens:             8,
		CacheCreationInputTokens: 10,
		CacheReadInputTokens:     50,
	})

	total := tr.Total()
	assert.Equal(t, 25, total.InputTokens)
	assert.Equal(t, 13, total.OutputTokens)
	assert.Equal(t, 30, total.CacheCreationInputTokens)
	assert.Equal(t, 80, total.CacheReadInputTokens)
}

func TestTracker_RingBuffer_Total_Preserved(t *testing.T) {
	var tr usage.Tracker

	// Add more entries than the ring buffer capacity (100).
	const n = 250
	for i := range n {
		tr.Add(usage.TokenCount{InputTokens: i + 1, OutputTokens: 1})
	}

	// Total must reflect all entries, not just the buffered ones.
	total := tr.Total()
	assert.Equal(t, n*(n+1)/2, total.InputTokens) // sum 1..250
	assert.Equal(t, n, total.OutputTokens)

	// Count returns total entries ever added.
	assert.Equal(t, n, tr.Count())
}

func TestTracker_RingBuffer_Last_After_Wrap(t *testing.T) {
	var tr usage.Tracker

	const n = 150
	for i := range n {
		tr.Add(usage.TokenCount{InputTokens: i})
	}

	last, ok := tr.Last()
	assert.True(t, ok)
	assert.Equal(t, n-1, last.InputTokens)
}

func TestTracker_Reset_After_RingWrap(t *testing.T) {
	var tr usage.Tracker

	for range 150 {
		tr.Add(usage.TokenCount{InputTokens: 10, OutputTokens: 5})
	}

	tr.Reset()
	assert.Equal(t, 0, tr.Count())
	assert.Equal(t, usage.TokenCount{}, tr.Total())

	_, ok := tr.Last()
	assert.False(t, ok)

	// Can add again after reset.
	tr.Add(usage.TokenCount{InputTokens: 42})
	last, ok := tr.Last()
	assert.True(t, ok)
	assert.Equal(t, 42, last.InputTokens)
	assert.Equal(t, 1, tr.Count())
	assert.Equal(t, 42, tr.Total().InputTokens)
}

func TestTracker_Concurrent_Add(t *testing.T) {
	var tr usage.Tracker

	const goroutines = 100

	var wg sync.WaitGroup
	wg.Add(goroutines)

	for range goroutines {
		go func() {
			defer wg.Done()
			tr.Add(usage.TokenCount{InputTokens: 1, OutputTokens: 1})
		}()
	}

	wg.Wait()

	assert.Equal(t, goroutines, tr.Count())

	total := tr.Total()
	assert.Equal(t, goroutines, total.InputTokens)
	assert.Equal(t, goroutines, total.OutputTokens)
}
