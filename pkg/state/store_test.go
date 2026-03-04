package state

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetSetBasic(t *testing.T) {
	s := &Store{}

	s.Set("foo", json.RawMessage(`"bar"`))

	v, ok := s.Get("foo")
	require.True(t, ok)
	assert.JSONEq(t, `"bar"`, string(v))
}

func TestGetMissing(t *testing.T) {
	s := &Store{}

	_, ok := s.Get("missing")
	assert.False(t, ok)
}

func TestSetOverwrite(t *testing.T) {
	s := &Store{}

	s.Set("k", json.RawMessage(`1`))
	s.Set("k", json.RawMessage(`2`))

	v, ok := s.Get("k")
	require.True(t, ok)
	assert.JSONEq(t, `2`, string(v))
}

func TestDelete(t *testing.T) {
	s := &Store{}

	s.Set("k", json.RawMessage(`"v"`))
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

	s.Set("b", json.RawMessage(`1`))
	s.Set("a", json.RawMessage(`2`))
	s.Set("c", json.RawMessage(`3`))

	keys := s.Keys()
	assert.Equal(t, []string{"a", "b", "c"}, keys)
}

func TestKeysEmpty(t *testing.T) {
	s := &Store{}
	assert.Empty(t, s.Keys())
}

func TestSnapshot(t *testing.T) {
	s := &Store{}

	s.Set("x", json.RawMessage(`10`))
	s.Set("y", json.RawMessage(`20`))

	snap := s.Snapshot()
	assert.JSONEq(t, `10`, string(snap["x"]))
	assert.JSONEq(t, `20`, string(snap["y"]))
	assert.Len(t, snap, 2)

	// Mutating snapshot should not affect store.
	snap["z"] = json.RawMessage(`30`)
	_, ok := s.Get("z")
	assert.False(t, ok)
}

func TestGetDeepCopiesRawMessage(t *testing.T) {
	s := &Store{}

	original := json.RawMessage(`{"key":"value"}`)
	s.Set("raw", original)

	v, ok := s.Get("raw")
	require.True(t, ok)

	// Mutate the returned copy.
	v[0] = 'X'

	// Original in store should be unchanged.
	v2, _ := s.Get("raw")
	assert.JSONEq(t, `{"key":"value"}`, string(v2))
}

func TestSetDeepCopiesInput(t *testing.T) {
	s := &Store{}

	original := json.RawMessage(`{"key":"value"}`)
	s.Set("raw", original)

	// Mutate the original slice after Set.
	original[0] = 'X'

	// Stored value should be unchanged.
	v, _ := s.Get("raw")
	assert.JSONEq(t, `{"key":"value"}`, string(v))
}

func TestSnapshotDeepCopiesRawMessage(t *testing.T) {
	s := &Store{}

	original := json.RawMessage(`{"key":"value"}`)
	s.Set("raw", original)

	snap := s.Snapshot()
	got := snap["raw"]
	// Mutate the snapshot copy.
	got[0] = 'X'

	// Original in store should be unchanged.
	v, _ := s.Get("raw")
	assert.JSONEq(t, `{"key":"value"}`, string(v))
}

func TestWatchKeyExists(t *testing.T) {
	s := &Store{}
	s.Set("ready", json.RawMessage(`"yes"`))

	v, err := s.Watch(context.Background(), "ready")
	require.NoError(t, err)
	assert.JSONEq(t, `"yes"`, string(v))
}

func TestWatchBlocksUntilSet(t *testing.T) {
	s := &Store{}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	done := make(chan struct{})
	var result json.RawMessage
	var watchErr error

	go func() {
		result, watchErr = s.Watch(ctx, "later")
		close(done)
	}()

	time.Sleep(50 * time.Millisecond)
	s.Set("later", json.RawMessage(`42`))

	<-done
	require.NoError(t, watchErr)
	assert.JSONEq(t, `42`, string(result))
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
			s.Set("key", json.RawMessage(fmt.Sprintf(`%d`, i)))
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
				s.Set("toggle", json.RawMessage(fmt.Sprintf(`%d`, i)))
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
	assert.JSONEq(t, `42`, string(v))
}

func TestToolsList(t *testing.T) {
	s := &Store{}
	s.Set("a", json.RawMessage(`1`))
	s.Set("b", json.RawMessage(`2`))

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
