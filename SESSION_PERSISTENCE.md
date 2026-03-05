# Session Persistence v2 — Directory-per-Session with Attachment Support

Redesign of the session storage layer to support binary attachments (images, documents) and improve `List()` performance by splitting metadata from message data.

## Current State

Single-file storage: `{sessionsDir}/{id}.json` containing both `SessionInfo` metadata and the full messages array in one JSON blob. `List()` reads and unmarshals every file to extract metadata.

**Problems:**
1. `List()` reads entire message payloads just to get metadata
2. `content.Image.Data` is base64-encoded inside JSON, bloating file size ~33%
3. No path for storing binary attachments (PDFs, images) without embedding them in JSON
4. As sessions grow, single-file reads become expensive

## Target Architecture

```
sessions/
  {id}/
    meta.json              # SessionInfo (~500 bytes)
    messages.json          # Message array (text + attachment references)
    attachments/
      {hash}.png
      {hash}.pdf
      ...
```

Each session gets its own directory. Metadata is separated from messages. Binary data lives as raw files referenced by content hash, never embedded in JSON.

---

## Phase 1: Directory-per-session with metadata split

**Goal**: Migrate from single-file to directory-per-session layout. Split metadata from messages. No attachment support yet — images still embedded as base64 in `messages.json`.

### Step 1.1: New store layout in `pkg/sessions/store.go`

Replace the flat-file `Save`/`Load`/`List`/`Delete` with directory-based equivalents.

**Directory helpers:**

```go
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
```

**`Save(info, msgs)`:**
1. `os.MkdirAll(sessionDir(info.ID), 0o750)`
2. Marshal `info` -> write atomically to `meta.json` (temp + rename)
3. `MarshalMessages(msgs)` -> write atomically to `messages.json` (temp + rename)

Both writes are independent atomic operations. If the process crashes between them, `meta.json` may be stale but `Load()` still works — the source of truth for message count comes from the messages file at load time, not the metadata.

**`Load(id)`:**
1. Read `meta.json` -> unmarshal `SessionInfo`
2. Read `messages.json` -> `UnmarshalMessages()`
3. Return both. If `messages.json` is missing, return error.

**`List()`:**
1. Read directory entries in `s.dir` (not glob — use `os.ReadDir`)
2. For each subdirectory, read only `meta.json`
3. Skip entries that aren't directories or where `meta.json` is missing/corrupt
4. Sort by `UpdatedAt` desc, return `[]SessionInfo`

This is the key performance win: `List()` reads only tiny metadata files.

**`Delete(id)`:**
1. `os.RemoveAll(sessionDir(id))` — removes directory and all contents

### Step 1.2: Migration from v1 format

Support loading legacy single-file sessions during a transition period.

**Strategy:**
- In `List()`, also glob for `*.json` files directly in `s.dir` (the v1 format). Parse them as `SessionInfo` and include in results.
- In `Load()`, if `sessionDir(id)` doesn't exist, fall back to reading `{id}.json` (v1 file).
- On first successful `Save()` after loading a v1 session, the new directory structure is written. Delete the old `{id}.json` file after successful directory write.

Add a private helper:

```go
func (s *Store) migrateV1(id string) error
```

This reads the v1 file, writes the v2 directory, and removes the v1 file. Called lazily from `Load()` when a v1 file is detected.

### Step 1.3: Update tests

- Update `TestStore_SaveLoad_RoundTrip` to verify directory structure exists
- Add `TestStore_List_OnlyReadsMetadata` — create a session dir with `meta.json` but a corrupted `messages.json`, verify `List()` still returns the metadata
- Add `TestStore_Migration_V1ToV2` — write a v1-format file, call `Load()`, verify it works and migrates to v2 on next `Save()`
- Add `TestStore_Delete_RemovesDirectory` — verify entire directory is gone
- Existing round-trip and sort tests should pass with minimal changes

### Step 1.4: Update `pkg/sessions/README.md`

Document the new directory layout and migration behavior.

**Deliverable**: Directory-per-session storage. `List()` is fast. V1 sessions auto-migrate. No changes needed in engine or TUI — the `Store` API (`Save`/`Load`/`List`/`Delete`) is unchanged.

---

## Phase 2: Attachment extraction and storage

**Goal**: Extract binary data from messages into the `attachments/` directory. Messages reference attachments by hash instead of embedding raw bytes.

### Step 2.1: Content hash utility in `pkg/sessions/attachments.go`

```go
// Hash returns the SHA-256 hex digest of data.
func attachmentHash(data []byte) string

// attachmentExt returns a file extension for the given media type.
// Falls back to ".bin" for unknown types.
func attachmentExt(mediaType string) string
```

### Step 2.2: Attachment-aware serialization in `pkg/sessions/serialize.go`

Add an `AttachmentStore` interface for the serializer to call during marshal/unmarshal:

```go
// AttachmentWriter stores binary data and returns a reference key.
type AttachmentWriter interface {
    WriteAttachment(data []byte, mediaType string) (ref string, err error)
}

// AttachmentReader loads binary data by reference key.
type AttachmentReader interface {
    ReadAttachment(ref string) (data []byte, mediaType string, err error)
}
```

New serialization functions (the old ones remain for backwards compatibility):

```go
func MarshalMessagesWithAttachments(msgs []message.Message, w AttachmentWriter) ([]byte, error)
func UnmarshalMessagesWithAttachments(data []byte, r AttachmentReader) ([]message.Message, error)
```

**Behavior during marshal:**
- When `partToJSON` encounters a `content.Image` with non-nil `Data`:
  1. Call `w.WriteAttachment(v.Data, v.MediaType)` -> get `ref`
  2. Set `jsonPart.AttachmentRef = ref` instead of `jsonPart.Data = v.Data`
  3. Clear `jsonPart.Data` (don't embed)
- For images with only a URL (no Data), serialize as before

**Behavior during unmarshal:**
- When `jsonToPart` encounters a `jsonPart` with `AttachmentRef != ""`:
  1. Call `r.ReadAttachment(ref)` -> get `data`, `mediaType`
  2. Populate `content.Image{Data: data, MediaType: mediaType}`
- For parts without `AttachmentRef`, behave as before (backwards compatible with existing messages.json)

Add to `jsonPart`:

```go
AttachmentRef string `json:"attachment_ref,omitempty"`
```

### Step 2.3: File-based attachment store in `pkg/sessions/attachments.go`

```go
type FileAttachmentStore struct {
    dir string // points to {sessionDir}/attachments/
}

func NewFileAttachmentStore(dir string) *FileAttachmentStore

func (s *FileAttachmentStore) WriteAttachment(data []byte, mediaType string) (string, error)
// 1. hash = sha256hex(data)
// 2. filename = hash + attachmentExt(mediaType)
// 3. If file already exists (dedup), return filename immediately
// 4. Write atomically (temp + rename)
// 5. Return filename

func (s *FileAttachmentStore) ReadAttachment(ref string) ([]byte, string, error)
// 1. Read file at filepath.Join(s.dir, ref)
// 2. Infer mediaType from extension
// 3. Return data, mediaType, nil
```

Content-addressable by hash means:
- Same image appearing in multiple messages is stored once
- No need to track reference counts — attachments are cheap to keep

### Step 2.4: Wire attachments into `Store.Save` and `Store.Load`

**`Save(info, msgs)`:**
1. Create `attachmentsDir(info.ID)` (mkdir -p)
2. Create `FileAttachmentStore` for the session
3. `MarshalMessagesWithAttachments(msgs, attachStore)` -> write `messages.json`
4. Write `meta.json` (unchanged)

**`Load(id)`:**
1. Read `meta.json`
2. Create `FileAttachmentStore` for the session
3. `UnmarshalMessagesWithAttachments(data, attachStore)` -> messages with binary data restored

### Step 2.5: Update `content.Image` handling in `serialize.go`

Extend `partToJSON` for future content types that carry binary data:

```go
case content.Image:
    jp := jsonPart{Kind: "image", URL: v.URL, MediaType: v.MediaType}
    if len(v.Data) > 0 && w != nil {
        ref, err := w.WriteAttachment(v.Data, v.MediaType)
        if err != nil {
            return jp, err  // or log and fall back to inline
        }
        jp.AttachmentRef = ref
    } else {
        jp.Data = v.Data
    }
    return jp, nil
```

### Step 2.6: Tests

- `TestAttachmentHash` — deterministic SHA-256
- `TestFileAttachmentStore_WriteRead_RoundTrip` — write data, read back, verify identical
- `TestFileAttachmentStore_Dedup` — write same data twice, verify single file on disk
- `TestMarshalWithAttachments_ExtractsImageData` — marshal message with image data, verify `messages.json` has `attachment_ref` not `data`, verify file exists in attachments dir
- `TestUnmarshalWithAttachments_RestoresImageData` — full round trip
- `TestUnmarshalWithAttachments_BackwardsCompatible` — unmarshal a `messages.json` with inline `data` (no `attachment_ref`), verify it still works
- `TestMarshalWithAttachments_URLOnlyImage` — image with URL but no Data is serialized without attachment

**Deliverable**: Binary data extracted to files. Messages stay small JSON. Backwards compatible with v1 messages that have inline data.

---

## Phase 3: New content types (Document part)

**Goal**: Add a `content.Document` part type for PDFs and other document files, with attachment storage support.

### Step 3.1: Add `content.Document` to `pkg/chats/content/content.go`

```go
// Document is a document content part (PDF, DOCX, etc.) referenced by path or embedded as raw bytes.
type Document struct {
    Path      string // Original file path (for display)
    Data      []byte // Raw document bytes
    MediaType string // MIME type (application/pdf, etc.)
}

func (d Document) PartKind() string { return "document" }
```

### Step 3.2: Serialization support in `pkg/sessions/serialize.go`

Add `"document"` case to `partToJSON` and `jsonToPart`:

```go
case content.Document:
    jp := jsonPart{Kind: "document", URL: v.Path, MediaType: v.MediaType}
    if len(v.Data) > 0 && w != nil {
        ref, err := w.WriteAttachment(v.Data, v.MediaType)
        // ...
        jp.AttachmentRef = ref
    } else {
        jp.Data = v.Data
    }
    return jp, nil
```

Add to `jsonPart` if needed (reuse existing `URL` field for `Path`, already has `Data`, `MediaType`, `AttachmentRef`).

### Step 3.3: Tests

- Round-trip test for `content.Document` with attachment extraction
- Verify mixed messages with Text + Image + Document parts

**Deliverable**: Documents are a first-class content type, persisted as attachments.

---

## Phase 4: Cleanup and optimization

### Step 4.1: Orphan attachment cleanup

Add a method to remove attachments no longer referenced by any message:

```go
func (s *Store) CleanAttachments(id string) error
```

1. Load `messages.json`, collect all `attachment_ref` values into a set
2. List files in `attachments/`
3. Remove any file not in the set

Call this from `Save()` after writing the new `messages.json` (compaction may have dropped messages that referenced attachments).

### Step 4.2: Remove v1 migration code

After a reasonable transition period, remove the v1 fallback from `Load()` and `List()`. Add a standalone migration command or one-time startup migration instead.

### Step 4.3: Attachment size limits

Add a configurable max attachment size. If an image or document exceeds the limit, store a reference/thumbnail instead of the full binary. Log a warning.

### Step 4.4: List pagination

If session count grows very large, add pagination to `List()`:

```go
func (s *Store) List(opts ListOpts) ([]SessionInfo, error)

type ListOpts struct {
    Limit  int
    Offset int
}
```

The session picker can load pages on demand.

---

## Migration Path

| Version | Storage format | List performance | Binary support |
|---------|---------------|-----------------|----------------|
| v1 (current) | `{id}.json` (single file) | O(n) full file reads | Inline base64 only |
| v2 (Phase 1) | `{id}/meta.json` + `messages.json` | O(n) small file reads | Inline base64 only |
| v2 + attachments (Phase 2) | + `{id}/attachments/{hash}.ext` | Same | Native binary files |

v1 -> v2 migration is lazy (on load) and transparent. No user action required.

## Files Changed/Created Per Phase

| Phase | Files | Dependencies |
|-------|-------|-------------|
| **1** | `pkg/sessions/store.go`, `store_test.go`, `README.md` | None (API unchanged) |
| **2** | `pkg/sessions/attachments.go`, `attachments_test.go`, `serialize.go`, `serialize_test.go`, `store.go`, `store_test.go` | Phase 1 |
| **3** | `pkg/chats/content/content.go`, `pkg/sessions/serialize.go`, `serialize_test.go` | Phase 2 |
| **4** | `pkg/sessions/store.go`, cleanup across existing files | Phase 3 |

Each phase is independently shippable. Phase 1 is a pure refactor with no behavior change for consumers. Phase 2 adds invisible optimization. Phase 3 adds new capability. Phase 4 hardens.
