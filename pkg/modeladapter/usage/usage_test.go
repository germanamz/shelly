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
