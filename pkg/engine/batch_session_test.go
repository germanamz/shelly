package engine

import (
	"strings"
	"testing"

	"github.com/germanamz/shelly/pkg/chats/content"
	"github.com/germanamz/shelly/pkg/chats/message"
	"github.com/germanamz/shelly/pkg/chats/role"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseBatchTasks_Valid(t *testing.T) {
	input := `{"id":"task-1","agent":"coder","task":"Review this file"}
{"id":"task-2","task":"Fix the bug","context":"src/main.go has an issue"}
`
	tasks, err := parseBatchTasks(strings.NewReader(input))
	require.NoError(t, err)
	assert.Len(t, tasks, 2)

	assert.Equal(t, "task-1", tasks[0].ID)
	assert.Equal(t, "coder", tasks[0].Agent)
	assert.Equal(t, "Review this file", tasks[0].Task)
	assert.Empty(t, tasks[0].Context)

	assert.Equal(t, "task-2", tasks[1].ID)
	assert.Empty(t, tasks[1].Agent)
	assert.Equal(t, "Fix the bug", tasks[1].Task)
	assert.Equal(t, "src/main.go has an issue", tasks[1].Context)
}

func TestParseBatchTasks_EmptyLines(t *testing.T) {
	input := `{"id":"task-1","task":"hello"}

{"id":"task-2","task":"world"}
`
	tasks, err := parseBatchTasks(strings.NewReader(input))
	require.NoError(t, err)
	assert.Len(t, tasks, 2)
}

func TestParseBatchTasks_MissingID(t *testing.T) {
	input := `{"task":"hello"}`
	_, err := parseBatchTasks(strings.NewReader(input))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "task id is required")
}

func TestParseBatchTasks_MissingTask(t *testing.T) {
	input := `{"id":"task-1"}`
	_, err := parseBatchTasks(strings.NewReader(input))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "task field is required")
}

func TestParseBatchTasks_InvalidJSON(t *testing.T) {
	input := `not json`
	_, err := parseBatchTasks(strings.NewReader(input))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "line 1")
}

func TestParseBatchTasks_Empty(t *testing.T) {
	tasks, err := parseBatchTasks(strings.NewReader(""))
	require.NoError(t, err)
	assert.Nil(t, tasks)
}

func TestReplyText(t *testing.T) {
	msg := message.New("", role.Assistant,
		content.Text{Text: "Hello "},
		content.ToolCall{ID: "tc-1", Name: "test"},
		content.Text{Text: "world"},
	)
	assert.Equal(t, "Hello world", replyText(msg))
}

func TestReplyText_Empty(t *testing.T) {
	msg := message.Message{}
	assert.Empty(t, replyText(msg))
}
