// Package state provides a thread-safe key-value store for inter-agent
// structured data sharing (blackboard pattern). The Store supports blocking
// Watch for key availability and can expose its operations as toolbox.Tool
// entries so agents interact with shared state through their normal tool-calling
// loop.
package state

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"sort"
	"sync"

	"github.com/germanamz/shelly/pkg/tools/toolbox"
)

// Store is a thread-safe key-value store. The zero value is ready to use.
// All values are stored as json.RawMessage to guarantee deep-copy safety and
// JSON compatibility.
type Store struct {
	mu     sync.RWMutex
	once   sync.Once
	signal chan struct{}
	data   map[string]json.RawMessage
}

// init ensures internal structures are allocated.
func (s *Store) init() {
	s.once.Do(func() {
		s.data = make(map[string]json.RawMessage)
		s.signal = make(chan struct{})
	})
}

// Get returns the value for key and whether it was found.
// The returned json.RawMessage is a deep copy to prevent callers from
// mutating the stored byte slice.
func (s *Store) Get(key string) (json.RawMessage, bool) {
	s.init()
	s.mu.RLock()
	defer s.mu.RUnlock()

	v, ok := s.data[key]
	if !ok {
		return nil, false
	}

	return slices.Clone(v), true
}

// Set stores a value under key and notifies any goroutines blocked in Watch.
// The value is deep-copied to prevent callers from mutating stored data.
func (s *Store) Set(key string, value json.RawMessage) {
	s.init()
	s.mu.Lock()
	defer s.mu.Unlock()

	s.data[key] = slices.Clone(value)
	close(s.signal)
	s.signal = make(chan struct{})
}

// Delete removes a key and notifies any goroutines blocked in Watch.
func (s *Store) Delete(key string) {
	s.init()
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.data, key)
	close(s.signal)
	s.signal = make(chan struct{})
}

// Keys returns a sorted slice of all keys in the store.
func (s *Store) Keys() []string {
	s.init()
	s.mu.RLock()
	defer s.mu.RUnlock()

	keys := make([]string, 0, len(s.data))
	for k := range s.data {
		keys = append(keys, k)
	}

	sort.Strings(keys)

	return keys
}

// Snapshot returns a deep copy of the entire store.
func (s *Store) Snapshot() map[string]json.RawMessage {
	s.init()
	s.mu.RLock()
	defer s.mu.RUnlock()

	cp := make(map[string]json.RawMessage, len(s.data))
	for k, v := range s.data {
		cp[k] = slices.Clone(v)
	}

	return cp
}

// Watch blocks until key exists in the store or ctx is cancelled.
// It returns a deep copy of the value when found, or an error if the context is done.
func (s *Store) Watch(ctx context.Context, key string) (json.RawMessage, error) {
	s.init()

	for {
		s.mu.RLock()
		v, ok := s.data[key]
		sig := s.signal
		s.mu.RUnlock()

		if ok {
			return slices.Clone(v), nil
		}

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-sig:
		}
	}
}

// --- Tool integration ---

// Tools returns a ToolBox with get, set, and list tools namespaced under the
// given prefix. Tool names are: {namespace}_state_get, {namespace}_state_set,
// {namespace}_state_list. Values are stored and retrieved as json.RawMessage.
func (s *Store) Tools(namespace string) *toolbox.ToolBox {
	tb := toolbox.New()

	tb.Register(
		toolbox.Tool{
			Name:        fmt.Sprintf("%s_state_get", namespace),
			Description: "Get a value from the shared state store by key.",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"key":{"type":"string"}},"required":["key"]}`),
			Handler:     s.handleGet,
		},
		toolbox.Tool{
			Name:        fmt.Sprintf("%s_state_set", namespace),
			Description: "Set a value in the shared state store.",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"key":{"type":"string"},"value":{}},"required":["key","value"]}`),
			Handler:     s.handleSet,
		},
		toolbox.Tool{
			Name:        fmt.Sprintf("%s_state_list", namespace),
			Description: "List all keys in the shared state store.",
			InputSchema: json.RawMessage(`{"type":"object"}`),
			Handler:     s.handleList,
		},
	)

	return tb
}

type getInput struct {
	Key string `json:"key"`
}

type setInput struct {
	Key   string          `json:"key"`
	Value json.RawMessage `json:"value"`
}

func (s *Store) handleGet(_ context.Context, input json.RawMessage) (string, error) {
	var in getInput
	if err := json.Unmarshal(input, &in); err != nil {
		return "", fmt.Errorf("invalid input: %w", err)
	}

	v, ok := s.Get(in.Key)
	if !ok {
		return "", errors.New("key not found")
	}

	return string(v), nil
}

func (s *Store) handleSet(_ context.Context, input json.RawMessage) (string, error) {
	var in setInput
	if err := json.Unmarshal(input, &in); err != nil {
		return "", fmt.Errorf("invalid input: %w", err)
	}

	s.Set(in.Key, in.Value)

	return "ok", nil
}

func (s *Store) handleList(_ context.Context, _ json.RawMessage) (string, error) {
	keys := s.Keys()

	b, err := json.Marshal(keys)
	if err != nil {
		return "", fmt.Errorf("failed to encode keys: %w", err)
	}

	return string(b), nil
}
