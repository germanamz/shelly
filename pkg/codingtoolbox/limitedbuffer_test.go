package codingtoolbox_test

import (
	"testing"

	"github.com/germanamz/shelly/pkg/codingtoolbox"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLimitedBuffer_WritesWithinCap(t *testing.T) {
	buf := codingtoolbox.NewLimitedBuffer(100)

	n, err := buf.Write([]byte("hello"))
	require.NoError(t, err)
	assert.Equal(t, 5, n)
	assert.Equal(t, 5, buf.Len())
	assert.Equal(t, "hello", buf.String())
}

func TestLimitedBuffer_TruncatesAtCap(t *testing.T) {
	buf := codingtoolbox.NewLimitedBuffer(10)

	n, err := buf.Write([]byte("hello world!")) // 12 bytes
	require.NoError(t, err)
	assert.Equal(t, 12, n, "Write always reports full len(p)")
	assert.Equal(t, 10, buf.Len())
	assert.Equal(t, "hello worl", buf.String())
}

func TestLimitedBuffer_MultipleWrites(t *testing.T) {
	buf := codingtoolbox.NewLimitedBuffer(10)

	_, _ = buf.Write([]byte("abcde"))
	_, _ = buf.Write([]byte("fghij"))
	_, _ = buf.Write([]byte("klmno"))

	assert.Equal(t, 10, buf.Len())
	assert.Equal(t, "abcdefghij", buf.String())
}

func TestLimitedBuffer_ZeroCap(t *testing.T) {
	buf := codingtoolbox.NewLimitedBuffer(0)

	n, err := buf.Write([]byte("data"))
	require.NoError(t, err)
	assert.Equal(t, 4, n)
	assert.Equal(t, 0, buf.Len())
	assert.Empty(t, buf.String())
}
