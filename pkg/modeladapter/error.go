package modeladapter

import (
	"fmt"
	"net/http"
	"strconv"
	"time"
)

// RateLimitError is returned when the API responds with HTTP 429 (Too Many Requests).
// It carries an optional RetryAfter duration parsed from the Retry-After header.
type RateLimitError struct {
	RetryAfter time.Duration
	Body       string
}

func (e *RateLimitError) Error() string {
	if e.RetryAfter > 0 {
		return fmt.Sprintf("rate limited (retry after %s): %s", e.RetryAfter, e.Body)
	}
	return fmt.Sprintf("rate limited: %s", e.Body)
}

// ParseRetryAfter parses the Retry-After header value as either seconds (integer)
// or an HTTP-date (RFC 7231). Returns zero if unparseable or if the date is in the past.
func ParseRetryAfter(val string) time.Duration {
	if val == "" {
		return 0
	}
	if secs, err := strconv.Atoi(val); err == nil {
		return time.Duration(secs) * time.Second
	}
	if t, err := http.ParseTime(val); err == nil {
		d := time.Until(t)
		if d > 0 {
			return d
		}
		return 0
	}
	return 0
}
