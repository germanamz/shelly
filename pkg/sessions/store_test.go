package sessions

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/germanamz/shelly/pkg/chats/content"
	"github.com/germanamz/shelly/pkg/chats/message"
	"github.com/germanamz/shelly/pkg/chats/role"
)

func testInfo(id string) SessionInfo {
	now := time.Now().Truncate(time.Millisecond)
	return SessionInfo{
		ID:        id,
		Agent:     "coder",
		Provider:  ProviderMeta{Kind: "anthropic", Model: "claude"},
		CreatedAt: now,
		UpdatedAt: now,
		Preview:   "hello world",
		MsgCount:  2,
	}
}

func testMessages() []message.Message {
	return []message.Message{
		message.NewText("", role.System, "You are helpful."),
		message.NewText("user", role.User, "Hi"),
	}
}

func TestStore_SaveLoad_RoundTrip(t *testing.T) {
	store := New(t.TempDir())
	info := testInfo("sess-1")
	msgs := testMessages()

	require.NoError(t, store.Save(info, msgs))

	// Verify v2 directory structure exists.
	assert.DirExists(t, store.sessionDir("sess-1"))
	assert.FileExists(t, store.metaPath("sess-1"))
	assert.FileExists(t, store.messagesPath("sess-1"))

	gotInfo, gotMsgs, err := store.Load("sess-1")
	require.NoError(t, err)

	assert.Equal(t, info.ID, gotInfo.ID)
	assert.Equal(t, info.Agent, gotInfo.Agent)
	assert.Equal(t, info.Provider, gotInfo.Provider)
	assert.Equal(t, info.Preview, gotInfo.Preview)
	assert.Equal(t, info.MsgCount, gotInfo.MsgCount)
	assert.True(t, info.CreatedAt.Equal(gotInfo.CreatedAt))
	assert.True(t, info.UpdatedAt.Equal(gotInfo.UpdatedAt))

	require.Len(t, gotMsgs, 2)
	assert.Equal(t, "You are helpful.", gotMsgs[0].TextContent())
	assert.Equal(t, "Hi", gotMsgs[1].TextContent())
}

func TestStore_SaveOverwrites(t *testing.T) {
	store := New(t.TempDir())
	info := testInfo("sess-1")

	require.NoError(t, store.Save(info, testMessages()))

	info.Preview = "updated preview"
	updatedMsgs := append(testMessages(), message.NewText("bot", role.Assistant, "Hello!"))
	require.NoError(t, store.Save(info, updatedMsgs))

	gotInfo, gotMsgs, err := store.Load("sess-1")
	require.NoError(t, err)
	assert.Equal(t, "updated preview", gotInfo.Preview)
	require.Len(t, gotMsgs, 3)
}

func TestStore_Load_NotFound(t *testing.T) {
	store := New(t.TempDir())
	_, _, err := store.Load("nonexistent")
	assert.Error(t, err)
}

func TestStore_List_SortedByUpdatedAt(t *testing.T) {
	store := New(t.TempDir())
	now := time.Now().Truncate(time.Millisecond)

	for i, id := range []string{"old", "mid", "new"} {
		info := testInfo(id)
		info.UpdatedAt = now.Add(time.Duration(i) * time.Hour)
		require.NoError(t, store.Save(info, testMessages()))
	}

	list, err := store.List()
	require.NoError(t, err)
	require.Len(t, list, 3)
	assert.Equal(t, "new", list[0].ID)
	assert.Equal(t, "mid", list[1].ID)
	assert.Equal(t, "old", list[2].ID)
}

func TestStore_List_Empty(t *testing.T) {
	store := New(t.TempDir())
	list, err := store.List()
	require.NoError(t, err)
	assert.Empty(t, list)
}

func TestStore_List_OnlyReadsMetadata(t *testing.T) {
	store := New(t.TempDir())
	info := testInfo("sess-1")

	// Create v2 directory with valid meta.json but corrupted messages.json.
	sessDir := store.sessionDir("sess-1")
	require.NoError(t, os.MkdirAll(sessDir, 0o750))

	metaData, err := json.MarshalIndent(info, "", "  ")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(store.metaPath("sess-1"), metaData, 0o600))
	require.NoError(t, os.WriteFile(store.messagesPath("sess-1"), []byte("corrupted"), 0o600))

	list, err := store.List()
	require.NoError(t, err)
	require.Len(t, list, 1)
	assert.Equal(t, "sess-1", list[0].ID)
}

func TestStore_List_Pagination(t *testing.T) {
	store := New(t.TempDir())
	now := time.Now().Truncate(time.Millisecond)

	for i := range 5 {
		info := testInfo("s" + string(rune('A'+i)))
		info.UpdatedAt = now.Add(time.Duration(i) * time.Hour)
		require.NoError(t, store.Save(info, testMessages()))
	}

	// Limit only.
	list, err := store.List(ListOpts{Limit: 2})
	require.NoError(t, err)
	require.Len(t, list, 2)
	assert.Equal(t, "sE", list[0].ID) // newest first
	assert.Equal(t, "sD", list[1].ID)

	// Offset only.
	list, err = store.List(ListOpts{Offset: 3})
	require.NoError(t, err)
	require.Len(t, list, 2)
	assert.Equal(t, "sB", list[0].ID)
	assert.Equal(t, "sA", list[1].ID)

	// Limit + Offset.
	list, err = store.List(ListOpts{Limit: 2, Offset: 1})
	require.NoError(t, err)
	require.Len(t, list, 2)
	assert.Equal(t, "sD", list[0].ID)
	assert.Equal(t, "sC", list[1].ID)

	// Offset past end.
	list, err = store.List(ListOpts{Offset: 100})
	require.NoError(t, err)
	assert.Empty(t, list)
}

func TestStore_Delete(t *testing.T) {
	store := New(t.TempDir())
	require.NoError(t, store.Save(testInfo("sess-1"), testMessages()))

	require.NoError(t, store.Delete("sess-1"))

	_, _, err := store.Load("sess-1")
	require.Error(t, err)
	assert.NoDirExists(t, store.sessionDir("sess-1"))
}

func TestStore_Delete_NotFound(t *testing.T) {
	store := New(t.TempDir())
	err := store.Delete("nonexistent")
	assert.Error(t, err)
}

func TestStore_SaveLoad_WithAllPartTypes(t *testing.T) {
	store := New(t.TempDir())
	info := testInfo("full")
	msgs := []message.Message{
		message.NewText("", role.System, "sys"),
		{
			Sender: "user",
			Role:   role.User,
			Parts: []content.Part{
				content.Text{Text: "look"},
				content.Image{URL: "http://img", Data: []byte{1, 2}, MediaType: "image/png"},
			},
		},
		message.New("bot", role.Assistant,
			content.Text{Text: "searching"},
			content.ToolCall{ID: "c1", Name: "search", Arguments: `{"q":"x"}`, Metadata: map[string]string{"k": "v"}},
		),
		message.New("", role.Tool,
			content.ToolResult{ToolCallID: "c1", Content: "found", IsError: false},
		),
	}

	require.NoError(t, store.Save(info, msgs))

	_, gotMsgs, err := store.Load("full")
	require.NoError(t, err)
	require.Len(t, gotMsgs, 4)

	img := gotMsgs[1].Parts[1].(content.Image)
	assert.Equal(t, []byte{1, 2}, img.Data)

	tc := gotMsgs[2].Parts[1].(content.ToolCall)
	assert.Equal(t, "v", tc.Metadata["k"])

	tr := gotMsgs[3].Parts[0].(content.ToolResult)
	assert.Equal(t, "found", tr.Content)
}

func TestStore_SaveLoad_WithAttachments(t *testing.T) {
	store := New(t.TempDir())
	info := testInfo("attach-sess")
	imgData := []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}
	msgs := []message.Message{
		{
			Sender: "user",
			Role:   role.User,
			Parts: []content.Part{
				content.Text{Text: "look at this"},
				content.Image{Data: imgData, MediaType: "image/png"},
			},
		},
	}

	require.NoError(t, store.Save(info, msgs))

	// Verify attachments directory was created with a file.
	attachDir := filepath.Join(store.sessionDir("attach-sess"), "attachments")
	assert.DirExists(t, attachDir)
	entries, err := os.ReadDir(attachDir)
	require.NoError(t, err)
	assert.Len(t, entries, 1)
	assert.Contains(t, entries[0].Name(), ".png")

	// Verify messages.json does not contain inline data.
	msgData, err := os.ReadFile(store.messagesPath("attach-sess"))
	require.NoError(t, err)
	assert.NotContains(t, string(msgData), `"data"`)
	assert.Contains(t, string(msgData), `"attachment_ref"`)

	// Load and verify round-trip.
	_, gotMsgs, err := store.Load("attach-sess")
	require.NoError(t, err)
	require.Len(t, gotMsgs, 1)
	img := gotMsgs[0].Parts[1].(content.Image)
	assert.Equal(t, imgData, img.Data)
	assert.Equal(t, "image/png", img.MediaType)
}

// writeV1File writes a legacy v1 single-file session directly to disk.
func writeV1File(t *testing.T, dir string, info SessionInfo, msgs []message.Message) {
	t.Helper()
	msgData, err := MarshalMessages(msgs)
	require.NoError(t, err)

	pf := v1PersistedFile{
		SessionInfo: info,
		Messages:    json.RawMessage(msgData),
	}
	data, err := json.MarshalIndent(pf, "", "  ")
	require.NoError(t, err)

	require.NoError(t, os.MkdirAll(dir, 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(dir, info.ID+".json"), data, 0o600))
}

func TestStore_MigrateV1(t *testing.T) {
	dir := t.TempDir()
	store := New(dir)

	// Write two v1-format files.
	writeV1File(t, dir, testInfo("legacy1"), testMessages())
	writeV1File(t, dir, testInfo("legacy2"), testMessages())

	migrated, err := store.MigrateV1()
	require.NoError(t, err)
	assert.Equal(t, 2, migrated)

	// V1 files should be gone.
	assert.NoFileExists(t, filepath.Join(dir, "legacy1.json"))
	assert.NoFileExists(t, filepath.Join(dir, "legacy2.json"))

	// V2 directories should exist and load correctly.
	for _, id := range []string{"legacy1", "legacy2"} {
		assert.DirExists(t, store.sessionDir(id))
		info, msgs, err := store.Load(id)
		require.NoError(t, err)
		assert.Equal(t, id, info.ID)
		require.Len(t, msgs, 2)
	}

	// List should show both.
	list, err := store.List()
	require.NoError(t, err)
	assert.Len(t, list, 2)
}

func TestStore_MigrateV1_NoV1Files(t *testing.T) {
	dir := t.TempDir()
	store := New(dir)

	// Save a v2 session.
	require.NoError(t, store.Save(testInfo("v2-sess"), testMessages()))

	migrated, err := store.MigrateV1()
	require.NoError(t, err)
	assert.Equal(t, 0, migrated)
}

func TestStore_CleanAttachments_RemovesOrphans(t *testing.T) {
	store := New(t.TempDir())
	info := testInfo("clean-sess")
	imgData := []byte{0x89, 0x50, 0x4E, 0x47}
	msgs := []message.Message{
		{
			Sender: "user",
			Role:   role.User,
			Parts: []content.Part{
				content.Image{Data: imgData, MediaType: "image/png"},
			},
		},
	}

	require.NoError(t, store.Save(info, msgs))

	// Add an orphan file to the attachments directory.
	attachDir := store.attachmentsDir("clean-sess")
	orphanPath := filepath.Join(attachDir, "orphan.png")
	require.NoError(t, os.WriteFile(orphanPath, []byte("orphan"), 0o600))

	// Verify orphan exists.
	entries, err := os.ReadDir(attachDir)
	require.NoError(t, err)
	assert.Len(t, entries, 2) // real attachment + orphan

	// Clean should remove the orphan.
	require.NoError(t, store.CleanAttachments("clean-sess"))

	entries, err = os.ReadDir(attachDir)
	require.NoError(t, err)
	assert.Len(t, entries, 1)
	assert.NotEqual(t, "orphan.png", entries[0].Name())
}

func TestStore_CleanAttachments_NoAttachmentsDir(t *testing.T) {
	store := New(t.TempDir())
	info := testInfo("no-attach")
	require.NoError(t, store.Save(info, testMessages()))

	// Should not error when there's no attachments directory.
	assert.NoError(t, store.CleanAttachments("no-attach"))
}

func TestStore_Save_CleansOrphansAutomatically(t *testing.T) {
	store := New(t.TempDir())
	info := testInfo("auto-clean")
	imgData := []byte{0x89, 0x50, 0x4E, 0x47}
	msgs := []message.Message{
		{
			Sender: "user",
			Role:   role.User,
			Parts: []content.Part{
				content.Image{Data: imgData, MediaType: "image/png"},
			},
		},
	}

	require.NoError(t, store.Save(info, msgs))

	// Add an orphan.
	attachDir := store.attachmentsDir("auto-clean")
	require.NoError(t, os.WriteFile(filepath.Join(attachDir, "orphan.bin"), []byte("x"), 0o600))

	// Re-save with the same messages — orphan should be cleaned.
	require.NoError(t, store.Save(info, msgs))

	entries, err := os.ReadDir(attachDir)
	require.NoError(t, err)
	assert.Len(t, entries, 1)
}

func TestStore_WithMaxAttachmentSize(t *testing.T) {
	store := New(t.TempDir(), WithMaxAttachmentSize(10))
	info := testInfo("size-limit")

	smallImg := []byte{1, 2, 3}
	bigImg := make([]byte, 20)
	msgs := []message.Message{
		{
			Sender: "user",
			Role:   role.User,
			Parts: []content.Part{
				content.Image{Data: smallImg, MediaType: "image/png"},
				content.Image{Data: bigImg, MediaType: "image/png"},
			},
		},
	}

	require.NoError(t, store.Save(info, msgs))

	// The small image should be stored as attachment; the big one falls back to inline.
	_, gotMsgs, err := store.Load("size-limit")
	require.NoError(t, err)
	require.Len(t, gotMsgs, 1)

	// Small image: round-trips via attachment.
	img0 := gotMsgs[0].Parts[0].(content.Image)
	assert.Equal(t, smallImg, img0.Data)

	// Big image: fell back to inline (data embedded in JSON).
	img1 := gotMsgs[0].Parts[1].(content.Image)
	assert.Equal(t, bigImg, img1.Data)
}
