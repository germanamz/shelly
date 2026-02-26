package state

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetSetBasic(t *testing.T) {
	s := &Store{}

	s.Set("foo", "bar")

	v, ok := s.Get("foo")
	require.True(t, ok)
	assert.Equal(t, "bar", v)
}

func TestGetMissing(t *testing.T) {
	s := &Store{}

	_, ok := s.Get("missing")
	assert.False(t, ok)
}

func TestSetOverwrite(t *testing.T) {
	s := &Store{}

	s.Set("k", 1)
	s.Set("k", 2)

	v, ok := s.Get("k")
	require.True(t, ok)
	assert.Equal(t, 2, v)
}

func TestDelete(t *testing.T) {
	s := &Store{}

	s.Set("k", "v")
	s.Delete("k")

	_, ok := s.Get("k")
	assert.False(t, ok)
}

func TestDeleteNonexistent(t *testing.T) {
	s := &Store{}
	// Should not panic.
	s.Delete("nope")
}

func TestKeys(t *testing.T) {
	s := &Store{}

	s.Set("b", 1)
	s.Set("a", 2)
	s.Set("c", 3)

	keys := s.Keys()
	assert.Equal(t, []string{"a", "b", "c"}, keys)
}

func TestKeysEmpty(t *testing.T) {
	s := &Store{}
	assert.Empty(t, s.Keys())
}

func TestSnapshot(t *testing.T) {
	s := &Store{}

	s.Set("x", 10)
	s.Set("y", 20)

	snap := s.Snapshot()
	assert.Equal(t, map[string]any{"x": 10, "y": 20}, snap)

	// Mutating snapshot should not affect store.
	snap["z"] = 30
	_, ok := s.Get("z")
	assert.False(t, ok)
}

func TestGetDeepCopiesRawMessage(t *testing.T) {
	s := &Store{}

	original := json.RawMessage(`{"key":"value"}`)
	s.Set("raw", original)

	v, ok := s.Get("raw")
	require.True(t, ok)

	got := v.(json.RawMessage)
	// Mutate the returned copy.
	got[0] = 'X'

	// Original in store should be unchanged.
	v2, _ := s.Get("raw")
	assert.JSONEq(t, `{"key":"value"}`, string(v2.(json.RawMessage)))
}

func TestSnapshotDeepCopiesRawMessage(t *testing.T) {
	s := &Store{}

	original := json.RawMessage(`{"key":"value"}`)
	s.Set("raw", original)

	snap := s.Snapshot()
	got := snap["raw"].(json.RawMessage)
	// Mutate the snapshot copy.
	got[0] = 'X'

	// Original in store should be unchanged.
	v, _ := s.Get("raw")
	assert.JSONEq(t, `{"key":"value"}`, string(v.(json.RawMessage)))
}

func TestGetDeepCopiesByteSlice(t *testing.T) {
	s := &Store{}

	original := []byte("hello")
	s.Set("bytes", original)

	v, ok := s.Get("bytes")
	require.True(t, ok)

	got := v.([]byte)
	got[0] = 'X'

	// Original in store should be unchanged.
	v2, _ := s.Get("bytes")
	assert.Equal(t, []byte("hello"), v2.([]byte))
}

func TestWatchKeyExists(t *testing.T) {
	s := &Store{}
	s.Set("ready", "yes")

	v, err := s.Watch(context.Background(), "ready")
	require.NoError(t, err)
	assert.Equal(t, "yes", v)
}

func TestWatchBlocksUntilSet(t *testing.T) {
	s := &Store{}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	done := make(chan struct{})
	var result any
	var watchErr error

	go func() {
		result, watchErr = s.Watch(ctx, "later")
		close(done)
	}()

	time.Sleep(50 * time.Millisecond)
	s.Set("later", 42)

	<-done
	require.NoError(t, watchErr)
	assert.Equal(t, 42, result)
}

func TestWatchCancelledContext(t *testing.T) {
	s := &Store{}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately.

	_, err := s.Watch(ctx, "never")
	require.ErrorIs(t, err, context.Canceled)
}

func TestWatchTimeout(t *testing.T) {
	s := &Store{}

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_, err := s.Watch(ctx, "never")
	require.ErrorIs(t, err, context.DeadlineExceeded)
}

// --- Concurrency tests ---

func TestConcurrentReadWrite(t *testing.T) {
	s := &Store{}

	var wg sync.WaitGroup

	// Writers
	for i := range 50 {
		wg.Go(func() {
			s.Set("key", i)
		})
	}

	// Readers
	for range 50 {
		wg.Go(func() {
			s.Get("key")
			s.Keys()
			s.Snapshot()
		})
	}

	wg.Wait()

	// Store should have the key set.
	_, ok := s.Get("key")
	assert.True(t, ok)
}

func TestConcurrentSetDelete(t *testing.T) {
	s := &Store{}

	var wg sync.WaitGroup

	for i := range 100 {
		wg.Go(func() {
			if i%2 == 0 {
				s.Set("toggle", i)
			} else {
				s.Delete("toggle")
			}
		})
	}

	wg.Wait()
	// No race or panic — that's the assertion.
}

// --- Tool integration tests ---

func TestToolsGet(t *testing.T) {
	s := &Store{}
	s.Set("greeting", json.RawMessage(`"hello"`))

	tb := s.Tools("ns")
	tool, ok := tb.Get("ns_state_get")
	require.True(t, ok)

	result, err := tool.Handler(context.Background(), json.RawMessage(`{"key":"greeting"}`))
	require.NoError(t, err)
	assert.JSONEq(t, `"hello"`, result)
}

func TestToolsGetNotFound(t *testing.T) {
	s := &Store{}
	tb := s.Tools("ns")
	tool, _ := tb.Get("ns_state_get")

	_, err := tool.Handler(context.Background(), json.RawMessage(`{"key":"missing"}`))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "key not found")
}

func TestToolsSet(t *testing.T) {
	s := &Store{}
	tb := s.Tools("ns")
	tool, ok := tb.Get("ns_state_set")
	require.True(t, ok)

	result, err := tool.Handler(context.Background(), json.RawMessage(`{"key":"x","value":42}`))
	require.NoError(t, err)
	assert.Equal(t, "ok", result)

	v, ok := s.Get("x")
	require.True(t, ok)
	assert.JSONEq(t, `42`, string(v.(json.RawMessage)))
}

func TestToolsList(t *testing.T) {
	s := &Store{}
	s.Set("a", 1)
	s.Set("b", 2)

	tb := s.Tools("ns")
	tool, ok := tb.Get("ns_state_list")
	require.True(t, ok)

	result, err := tool.Handler(context.Background(), json.RawMessage(`{}`))
	require.NoError(t, err)
	assert.JSONEq(t, `["a","b"]`, result)
}

func TestToolsListEmpty(t *testing.T) {
	s := &Store{}
	tb := s.Tools("ns")
	tool, _ := tb.Get("ns_state_list")

	result, err := tool.Handler(context.Background(), json.RawMessage(`{}`))
	require.NoError(t, err)
	assert.JSONEq(t, `[]`, result)
}

func TestToolsNamespace(t *testing.T) {
	s := &Store{}
	tb := s.Tools("myapp")

	names := make(map[string]bool)
	for _, tool := range tb.Tools() {
		names[tool.Name] = true
	}

	assert.True(t, names["myapp_state_get"])
	assert.True(t, names["myapp_state_set"])
	assert.True(t, names["myapp_state_list"])
	assert.Len(t, names, 3)
}

func TestToolsInvalidInput(t *testing.T) {
	s := &Store{}
	tb := s.Tools("ns")

	t.Run("get invalid json", func(t *testing.T) {
		tool, _ := tb.Get("ns_state_get")
		_, err := tool.Handler(context.Background(), json.RawMessage(`not json`))
		require.Error(t, err)
	})

	t.Run("set invalid json", func(t *testing.T) {
		tool, _ := tb.Get("ns_state_set")
		_, err := tool.Handler(context.Background(), json.RawMessage(`not json`))
		require.Error(t, err)
	})
}
