package sessions

import (
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

func TestStore_Delete(t *testing.T) {
	store := New(t.TempDir())
	require.NoError(t, store.Save(testInfo("sess-1"), testMessages()))

	require.NoError(t, store.Delete("sess-1"))

	_, _, err := store.Load("sess-1")
	assert.Error(t, err)
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
