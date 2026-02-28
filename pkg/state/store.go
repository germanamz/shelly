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
	"sort"
	"sync"

	"github.com/germanamz/shelly/pkg/tools/toolbox"
)

// Store is a thread-safe key-value store. The zero value is ready to use.
type Store struct {
	mu     sync.RWMutex
	once   sync.Once
	signal chan struct{}
	data   map[string]any
}

// init ensures internal structures are allocated.
func (s *Store) init() {
	s.once.Do(func() {
		s.data = make(map[string]any)
		s.signal = make(chan struct{})
	})
}

// Get returns the value for key and whether it was found.
// If the value is a json.RawMessage or []byte, a deep copy is returned
// to prevent callers from mutating the stored byte slice.
func (s *Store) Get(key string) (any, bool) {
	s.init()
	s.mu.RLock()
	defer s.mu.RUnlock()

	v, ok := s.data[key]
	if !ok {
		return v, false
	}

	return copyValue(v), true
}

// Set stores a value under key and notifies any goroutines blocked in Watch.
func (s *Store) Set(key string, value any) {
	s.init()
	s.mu.Lock()
	defer s.mu.Unlock()

	s.data[key] = value
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

// Snapshot returns a copy of the entire store. Values that are
// json.RawMessage or []byte are deep-copied to prevent aliasing.
func (s *Store) Snapshot() map[string]any {
	s.init()
	s.mu.RLock()
	defer s.mu.RUnlock()

	cp := make(map[string]any, len(s.data))
	for k, v := range s.data {
		cp[k] = copyValue(v)
	}

	return cp
}

// copyValue returns a deep copy of v if it is a json.RawMessage or []byte,
// otherwise it returns v unchanged. Only json.RawMessage and []byte are
// deep-copied; all other types (including slices, maps, and pointers) are
// returned as shared references. Callers storing mutable aggregate types
// should be aware that mutations will affect the stored value.
func copyValue(v any) any {
	switch raw := v.(type) {
	case json.RawMessage:
		cp := make(json.RawMessage, len(raw))
		copy(cp, raw)
		return cp
	case []byte:
		cp := make([]byte, len(raw))
		copy(cp, raw)
		return cp
	default:
		return v
	}
}

// Watch blocks until key exists in the store or ctx is cancelled.
// It returns the value when found, or an error if the context is done.
func (s *Store) Watch(ctx context.Context, key string) (any, error) {
	s.init()

	for {
		s.mu.RLock()
		v, ok := s.data[key]
		sig := s.signal
		s.mu.RUnlock()

		if ok {
			return copyValue(v), nil
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

	b, err := json.Marshal(v)
	if err != nil {
		return "", fmt.Errorf("failed to encode value: %w", err)
	}

	return string(b), nil
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
