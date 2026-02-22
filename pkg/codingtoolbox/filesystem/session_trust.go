package filesystem

import (
	"context"
	"sync"
)

// SessionTrust tracks whether the user has opted to trust all file changes for
// the current session. It is safe for concurrent use.
type SessionTrust struct {
	mu      sync.RWMutex
	trusted bool
}

// IsTrusted reports whether the session is trusted.
func (st *SessionTrust) IsTrusted() bool {
	st.mu.RLock()
	defer st.mu.RUnlock()

	return st.trusted
}

// Trust marks the session as trusted. Subsequent file changes will be displayed
// but not block for approval.
func (st *SessionTrust) Trust() {
	st.mu.Lock()
	defer st.mu.Unlock()

	st.trusted = true
}

type sessionTrustKey struct{}

// WithSessionTrust returns a new context carrying the given SessionTrust.
func WithSessionTrust(ctx context.Context, st *SessionTrust) context.Context {
	return context.WithValue(ctx, sessionTrustKey{}, st)
}

func sessionTrustFromContext(ctx context.Context) *SessionTrust {
	st, _ := ctx.Value(sessionTrustKey{}).(*SessionTrust)
	return st
}
