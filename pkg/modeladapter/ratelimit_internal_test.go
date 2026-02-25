package modeladapter

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestPruneWindow_ReleasesBackingArray(t *testing.T) {
	r := &RateLimitedCompleter{
		nowFunc: time.Now,
	}

	now := time.Now()

	// Add many entries with old timestamps so they will be pruned.
	const n = 1000
	for i := range n {
		r.window = append(r.window, tokenEntry{
			timestamp:    now.Add(-2 * time.Minute).Add(time.Duration(i) * time.Millisecond),
			inputTokens:  10,
			outputTokens: 5,
		})
	}

	// Add one recent entry that should survive pruning.
	r.window = append(r.window, tokenEntry{
		timestamp:    now,
		inputTokens:  10,
		outputTokens: 5,
	})

	capBefore := cap(r.window)
	assert.Greater(t, capBefore, n, "backing array should be large before pruning")

	r.pruneWindow(now)

	assert.Len(t, r.window, 1, "only the recent entry should remain")
	assert.Less(t, cap(r.window), capBefore, "backing array capacity should shrink after pruning")
}
