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

// writeV1File writes a legacy v1 single-file session directly to disk.
func writeV1File(t *testing.T, dir string, info SessionInfo, msgs []message.Message) {
	t.Helper()
	msgData, err := MarshalMessages(msgs)
	require.NoError(t, err)

	pf := persistedFile{
		SessionInfo: info,
		Messages:    json.RawMessage(msgData),
	}
	data, err := json.MarshalIndent(pf, "", "  ")
	require.NoError(t, err)

	require.NoError(t, os.MkdirAll(dir, 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(dir, info.ID+".json"), data, 0o600))
}

func TestStore_Migration_V1ToV2(t *testing.T) {
	dir := t.TempDir()
	store := New(dir)
	info := testInfo("legacy")
	msgs := testMessages()

	// Write a v1-format file directly.
	writeV1File(t, dir, info, msgs)

	// Load should work via v1 fallback.
	gotInfo, gotMsgs, err := store.Load("legacy")
	require.NoError(t, err)
	assert.Equal(t, info.ID, gotInfo.ID)
	require.Len(t, gotMsgs, 2)

	// Save migrates to v2 and removes v1 file.
	require.NoError(t, store.Save(gotInfo, gotMsgs))
	assert.DirExists(t, store.sessionDir("legacy"))
	assert.NoFileExists(t, filepath.Join(dir, "legacy.json"))

	// Load should now use v2.
	gotInfo2, gotMsgs2, err := store.Load("legacy")
	require.NoError(t, err)
	assert.Equal(t, info.ID, gotInfo2.ID)
	require.Len(t, gotMsgs2, 2)
}

func TestStore_List_MixedV1V2(t *testing.T) {
	dir := t.TempDir()
	store := New(dir)
	now := time.Now().Truncate(time.Millisecond)

	// Write a v2 session.
	v2Info := testInfo("v2-sess")
	v2Info.UpdatedAt = now.Add(time.Hour)
	require.NoError(t, store.Save(v2Info, testMessages()))

	// Write a v1 session.
	v1Info := testInfo("v1-sess")
	v1Info.UpdatedAt = now
	writeV1File(t, dir, v1Info, testMessages())

	list, err := store.List()
	require.NoError(t, err)
	require.Len(t, list, 2)
	assert.Equal(t, "v2-sess", list[0].ID)
	assert.Equal(t, "v1-sess", list[1].ID)
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

func TestStore_Delete_V1(t *testing.T) {
	dir := t.TempDir()
	store := New(dir)

	writeV1File(t, dir, testInfo("v1-sess"), testMessages())

	require.NoError(t, store.Delete("v1-sess"))
	assert.NoFileExists(t, filepath.Join(dir, "v1-sess.json"))
}
