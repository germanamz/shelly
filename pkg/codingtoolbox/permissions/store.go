// Package permissions provides a shared, thread-safe store for persisting
// permission grants. It manages a single JSON file containing approved
// filesystem directories and trusted CLI commands. Both the filesystem and
// exec tool packages share one Store to maintain a unified trust file.
package permissions

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// Store manages permission grants persisted to a JSON file.
type Store struct {
	mu       sync.RWMutex
	dirs     map[string]struct{}
	commands map[string]struct{}
	domains  map[string]struct{}
	filePath string
}

// fileFormat is the JSON structure written to disk.
type fileFormat struct {
	FsDirectories   []string `json:"fs_directories"`
	TrustedCommands []string `json:"trusted_commands"`
	TrustedDomains  []string `json:"trusted_domains,omitempty"`
}

// New creates a Store backed by the given file. Existing data is loaded
// immediately. Both the current object format and the legacy flat-array format
// (used by earlier versions of the filesystem tool) are supported on read.
func New(filePath string) (*Store, error) {
	abs, err := filepath.Abs(filePath)
	if err != nil {
		return nil, fmt.Errorf("permissions: resolve path: %w", err)
	}

	s := &Store{
		dirs:     make(map[string]struct{}),
		commands: make(map[string]struct{}),
		domains:  make(map[string]struct{}),
		filePath: abs,
	}

	if err := s.load(); err != nil {
		return nil, err
	}

	return s, nil
}

// IsDirApproved reports whether dir or any of its ancestors has been approved.
func (s *Store) IsDirApproved(dir string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	cur := dir
	for {
		if _, ok := s.dirs[cur]; ok {
			return true
		}

		parent := filepath.Dir(cur)
		if parent == cur {
			return false
		}

		cur = parent
	}
}

// ApproveDir marks dir as approved and persists the change.
func (s *Store) ApproveDir(dir string) error {
	s.mu.Lock()
	s.dirs[dir] = struct{}{}
	snap := s.snapshot()
	s.mu.Unlock()

	return s.persistSnapshot(snap)
}

// IsCommandTrusted reports whether a command has been trusted.
func (s *Store) IsCommandTrusted(cmd string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	_, ok := s.commands[cmd]

	return ok
}

// TrustCommand marks a command as trusted and persists the change.
func (s *Store) TrustCommand(cmd string) error {
	s.mu.Lock()
	s.commands[cmd] = struct{}{}
	snap := s.snapshot()
	s.mu.Unlock()

	return s.persistSnapshot(snap)
}

// IsDomainTrusted reports whether a domain has been trusted.
func (s *Store) IsDomainTrusted(domain string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	_, ok := s.domains[domain]

	return ok
}

// TrustDomain marks a domain as trusted and persists the change.
func (s *Store) TrustDomain(domain string) error {
	s.mu.Lock()
	s.domains[domain] = struct{}{}
	snap := s.snapshot()
	s.mu.Unlock()

	return s.persistSnapshot(snap)
}

// --- persistence ---

func (s *Store) load() error {
	data, err := os.ReadFile(s.filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}

		return fmt.Errorf("permissions: read file: %w", err)
	}

	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 {
		return nil
	}

	// Legacy format: a flat JSON array of directory strings.
	if trimmed[0] == '[' {
		var dirs []string
		if err := json.Unmarshal(trimmed, &dirs); err != nil {
			return fmt.Errorf("permissions: parse legacy file: %w", err)
		}

		for _, d := range dirs {
			s.dirs[d] = struct{}{}
		}

		return nil
	}

	// Current format: object with typed fields.
	var ff fileFormat
	if err := json.Unmarshal(trimmed, &ff); err != nil {
		return fmt.Errorf("permissions: parse file: %w", err)
	}

	for _, d := range ff.FsDirectories {
		s.dirs[d] = struct{}{}
	}

	for _, c := range ff.TrustedCommands {
		s.commands[c] = struct{}{}
	}

	for _, d := range ff.TrustedDomains {
		s.domains[d] = struct{}{}
	}

	return nil
}

// snapshot returns a copy of the current permission data. Must be called
// while s.mu is held.
func (s *Store) snapshot() fileFormat {
	ff := fileFormat{
		FsDirectories:   make([]string, 0, len(s.dirs)),
		TrustedCommands: make([]string, 0, len(s.commands)),
		TrustedDomains:  make([]string, 0, len(s.domains)),
	}

	for d := range s.dirs {
		ff.FsDirectories = append(ff.FsDirectories, d)
	}

	for c := range s.commands {
		ff.TrustedCommands = append(ff.TrustedCommands, c)
	}

	for d := range s.domains {
		ff.TrustedDomains = append(ff.TrustedDomains, d)
	}

	return ff
}

// persistSnapshot writes the given snapshot to disk. It must be called
// outside the lock so that blocking I/O does not hold the mutex.
func (s *Store) persistSnapshot(ff fileFormat) error {
	data, err := json.MarshalIndent(ff, "", "  ")
	if err != nil {
		return fmt.Errorf("permissions: marshal: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(s.filePath), 0o750); err != nil {
		return fmt.Errorf("permissions: create dir: %w", err)
	}

	tmp, err := os.CreateTemp(filepath.Dir(s.filePath), ".perms-*.tmp")
	if err != nil {
		return fmt.Errorf("permissions: create temp file: %w", err)
	}
	tmpName := tmp.Name()

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName) //nolint:gosec // tmpName comes from os.CreateTemp in a known directory
		return fmt.Errorf("permissions: write temp file: %w", err)
	}

	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName) //nolint:gosec // tmpName comes from os.CreateTemp in a known directory
		return fmt.Errorf("permissions: close temp file: %w", err)
	}

	if err := os.Rename(tmpName, s.filePath); err != nil { //nolint:gosec // tmpName comes from os.CreateTemp in a known directory
		_ = os.Remove(tmpName) //nolint:gosec // tmpName comes from os.CreateTemp in a known directory
		return fmt.Errorf("permissions: rename temp file: %w", err)
	}

	return nil
}
