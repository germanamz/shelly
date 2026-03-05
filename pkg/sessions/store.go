package sessions

import (
	"encoding/json"
	"fmt"
	"log/slog"
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

// ListOpts controls pagination for List().
type ListOpts struct {
	Limit  int // Maximum number of results (0 = unlimited).
	Offset int // Number of results to skip.
}

// StoreOption configures optional Store behavior.
type StoreOption func(*Store)

// WithMaxAttachmentSize sets the maximum size in bytes for a single attachment.
// Attachments exceeding this limit are skipped with a warning log.
// A value of 0 (the default) means no limit.
func WithMaxAttachmentSize(n int64) StoreOption {
	return func(s *Store) {
		s.maxAttachmentSize = n
	}
}

// Store manages session files in a directory.
type Store struct {
	dir               string
	maxAttachmentSize int64 // 0 = unlimited
}

// New creates a Store that reads/writes session files under dir.
func New(dir string, opts ...StoreOption) *Store {
	s := &Store{dir: dir}
	for _, o := range opts {
		o(s)
	}
	return s
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

// Save writes a session to disk atomically using the v2 directory layout.
// Binary data (e.g. images) is extracted to the attachments/ subdirectory.
// After writing, orphan attachments no longer referenced by any message are removed.
func (s *Store) Save(info SessionInfo, msgs []message.Message) error {
	sessDir := s.sessionDir(info.ID)
	if err := os.MkdirAll(sessDir, 0o750); err != nil {
		return fmt.Errorf("sessions: create session dir: %w", err)
	}

	attachStore := NewFileAttachmentStore(s.attachmentsDir(info.ID))
	var w AttachmentWriter = attachStore
	if s.maxAttachmentSize > 0 {
		w = &sizeLimitedWriter{inner: attachStore, maxSize: s.maxAttachmentSize}
	}
	msgData, err := MarshalMessagesWithAttachments(msgs, w)
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

	// Remove orphan attachments after writing the new messages file.
	if err := s.CleanAttachments(info.ID); err != nil {
		slog.Warn("sessions: clean attachments", "id", info.ID, "err", err)
	}

	return nil
}

// Load reads a session from disk by ID (v2 directory layout only).
func (s *Store) Load(id string) (SessionInfo, []message.Message, error) {
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

// List returns metadata for all sessions, sorted by UpdatedAt descending.
// Use opts to paginate; a zero-value ListOpts returns all sessions.
func (s *Store) List(opts ...ListOpts) ([]SessionInfo, error) {
	var o ListOpts
	if len(opts) > 0 {
		o = opts[0]
	}

	entries, err := os.ReadDir(s.dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("sessions: read dir: %w", err)
	}

	var sessions []SessionInfo

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
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
	}

	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].UpdatedAt.After(sessions[j].UpdatedAt)
	})

	// Apply pagination.
	if o.Offset > 0 {
		if o.Offset >= len(sessions) {
			return nil, nil
		}
		sessions = sessions[o.Offset:]
	}
	if o.Limit > 0 && o.Limit < len(sessions) {
		sessions = sessions[:o.Limit]
	}

	return sessions, nil
}

// Delete removes a session by ID.
func (s *Store) Delete(id string) error {
	sessDir := s.sessionDir(id)
	if _, err := os.Stat(sessDir); err != nil {
		return fmt.Errorf("sessions: delete: %w", err)
	}
	if err := os.RemoveAll(sessDir); err != nil { //nolint:gosec // path from trusted dir + ID
		return fmt.Errorf("sessions: delete: %w", err)
	}
	return nil
}

// CleanAttachments removes attachment files not referenced by any message in the session.
func (s *Store) CleanAttachments(id string) error {
	attachDir := s.attachmentsDir(id)
	entries, err := os.ReadDir(attachDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("sessions: read attachments dir: %w", err)
	}
	if len(entries) == 0 {
		return nil
	}

	// Collect all attachment refs from messages.json.
	msgData, err := os.ReadFile(s.messagesPath(id)) //nolint:gosec // trusted path
	if err != nil {
		return fmt.Errorf("sessions: read messages for cleanup: %w", err)
	}
	refs, err := collectAttachmentRefs(msgData)
	if err != nil {
		return fmt.Errorf("sessions: parse messages for cleanup: %w", err)
	}

	// Remove files not in the refs set.
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if !refs[entry.Name()] {
			path := filepath.Join(attachDir, entry.Name())
			if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
				slog.Warn("sessions: remove orphan attachment", "path", path, "err", err)
			}
		}
	}
	return nil
}

// collectAttachmentRefs scans serialized messages JSON for all attachment_ref values.
func collectAttachmentRefs(msgData []byte) (map[string]bool, error) {
	var jmsgs []jsonMessage
	if err := json.Unmarshal(msgData, &jmsgs); err != nil {
		return nil, err
	}
	refs := make(map[string]bool)
	for _, jm := range jmsgs {
		for _, jp := range jm.Parts {
			if jp.AttachmentRef != "" {
				refs[jp.AttachmentRef] = true
			}
		}
	}
	return refs, nil
}

// MigrateV1 migrates all legacy v1 single-file sessions ({id}.json) in the store
// directory to the v2 directory-per-session layout. Returns the number of sessions migrated.
func (s *Store) MigrateV1() (int, error) {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, fmt.Errorf("sessions: read dir for migration: %w", err)
	}

	migrated := 0
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		path := filepath.Join(s.dir, entry.Name())
		if err := s.migrateOneV1(path); err != nil {
			slog.Warn("sessions: v1 migration failed", "file", entry.Name(), "err", err)
			continue
		}
		migrated++
	}
	return migrated, nil
}

// v1PersistedFile is the legacy v1 on-disk JSON structure combining metadata and messages.
type v1PersistedFile struct {
	SessionInfo
	Messages json.RawMessage `json:"messages"`
}

func (s *Store) migrateOneV1(path string) error {
	data, err := os.ReadFile(path) //nolint:gosec // path from trusted dir
	if err != nil {
		return err
	}

	var pf v1PersistedFile
	if err := json.Unmarshal(data, &pf); err != nil {
		return err
	}

	msgs, err := UnmarshalMessages(pf.Messages)
	if err != nil {
		return err
	}

	if err := s.Save(pf.SessionInfo, msgs); err != nil {
		return err
	}

	return os.Remove(path)
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
