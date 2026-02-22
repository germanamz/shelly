package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFormatToolCall(t *testing.T) {
	tests := []struct {
		name     string
		tool     string
		args     string
		expected string
	}{
		// Filesystem
		{name: "fs_read", tool: "fs_read", args: `{"path":"src/main.go"}`, expected: `Reading file "src/main.go"`},
		{name: "fs_write", tool: "fs_write", args: `{"path":"out.txt","content":"hi"}`, expected: `Writing file "out.txt"`},
		{name: "fs_edit", tool: "fs_edit", args: `{"path":"foo.go","old_text":"a","new_text":"b"}`, expected: `Editing file "foo.go"`},
		{name: "fs_list", tool: "fs_list", args: `{"path":"src"}`, expected: `Listing directory "src"`},
		{name: "fs_delete", tool: "fs_delete", args: `{"path":"tmp"}`, expected: `Deleting "tmp"`},
		{name: "fs_move", tool: "fs_move", args: `{"source":"a.txt","destination":"b.txt"}`, expected: `Moving "a.txt" to "b.txt"`},
		{name: "fs_copy", tool: "fs_copy", args: `{"source":"a","destination":"b"}`, expected: `Copying "a" to "b"`},
		{name: "fs_mkdir", tool: "fs_mkdir", args: `{"path":"new_dir"}`, expected: `Creating directory "new_dir"`},
		{name: "fs_stat", tool: "fs_stat", args: `{"path":"file.go"}`, expected: `Getting info for "file.go"`},
		{name: "fs_diff", tool: "fs_diff", args: `{"file_a":"a.go","file_b":"b.go"}`, expected: `Comparing "a.go" and "b.go"`},
		{name: "fs_patch", tool: "fs_patch", args: `{"path":"main.go","hunks":[]}`, expected: `Patching file "main.go"`},

		// Search
		{name: "search_content with dir", tool: "search_content", args: `{"pattern":"TODO","directory":"src"}`, expected: `Searching for "TODO" in "src"`},
		{name: "search_content no dir", tool: "search_content", args: `{"pattern":"TODO"}`, expected: `Searching for "TODO"`},
		{name: "search_files with dir", tool: "search_files", args: `{"pattern":"*.go","directory":"pkg"}`, expected: `Finding files "*.go" in "pkg"`},
		{name: "search_files no dir", tool: "search_files", args: `{"pattern":"*.go"}`, expected: `Finding files "*.go"`},

		// Exec
		{name: "exec_run simple", tool: "exec_run", args: `{"command":"go","args":["test","./..."]}`, expected: `Running "go test ./..."`},
		{name: "exec_run no args", tool: "exec_run", args: `{"command":"ls"}`, expected: `Running "ls"`},

		// Git
		{name: "git_status", tool: "git_status", args: `{}`, expected: "Checking git status"},
		{name: "git_diff with path", tool: "git_diff", args: `{"path":"main.go"}`, expected: `Showing git diff for "main.go"`},
		{name: "git_diff no path", tool: "git_diff", args: `{}`, expected: "Showing git diff"},
		{name: "git_log", tool: "git_log", args: `{}`, expected: "Showing git log"},
		{name: "git_commit", tool: "git_commit", args: `{"message":"fix: resolve bug"}`, expected: `Committing "fix: resolve bug"`},

		// HTTP
		{name: "http_fetch GET", tool: "http_fetch", args: `{"url":"https://example.com"}`, expected: `Fetching GET "https://example.com"`},
		{name: "http_fetch POST", tool: "http_fetch", args: `{"url":"https://api.io/data","method":"POST"}`, expected: `Fetching POST "https://api.io/data"`},

		// Ask
		{name: "ask_user", tool: "ask_user", args: `{"question":"Continue?"}`, expected: "Asking user"},

		// Agent orchestration
		{name: "list_agents", tool: "list_agents", args: `{}`, expected: "Listing agents"},
		{name: "delegate_to_agent", tool: "delegate_to_agent", args: `{"agent":"coder","task":"fix it"}`, expected: `Delegating to "coder"`},
		{name: "spawn_agents", tool: "spawn_agents", args: `{"tasks":[]}`, expected: "Spawning agents"},

		// Skills
		{name: "load_skill", tool: "load_skill", args: `{"name":"review"}`, expected: `Loading skill "review"`},

		// Dynamic state tools
		{name: "state_get", tool: "project_state_get", args: `{"key":"count"}`, expected: `Getting state "count"`},
		{name: "state_set", tool: "project_state_set", args: `{"key":"count","value":42}`, expected: `Setting state "count"`},
		{name: "state_list", tool: "project_state_list", args: `{}`, expected: "Listing state keys"},

		// Dynamic task tools
		{name: "tasks_create", tool: "project_tasks_create", args: `{"title":"Do something"}`, expected: `Creating task "Do something"`},
		{name: "tasks_list", tool: "project_tasks_list", args: `{}`, expected: "Listing tasks"},
		{name: "tasks_get", tool: "project_tasks_get", args: `{"id":"abc"}`, expected: `Getting task "abc"`},
		{name: "tasks_claim", tool: "project_tasks_claim", args: `{"id":"abc"}`, expected: `Claiming task "abc"`},
		{name: "tasks_update", tool: "project_tasks_update", args: `{"id":"abc","status":"completed"}`, expected: `Updating task "abc"`},
		{name: "tasks_watch", tool: "project_tasks_watch", args: `{"id":"abc"}`, expected: `Watching task "abc"`},

		// Unknown / MCP tools
		{name: "unknown with args", tool: "mcp_weather", args: `{"city":"NYC"}`, expected: `Calling mcp_weather {"city":"NYC"}`},
		{name: "unknown no args", tool: "mcp_weather", args: ``, expected: "Calling mcp_weather"},

		// Edge cases
		{name: "empty args", tool: "fs_read", args: ``, expected: `Reading file ""`},
		{name: "invalid json", tool: "fs_read", args: `not-json`, expected: `Reading file ""`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatToolCall(tt.tool, tt.args)
			assert.Equal(t, tt.expected, result)
		})
	}
}
