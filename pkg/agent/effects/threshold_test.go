package effects

import (
	"testing"

	"github.com/germanamz/shelly/pkg/modeladapter/usage"
	"github.com/stretchr/testify/assert"
)

func TestExceedsThreshold_EstimateAbove(t *testing.T) {
	uc := &usageCompleter{}
	assert.True(t, exceedsThreshold(uc, 900, 1000, 0.8))
}

func TestExceedsThreshold_EstimateBelow(t *testing.T) {
	uc := &usageCompleter{}
	assert.False(t, exceedsThreshold(uc, 500, 1000, 0.8))
}

func TestExceedsThreshold_UsageAbove(t *testing.T) {
	uc := &usageCompleter{}
	uc.tracker.Add(usage.TokenCount{InputTokens: 900})
	assert.True(t, exceedsThreshold(uc, 0, 1000, 0.8))
}

func TestExceedsThreshold_UsageBelow(t *testing.T) {
	uc := &usageCompleter{}
	uc.tracker.Add(usage.TokenCount{InputTokens: 500})
	assert.False(t, exceedsThreshold(uc, 0, 1000, 0.8))
}

func TestExceedsThreshold_ZeroContextWindow(t *testing.T) {
	uc := &usageCompleter{}
	assert.False(t, exceedsThreshold(uc, 900, 0, 0.8))
}

func TestExceedsThreshold_ZeroThreshold(t *testing.T) {
	uc := &usageCompleter{}
	assert.False(t, exceedsThreshold(uc, 900, 1000, 0))
}

func TestExceedsThreshold_NoUsageReporter(t *testing.T) {
	sc := &sequenceCompleter{}
	assert.False(t, exceedsThreshold(sc, 0, 1000, 0.8))
}
