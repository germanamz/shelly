package codingtoolbox

// LimitedBuffer is a byte buffer that silently discards writes beyond a cap.
type LimitedBuffer struct {
	buf     []byte
	maxSize int
}

// NewLimitedBuffer creates a LimitedBuffer with the given maximum size.
func NewLimitedBuffer(maxSize int) *LimitedBuffer {
	return &LimitedBuffer{maxSize: maxSize}
}

// Write appends p to the buffer. Bytes beyond the cap are silently discarded.
// The returned count is always len(p) so callers never see a short write.
func (b *LimitedBuffer) Write(p []byte) (int, error) {
	remaining := b.maxSize - len(b.buf)
	if remaining > 0 {
		if len(p) > remaining {
			b.buf = append(b.buf, p[:remaining]...)
		} else {
			b.buf = append(b.buf, p...)
		}
	}

	return len(p), nil
}

// Len returns the number of bytes stored in the buffer.
func (b *LimitedBuffer) Len() int { return len(b.buf) }

// String returns the buffered content as a string.
func (b *LimitedBuffer) String() string { return string(b.buf) }
