package effects

import "github.com/germanamz/shelly/pkg/modeladapter"

// exceedsThreshold returns true when the estimated or measured token count
// has reached or exceeded contextWindow * threshold.
func exceedsThreshold(completer modeladapter.Completer, estimatedTokens, contextWindow int, threshold float64) bool {
	if contextWindow <= 0 || threshold <= 0 {
		return false
	}

	limit := int(float64(contextWindow) * threshold)

	if estimatedTokens > 0 && estimatedTokens >= limit {
		return true
	}

	reporter, ok := completer.(modeladapter.UsageReporter)
	if !ok {
		return false
	}

	last, ok := reporter.UsageTracker().Last()
	if !ok {
		return false
	}

	return last.InputTokens >= limit
}
