package sessions

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
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

// persistedFile is the legacy v1 on-disk JSON structure combining metadata and messages.
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

func (s *Store) sessionDir(id string) string {
	return filepath.Join(s.dir, id)
}

func (s *Store) metaPath(id string) string {
	return filepath.Join(s.sessionDir(id), "meta.json")
}

func (s *Store) messagesPath(id string) string {
	return filepath.Join(s.sessionDir(id), "messages.json")
}

func (s *Store) attachmentsDir(id string) string {
	return filepath.Join(s.sessionDir(id), "attachments")
}

func (s *Store) v1Path(id string) string {
	return filepath.Join(s.dir, id+".json")
}

// Save writes a session to disk atomically using the v2 directory layout.
// Binary data (e.g. images) is extracted to the attachments/ subdirectory.
func (s *Store) Save(info SessionInfo, msgs []message.Message) error {
	sessDir := s.sessionDir(info.ID)
	if err := os.MkdirAll(sessDir, 0o750); err != nil {
		return fmt.Errorf("sessions: create session dir: %w", err)
	}

	attachStore := NewFileAttachmentStore(s.attachmentsDir(info.ID))
	msgData, err := MarshalMessagesWithAttachments(msgs, attachStore)
	if err != nil {
		return fmt.Errorf("sessions: marshal messages: %w", err)
	}

	// Write meta.json atomically.
	metaData, err := json.MarshalIndent(info, "", "  ")
	if err != nil {
		return fmt.Errorf("sessions: marshal meta: %w", err)
	}
	if err := atomicWrite(sessDir, s.metaPath(info.ID), metaData); err != nil {
		return fmt.Errorf("sessions: write meta: %w", err)
	}

	// Write messages.json atomically.
	if err := atomicWrite(sessDir, s.messagesPath(info.ID), msgData); err != nil {
		return fmt.Errorf("sessions: write messages: %w", err)
	}

	// Clean up v1 file if it exists.
	_ = os.Remove(s.v1Path(info.ID)) //nolint:gosec // path from trusted dir + ID

	return nil
}

// Load reads a session from disk by ID. Migrates v1 sessions lazily.
func (s *Store) Load(id string) (SessionInfo, []message.Message, error) {
	// Try v2 layout first.
	if _, err := os.Stat(s.sessionDir(id)); err == nil {
		return s.loadV2(id)
	}

	// Fall back to v1 single-file format.
	return s.loadV1(id)
}

func (s *Store) loadV2(id string) (SessionInfo, []message.Message, error) {
	metaData, err := os.ReadFile(s.metaPath(id)) //nolint:gosec // path from trusted dir + ID
	if err != nil {
		return SessionInfo{}, nil, fmt.Errorf("sessions: read meta: %w", err)
	}
	var info SessionInfo
	if err := json.Unmarshal(metaData, &info); err != nil {
		return SessionInfo{}, nil, fmt.Errorf("sessions: unmarshal meta: %w", err)
	}

	msgData, err := os.ReadFile(s.messagesPath(id)) //nolint:gosec // path from trusted dir + ID
	if err != nil {
		return SessionInfo{}, nil, fmt.Errorf("sessions: read messages: %w", err)
	}

	attachStore := NewFileAttachmentStore(s.attachmentsDir(id))
	msgs, err := UnmarshalMessagesWithAttachments(msgData, attachStore)
	if err != nil {
		return SessionInfo{}, nil, err
	}

	return info, msgs, nil
}

func (s *Store) loadV1(id string) (SessionInfo, []message.Message, error) {
	path := s.v1Path(id)
	data, err := os.ReadFile(path) //nolint:gosec // path from trusted dir + ID
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
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("sessions: read dir: %w", err)
	}

	var sessions []SessionInfo

	for _, entry := range entries {
		if entry.IsDir() {
			// v2: read meta.json from subdirectory.
			metaPath := filepath.Join(s.dir, entry.Name(), "meta.json")
			data, err := os.ReadFile(metaPath) //nolint:gosec // path from trusted dir
			if err != nil {
				continue
			}
			var info SessionInfo
			if err := json.Unmarshal(data, &info); err != nil {
				continue
			}
			sessions = append(sessions, info)
		} else if strings.HasSuffix(entry.Name(), ".json") {
			// v1: legacy single-file format.
			path := filepath.Join(s.dir, entry.Name())
			data, err := os.ReadFile(path) //nolint:gosec // path from trusted dir
			if err != nil {
				continue
			}
			var info SessionInfo
			if err := json.Unmarshal(data, &info); err != nil {
				continue
			}
			sessions = append(sessions, info)
		}
	}

	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].UpdatedAt.After(sessions[j].UpdatedAt)
	})

	return sessions, nil
}

// Delete removes a session by ID (supports both v1 and v2 layouts).
func (s *Store) Delete(id string) error {
	sessDir := s.sessionDir(id)
	if _, err := os.Stat(sessDir); err == nil {
		if err := os.RemoveAll(sessDir); err != nil { //nolint:gosec // path from trusted dir + ID
			return fmt.Errorf("sessions: delete: %w", err)
		}
		return nil
	}

	// Fall back to v1 single file.
	path := s.v1Path(id)
	if err := os.Remove(path); err != nil {
		return fmt.Errorf("sessions: delete: %w", err)
	}
	return nil
}

// atomicWrite writes data to target via a temp file + rename in tmpDir.
func atomicWrite(tmpDir, target string, data []byte) error {
	tmp, err := os.CreateTemp(tmpDir, "*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName) //nolint:gosec // tmpName from CreateTemp
		return err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName) //nolint:gosec // tmpName from CreateTemp
		return err
	}

	if err := os.Rename(tmpName, target); err != nil { //nolint:gosec // target is constructed from trusted dir + ID
		_ = os.Remove(tmpName) //nolint:gosec // tmpName from CreateTemp
		return err
	}
	return nil
}
