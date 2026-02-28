package format

import (
	"encoding/json"
	"fmt"
	"strings"
)

// ToolFormatter produces a human-readable label from parsed tool arguments.
type ToolFormatter func(str func(string) string, args map[string]any) string

// ToolFormatters maps known tool names to their human-readable formatters.
var ToolFormatters = map[string]ToolFormatter{
	// Filesystem
	"fs_read":  func(s func(string) string, _ map[string]any) string { return fmt.Sprintf("Reading file %q", s("path")) },
	"fs_write": func(s func(string) string, _ map[string]any) string { return fmt.Sprintf("Writing file %q", s("path")) },
	"fs_edit":  func(s func(string) string, _ map[string]any) string { return fmt.Sprintf("Editing file %q", s("path")) },
	"fs_list": func(s func(string) string, _ map[string]any) string {
		return fmt.Sprintf("Listing directory %q", s("path"))
	},
	"fs_delete": func(s func(string) string, _ map[string]any) string {
		return fmt.Sprintf("Deleting %q", s("path"))
	},
	"fs_move": func(s func(string) string, _ map[string]any) string {
		return fmt.Sprintf("Moving %q to %q", s("source"), s("destination"))
	},
	"fs_copy": func(s func(string) string, _ map[string]any) string {
		return fmt.Sprintf("Copying %q to %q", s("source"), s("destination"))
	},
	"fs_mkdir": func(s func(string) string, _ map[string]any) string {
		return fmt.Sprintf("Creating directory %q", s("path"))
	},
	"fs_stat": func(s func(string) string, _ map[string]any) string {
		return fmt.Sprintf("Getting info for %q", s("path"))
	},
	"fs_diff": func(s func(string) string, _ map[string]any) string {
		return fmt.Sprintf("Comparing %q and %q", s("file_a"), s("file_b"))
	},
	"fs_patch": func(s func(string) string, _ map[string]any) string {
		return fmt.Sprintf("Patching file %q", s("path"))
	},

	// Search
	"search_content": func(s func(string) string, _ map[string]any) string {
		if dir := s("directory"); dir != "" {
			return fmt.Sprintf("Searching for %q in %q", s("pattern"), dir)
		}
		return fmt.Sprintf("Searching for %q", s("pattern"))
	},
	"search_files": func(s func(string) string, _ map[string]any) string {
		if dir := s("directory"); dir != "" {
			return fmt.Sprintf("Finding files %q in %q", s("pattern"), dir)
		}
		return fmt.Sprintf("Finding files %q", s("pattern"))
	},

	// Exec
	"exec_run": func(s func(string) string, args map[string]any) string {
		cmd := s("command")
		if argsArr, ok := args["args"]; ok {
			if arr, ok := argsArr.([]any); ok {
				parts := make([]string, 0, len(arr))
				for _, a := range arr {
					if v, ok := a.(string); ok {
						parts = append(parts, v)
					}
				}
				if len(parts) > 0 {
					cmd += " " + strings.Join(parts, " ")
				}
			}
		}
		return fmt.Sprintf("Running %q", Truncate(cmd, 80))
	},

	// Git
	"git_status": func(_ func(string) string, _ map[string]any) string { return "Checking git status" },
	"git_diff": func(s func(string) string, _ map[string]any) string {
		if p := s("path"); p != "" {
			return fmt.Sprintf("Showing git diff for %q", p)
		}
		return "Showing git diff"
	},
	"git_log": func(_ func(string) string, _ map[string]any) string { return "Showing git log" },
	"git_commit": func(s func(string) string, _ map[string]any) string {
		return fmt.Sprintf("Committing %q", Truncate(s("message"), 60))
	},

	// HTTP
	"http_fetch": func(s func(string) string, _ map[string]any) string {
		method := s("method")
		if method == "" {
			method = "GET"
		}
		return fmt.Sprintf("Fetching %s %q", method, Truncate(s("url"), 80))
	},

	// Ask
	"ask_user": func(_ func(string) string, _ map[string]any) string { return "Asking user" },

	// Agent orchestration
	"list_agents": func(_ func(string) string, _ map[string]any) string { return "Listing agents" },
	"delegate": func(_ func(string) string, args map[string]any) string {
		if tasks, ok := args["tasks"]; ok {
			if arr, ok := tasks.([]any); ok {
				var agents []string
				var taskLines []string
				for _, t := range arr {
					if m, ok := t.(map[string]any); ok {
						if agent, ok := m["agent"].(string); ok {
							agents = append(agents, agent)
							if task, ok := m["task"].(string); ok && task != "" {
								taskLines = append(taskLines, task)
							}
						}
					}
				}
				if len(agents) > 0 {
					title := fmt.Sprintf("Delegating to %s", strings.Join(agents, ", "))
					if len(taskLines) > 0 {
						return title + "\n" + strings.Join(taskLines, "\n")
					}
					return title
				}
			}
		}
		return "Delegating"
	},

	// Skills
	"load_skill": func(s func(string) string, _ map[string]any) string {
		return fmt.Sprintf("Loading skill %q", s("name"))
	},
}

// SuffixFormatters handles dynamically-namespaced tools (state, tasks).
var SuffixFormatters = []struct {
	Suffix string
	Fn     ToolFormatter
}{
	{"_state_get", func(s func(string) string, _ map[string]any) string {
		return fmt.Sprintf("Getting state %q", s("key"))
	}},
	{"_state_set", func(s func(string) string, _ map[string]any) string {
		return fmt.Sprintf("Setting state %q", s("key"))
	}},
	{"_state_list", func(_ func(string) string, _ map[string]any) string { return "Listing state keys" }},
	{"_tasks_create", func(s func(string) string, _ map[string]any) string {
		return fmt.Sprintf("Creating task %q", Truncate(s("title"), 60))
	}},
	{"_tasks_list", func(_ func(string) string, _ map[string]any) string { return "Listing tasks" }},
	{"_tasks_get", func(s func(string) string, _ map[string]any) string {
		return fmt.Sprintf("Getting task %q", s("id"))
	}},
	{"_tasks_claim", func(s func(string) string, _ map[string]any) string {
		return fmt.Sprintf("Claiming task %q", s("id"))
	}},
	{"_tasks_update", func(s func(string) string, _ map[string]any) string {
		return fmt.Sprintf("Updating task %q", s("id"))
	}},
	{"_tasks_watch", func(s func(string) string, _ map[string]any) string {
		return fmt.Sprintf("Watching task %q", s("id"))
	}},
}

// FormatToolCall returns a human-readable description of a tool invocation.
func FormatToolCall(toolName, argsJSON string) string {
	var args map[string]any
	if argsJSON != "" {
		_ = json.Unmarshal([]byte(argsJSON), &args)
	}

	str := func(key string) string {
		if v, ok := args[key]; ok {
			if s, ok := v.(string); ok {
				return s
			}
		}
		return ""
	}

	if fn, ok := ToolFormatters[toolName]; ok {
		return fn(str, args)
	}

	for _, sf := range SuffixFormatters {
		if strings.HasSuffix(toolName, sf.Suffix) {
			return sf.Fn(str, args)
		}
	}

	// Unknown / MCP tools — show name + truncated args.
	if argsJSON != "" {
		return fmt.Sprintf("Calling %s %s", toolName, Truncate(argsJSON, 80))
	}
	return fmt.Sprintf("Calling %s", toolName)
}
