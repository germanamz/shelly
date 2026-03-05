package sessions

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/germanamz/shelly/pkg/chats/message"
)

// ProviderMeta holds provider identification for a session.
type ProviderMeta struct {
	Kind  string `json:"kind"`
	Model string `json:"model"`
}

// SessionInfo contains metadata about a persisted session.
type SessionInfo struct {
	ID        string       `json:"id"`
	Agent     string       `json:"agent"`
	Provider  ProviderMeta `json:"provider"`
	CreatedAt time.Time    `json:"created_at"`
	UpdatedAt time.Time    `json:"updated_at"`
	Preview   string       `json:"preview"`
	MsgCount  int          `json:"msg_count"`
}

// persistedFile is the on-disk JSON structure combining metadata and messages.
type persistedFile struct {
	SessionInfo
	Messages json.RawMessage `json:"messages"`
}

// Store manages session files in a directory.
type Store struct {
	dir string
}

// New creates a Store that reads/writes session files under dir.
func New(dir string) *Store {
	return &Store{dir: dir}
}

// Save writes a session to disk atomically.
func (s *Store) Save(info SessionInfo, msgs []message.Message) error {
	msgData, err := MarshalMessages(msgs)
	if err != nil {
		return fmt.Errorf("sessions: marshal messages: %w", err)
	}

	pf := persistedFile{
		SessionInfo: info,
		Messages:    json.RawMessage(msgData),
	}

	data, err := json.MarshalIndent(pf, "", "  ")
	if err != nil {
		return fmt.Errorf("sessions: marshal session: %w", err)
	}

	if err := os.MkdirAll(s.dir, 0o750); err != nil {
		return fmt.Errorf("sessions: create dir: %w", err)
	}

	tmp, err := os.CreateTemp(s.dir, "*.tmp")
	if err != nil {
		return fmt.Errorf("sessions: create temp file: %w", err)
	}
	tmpName := tmp.Name()

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName) //nolint:gosec // tmpName from CreateTemp
		return fmt.Errorf("sessions: write temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName) //nolint:gosec // tmpName from CreateTemp
		return fmt.Errorf("sessions: close temp file: %w", err)
	}

	target := filepath.Join(s.dir, info.ID+".json")
	if err := os.Rename(tmpName, target); err != nil { //nolint:gosec // paths constructed from trusted dir + ID
		_ = os.Remove(tmpName) //nolint:gosec // tmpName from CreateTemp
		return fmt.Errorf("sessions: rename temp file: %w", err)
	}

	return nil
}

// Load reads a session from disk by ID.
func (s *Store) Load(id string) (SessionInfo, []message.Message, error) {
	path := filepath.Join(s.dir, id+".json") //nolint:gosec // ID is internally generated
	data, err := os.ReadFile(path)           //nolint:gosec // path is constructed from trusted dir + ID
	if err != nil {
		return SessionInfo{}, nil, fmt.Errorf("sessions: read file: %w", err)
	}

	var pf persistedFile
	if err := json.Unmarshal(data, &pf); err != nil {
		return SessionInfo{}, nil, fmt.Errorf("sessions: unmarshal session: %w", err)
	}

	msgs, err := UnmarshalMessages(pf.Messages)
	if err != nil {
		return SessionInfo{}, nil, err
	}

	return pf.SessionInfo, msgs, nil
}

// List returns metadata for all sessions, sorted by UpdatedAt descending.
func (s *Store) List() ([]SessionInfo, error) {
	pattern := filepath.Join(s.dir, "*.json")
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return nil, fmt.Errorf("sessions: glob: %w", err)
	}

	var sessions []SessionInfo
	for _, path := range matches {
		data, err := os.ReadFile(path) //nolint:gosec // path from trusted glob
		if err != nil {
			continue
		}
		var info SessionInfo
		if err := json.Unmarshal(data, &info); err != nil {
			continue
		}
		sessions = append(sessions, info)
	}

	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].UpdatedAt.After(sessions[j].UpdatedAt)
	})

	return sessions, nil
}

// Delete removes a session file by ID.
func (s *Store) Delete(id string) error {
	path := filepath.Join(s.dir, id+".json")
	if err := os.Remove(path); err != nil {
		return fmt.Errorf("sessions: delete: %w", err)
	}
	return nil
}
