# General
All multi-line text wraps using pre-word wrapping at the terminal width.
The UI resizes dynamically with the terminal. Minimum supported terminal width is 80 columns.
Color theme follows the GitHub terminal light theme.
## Layout

The screen is split into three vertically stacked regions:

1. **Header region** (top) — a fixed-height area displaying the Shelly logo and session status. This region is re-rendered in place as status changes (e.g., connection state). See **Header Region** for details.
2. **Messages region** (middle) — a grow-only block that holds all user messages, agent messages, tool calls, and sub-agent containers. New items are appended at the bottom of this block, increasing its height. This region is never re-rendered in place; it only grows downward. The terminal's native scroll is used for browsing history.
3. **Input region** (bottom) — a fixed-height area rendered immediately below the messages region. It contains, in order from top to bottom: the task panel (when active), the user input field, and the token counter. When the questions UI is active it replaces the entire input region. This region is re-rendered in place as its contents change (e.g., task status updates, spinner animation) without affecting the messages region above.

Because the messages region only appends and never re-renders, and the header/input regions re-render independently, there are no race conditions between agent output and the surrounding UI. The terminal auto-scrolls as new content arrives in the messages region.

## Header Region

The header region is a persistent status bar displayed at the top of the screen. It contains the Shelly logo and session metadata arranged on two lines.

The first line shows `shelly` in **bold accent** (#8250df). The second line shows status items separated by ` · ` (space-dot-space) in **dim** (#656d76):

- **Agent**: the name of the entry agent (e.g., `coder`)
- **Model**: the model identifier (e.g., `claude-opus-4-6`)
- **Provider**: the provider name (e.g., `my-anthropic`)
- **Connection status**: `● Connected` in **green** (#1a7f37) or `● Disconnected` in **red** (#cf222e)
- **Config**: the path to the loaded config file (e.g., `~/.shelly/config.yaml`), or `No config` if none is loaded

|                                                                                                                |
|                                                                                                                |
| shelly                                                                                                         |
| coder · claude-opus-4-6 · my-anthropic · ● Connected · ~/.shelly/config.yaml                                  |
|                                                                                                                |
|                                                                                                                |

When the config has no agents configured, the agent field is omitted. When no config is loaded, the line reads `No config · ● Disconnected`.

|                                                                                                                |
|                                                                                                                |
| shelly                                                                                                         |
| No config · ● Disconnected                                                                                    |
|                                                                                                                |
|                                                                                                                |

## Agent Name Colors
Each sub-agent is assigned a distinct color for its name, determined by the order in which it first appears in the session. The same color is applied consistently everywhere the agent name is rendered: container headers, task list parentheticals, delegation descriptions, and finished summaries. This allows quick visual identification across concurrent activity.

Color palette (cycled round-robin by agent registration order):

| Slot | Color name | Hex |
|------|-----------|-----|
| 1 | Blue | #0969da |
| 2 | Green | #1a7f37 |
| 3 | Orange | #bc4c00 |
| 4 | Pink | #bf3989 |
| 5 | Teal | #0a7480 |
| 6 | Purple | #8250df |

The entry/top-level agent always uses the primary foreground color (#24292f) and is not assigned a slot color.

# User Input
The user input lives inside the input region (see Layout). Below the user input there is the total sum of used tokens of the session. Large token counts are formatted (e.g., 1.2k, 15k).
Enter submits the message. Multi-line input is supported by pressing Shift+Enter or Alt+Enter to insert a newline.
The token counter is hidden when a picker (file or command) is open.
The input remains enabled while the agent is working. Messages sent during agent processing are queued and delivered when the agent is ready. Press Escape to interrupt the agent.
Escape is consumed by the innermost active context first: an open picker is dismissed before the agent is interrupted.

## Current Tasks
When the agent has active tasks, a task panel is displayed immediately above the user input. It opens with a title line showing the word **Tasks** followed by counts for each status: `N pending  N in progress  N completed`. Below the title, tasks are listed sorted by status: pending first, in progress second, completed last. Each task uses an icon to reflect its state:
- `○` — pending (not yet started)
- `⣾` — in progress (braille spinner, animates in **magenta**)
- `✓` — completed; both the icon and text are rendered in **light gray** (#656d76)

A maximum of 6 tasks are shown at a time. When there are more, the list shows the 6 most relevant (prioritizing pending and in-progress over completed). The task list disappears once all tasks are done.

The agent assigned to a task is shown in parentheses immediately after the task text. If no agent is assigned, the parenthetical is omitted.

|                                                                                                                |
|                                                                                                                |
| Tasks  1 pending  1 in progress  1 completed                                                                   |
| ○ Implementing login endpoint (coder)                                                                          |
| ⣾ Researching authentication patterns (researcher)                                                             |
| ✓ Writing unit tests (coder)                                                                                   |
| -------------------------------------------------------------------------------------------------------------- |
| >  (placeholder for the user input)                                                                            |
| -------------------------------------------------------------------------------------------------------------- |
| 19000 tokens                                                                                                   |
|                                                                                                                |
|                                                                                                                |

## Simple User Input
|                                                                                                                |
|                                                                                                                |
| -------------------------------------------------------------------------------------------------------------- |
| >  (placeholder for the user input)                                                                            |
| -------------------------------------------------------------------------------------------------------------- |
| 19000 tokens                                                                                                   |
|                                                                                                                |
|                                                                                                                |

## File Input
When the user types @ at the cursor position the file picker is displayed, the user can select a file from the file system and the file path is inserted into the user input. The user can reference multiple files in a single message; each @ triggers a separate picker. File paths in the sent message are displayed highlighted.
The list filters as the user types. Press Escape to dismiss the picker. The currently selected item is displayed in **bold and underlined**. The picker shows a maximum of 4 visible items; the rest are accessible by scrolling with the up and down arrow keys. When no files match the filter, the picker displays "No files".
|                                                                                                                |
|                                                                                                                |
| -------------------------------------------------------------------------------------------------------------- |
| >  @filename.txt                                                                                               |
| -------------------------------------------------------------------------------------------------------------- |
| filename.txt                                                                                                   |
| filename.json                                                                                                  |
|                                                                                                                |
|                                                                                                                |

## Command Input
When the user types / the command picker is displayed. The list filters as the user types; navigate with the up and down arrow keys. Pressing Enter on a highlighted command executes it immediately — the picker closes, the command runs without requiring a second Enter, and a command log entry is appended to the message area (see **Command message**). Press Escape to dismiss the picker without executing. The currently selected item is displayed in **bold and underlined**. The picker shows a maximum of 4 visible items; the rest are accessible by scrolling with the up and down arrow keys.

### Picker open
|                                                                                                                |
|                                                                                                                |
| -------------------------------------------------------------------------------------------------------------- |
| > /                                                                                                            |
| -------------------------------------------------------------------------------------------------------------- |
| /help                                                                                                          |
| /clear                                                                                                         |
| /exit                                                                                                          |
|                                                                                                                |
|                                                                                                                |

# Messages
Messages can be displayed as markdown. Agent text streams in chunk by chunk as it is generated.
There must be at least 1 line of padding between messages.

## Empty state
Before the first message, the message area displays a seashell ASCII art drawing.


## User message
|                                                                                                                |
|                                                                                                                |
| 🧑‍💻 User                                                                                                        |
|  └ Lorem ipsum dolor sit amet, consectetur adipiscing elit. Pellentesque rutrum convallis risus. Pellentesque  |
|    sit amet ipsum erat. Nulla aliquam elit feugiat, ornare est sed, semper augue. Suspendisse et               |
|    neque rhoncus, bibendum lacus eleifend, scelerisque augue. Nam ut est dolor. Mauris ullamcorper, neque      |
|    seddignissim ornare, neque mauris auctor ex, ac lobortis elit sem eget purus. Sed eleifend mattis sem in    |
|    fringilla. Fusce condimentum risus ut maximus feugiat.                                                      |
|                                                                                                                |
|                                                                                                                |

## Command message
When a command is executed it appends a log entry to the message area so users can track which commands have run during the session. The entry shows the command name and any arguments in **dim** (#656d76), prefixed with a `⌘` symbol.
|                                                                                                                |
|                                                                                                                |
| ⌘ /clear                                                                                                       |
|                                                                                                                |
|                                                                                                                |

## Agent processing indicator
A standalone loading indicator shown while the agent is processing but before any agent container is created. The spinner uses a random whimsical message selected once from a pool of options (e.g., "Thinking...", "Pondering the cosmos...", "Consulting ancient scrolls..."). Both the spinner and message are styled in **magenta** (#8250df). The spinner cycles through braille animation frames: ⣾ ⣽ ⣻ ⢿ ⡿ ⣟ ⣯ ⣷.
|                                                                                                                |
|                                                                                                                |
|   ⣾ Pondering the cosmos...                                                                                   |
|                                                                                                                |
|                                                                                                                |

## Agent thinking message
A loading indicator shown when an agent container is created but has no items yet. Displayed with the agent's emoji prefix, name, and a braille spinner in **magenta**.
|                                                                                                                |
|                                                                                                                |
| 🤖 <agent name> is thinking... ⣾                                                                              |
|                                                                                                                |
|                                                                                                                |

## Agent reasoning message
When the agent produces reasoning text alongside tool calls, it is displayed as a markdown-rendered block. The header shows the agent's emoji prefix and name. Content is indented under a tree corner connector (`└`), with continuation lines indented by 3 spaces.
|                                                                                                                |
|                                                                                                                |
| 🤖 <agent name>                                                                                                |
|  └ Lorem ipsum dolor sit amet, consectetur adipiscing elit. Pellentesque rutrum convallis risus. Pellentesque  |
|    sit amet ipsum erat. Nulla aliquam elit feugiat, ornare est sed, semper augue. Suspendisse et               |
|    neque rhoncus, bibendum lacus eleifend, scelerisque augue. Nam ut est dolor. Mauris ullamcorper, neque      |
|    seddignissim ornare, neque mauris auctor ex, ac lobortis elit sem eget purus. Sed eleifend mattis sem in    |
|    fringilla. Fusce condimentum risus ut maximus feugiat.                                                      |
|                                                                                                                |
|                                                                                                                |

## Agent plan message
When an agent configured with the 📝 prefix produces text alongside tool calls, it is displayed as a plan. Content is markdown-rendered. Both the header and rendered content are styled in **light gray** (#656d76).
|                                                                                                                |
|                                                                                                                |
| 📝 <agent name> plan:                                                                                          |
|  └ Lorem ipsum dolor sit amet, consectetur adipiscing elit. Pellentesque rutrum convallis risus. Pellentesque  |
|    sit amet ipsum erat. Nulla aliquam elit feugiat, ornare est sed, semper augue.                              |
|                                                                                                                |
|                                                                                                                |

## Agent answer message
The final agent response (an assistant message with no tool calls). The header shows the agent's emoji prefix and name in **bold primary foreground** (#24292f). Content is markdown-rendered and indented under a tree corner connector, same as reasoning messages. Sub-agent final answers are displayed inside their container instead of being committed to scrollback.
|                                                                                                                |
|                                                                                                                |
| 🤖 <agent name>                                                                                                |
|  └ Lorem ipsum dolor sit amet, consectetur adipiscing elit. Pellentesque rutrum convallis risus. Pellentesque  |
|    sit amet ipsum erat. Nulla aliquam elit feugiat, ornare est sed, semper augue. Suspendisse et               |
|    neque rhoncus, bibendum lacus eleifend, scelerisque augue.                                                  |
|                                                                                                                |
|                                                                                                                |

## Tool call message
Tool names are displayed as human-readable descriptions rather than raw function signatures. Known tools have custom formatters (e.g., `fs_read` → `Reading file "/path/to/file.txt"`, `exec_run` → `Running "go test ./..."`). Unknown and MCP tools show `Calling <tool_name> <truncated args>`. The tool description is displayed in **bold**.

### While running
The tool call shows the formatted description in **bold**, elapsed time in **dim**, and a braille spinner in **magenta**.
|                                                                                                                |
|                                                                                                                |
| 🔧 Reading file "/path/to/file.txt" 0.5s ⣾                                                                   |
|                                                                                                                |
|                                                                                                                |

### After finished
The result is displayed under a tree corner connector (`└`) in **dim** (#656d76). Results are truncated to 200 characters. Elapsed time is appended as `in <time>` suffix. Error results are displayed in **red** (#cf222e) instead of dim.
|                                                                                                                |
|                                                                                                                |
| 🔧 Reading file "/path/to/file.txt"                                                                           |
|  └ Read 200 lines from file /path/to/file.txt in 0.5s                                                         |
|                                                                                                                |
|                                                                                                                |

### Multi-line tool descriptions
Some tools produce multi-line descriptions. The first line is the title; subsequent lines are detail lines displayed with tree connectors (`├─` and `│`) in **dim**. The delegation tool uses this format to show agent names and task descriptions.
|                                                                                                                |
|                                                                                                                |
| 🔧 Delegating to researcher, coder 1.2s ⣾                                                                    |
|  ├─ Research the authentication patterns in the codebase                                                       |
|  │  and identify all JWT usage                                                                                 |
|  ├─ Implement the login endpoint using the patterns found                                                      |
|  │  by the researcher                                                                                          |
|                                                                                                                |
|                                                                                                                |

## Multiple tool calls (tool group)
Parallel calls of the **same tool** are grouped under a "Using tools" parent and displayed as a tree. Sequential calls of different tools are displayed individually (flat), one after another. Each call within the group follows the same display rules as a single tool call.

### While running
|                                                                                                                |
|                                                                                                                |
| 🔧 Using tools                                                                                                 |
| ├─ Reading file "/path/to/foo.txt"                                                                            |
| │  └ Read 150 lines from file /path/to/foo.txt in 0.3s                                                        |
| ├─ Reading file "/path/to/bar.txt" 0.5s ⣾                                                                    |
| └ Reading file "/path/to/baz.txt" 0.2s ⣾                                                                     |
|                                                                                                                |
|                                                                                                                |

### After finished
The group collapses to a summary showing the number of tools and total elapsed time.
|                                                                                                                |
|                                                                                                                |
| 🔧 Used tools                                                                                                  |
|  └ Finished with 3 tools in 0.5s                                                                               |
|                                                                                                                |
|                                                                                                                |

## Error states
### Tool call error
When a tool call fails, the result is displayed in **red** (#cf222e) instead of the usual dim style.
|                                                                                                                |
|                                                                                                                |
| 🔧 Fetching GET "https://api.example.com/data"                                                                |
|  └ Error: connection refused: could not reach api.example.com in 0.5s                                          |
|                                                                                                                |
|                                                                                                                |

### Error block
When a send or response error occurs (not a tool error), the error message is displayed with a **thick red left border** (#cf222e) and left padding.
|                                                                                                                |
|                                                                                                                |
| ┃ error: connection timed out                                                                                  |
|                                                                                                                |
|                                                                                                                |

# Sub agents
Sub agents are displayed inside agent containers. Each agent container accumulates display items chronologically: reasoning, tool calls, plan items, and nested sub-agent items. Top-level agent containers render their items sequentially in the message area with no container header (each item carries its own visual identity). Sub-agent containers are nested inside their parent's container with a header, indentation, and windowing.

## Agent container
Top-level agent containers have no persistent header. Items (reasoning, tool calls, etc.) are rendered one after another, each with their own prefix and formatting. When the container has no items yet, it shows the "is thinking..." indicator.
|                                                                                                                |
|                                                                                                                |
| 🤖 <agent name>                                                                                                |
|  └ I'll read the file and make the requested changes.                                                          |
|                                                                                                                |
| 🔧 Reading file "/path/to/file.txt"                                                                           |
|  └ Read 200 lines in 0.3s                                                                                     |
|                                                                                                                |
| 🔧 Editing file "/path/to/file.txt"                                                                           |
|  └ Applied edit in 0.1s                                                                                        |
|                                                                                                                |
|                                                                                                                |

When the agent finishes, the container collapses to show the agent name, the final answer/result text (markdown-rendered), and the elapsed time below the result in **dim**:
|                                                                                                                |
|                                                                                                                |
| 🤖 <agent name>                                                                                                |
|  └ <final answer or result text from the agent>                                                                |
|    Finished in 5.3s                                                                                            |
|                                                                                                                |
|                                                                                                                |

## Delegation to sub agent
The delegate tool call is displayed in the parent agent's container as a regular tool call. It uses the multi-line tool description format to show agent names as the title and task descriptions as detail lines.

### While delegating
|                                                                                                                |
|                                                                                                                |
| 🔧 Delegating to researcher 2.1s ⣾                                                                           |
|  ├─ Research the authentication patterns in the codebase                                                       |
|  │  and identify all JWT usage                                                                                 |
|                                                                                                                |
|                                                                                                                |

### After delegation finished
|                                                                                                                |
|                                                                                                                |
| 🔧 Delegating to researcher                                                                                   |
|  ├─ Research the authentication patterns in the codebase                                                       |
|  │  and identify all JWT usage                                                                                 |
|  └ <result text> in 5.3s                                                                                      |
|                                                                                                                |
|                                                                                                                |

## Sub-agent container structure
Sub-agent containers are nested inside their parent's container. While active, the header shows the agent name in **magenta** (#8250df) with a braille spinner. All items inside the container are indented with `  │ ` (2 spaces + tree pipe). The container shows a maximum of 4 visible items; older items are hidden behind a "... N more items" indicator in **dim**. Items inside the container use the same rendering as top-level items (reasoning, tool calls, groups) but at a reduced width. Reasoning text from sub-agents is always rendered as markdown text — never as raw JSON.

### Sub-agent thinking (no items yet)
|                                                                                                                |
|                                                                                                                |
| 🤖 <sub agent name> is thinking... ⣾                                                                          |
|                                                                                                                |
|                                                                                                                |

### Sub-agent with items (active)
|                                                                                                                |
|                                                                                                                |
| 🤖 <sub agent name> ⣾                                                                                         |
|   ... 3 more items                                                                                             |
|   │ 🤖 <sub agent name>                                                                                        |
|   │  └ I found the relevant patterns in the codebase.                                                          |
|   │ 🔧 Reading file "/path/to/file.txt"                                                                        |
|   │  └ Read 200 lines in 0.3s                                                                                  |
|   │ 🔧 Searching for "JWT" in "/src" 0.5s ⣾                                                                    |
|                                                                                                                |
|                                                                                                                |

### Sub-agent finished
When a sub-agent finishes, its container collapses to show the agent header, the final answer/result text (markdown-rendered), and the elapsed time below that result in **dim**.
|                                                                                                                |
|                                                                                                                |
| 🤖 <sub agent name>                                                                                            |
|  └ <final answer or result text from the sub-agent>                                                            |
|    Finished in 5.3s                                                                                            |
|                                                                                                                |
|                                                                                                                |

## Full sub-agent running example
A complete example showing two sub-agents running concurrently inside the parent container. Each sub-agent has its own windowed view with indented items.
|                                                                                                                |
|                                                                                                                |
| 🤖 <sub agent 1 name> ⣾                                                                                       |
|   ... 2 more items                                                                                             |
|   │ 🔧 Used tools                                                                                              |
|   │  └ Finished with 2 tools in 1.2s                                                                           |
|   │ 🤖 <sub agent 1 name>                                                                                      |
|   │  └ Analysis complete, found the relevant patterns.                                                          |
|   │ 🔧 Reading file "/path/to/file.txt" 0.5s ⣾                                                                |
|                                                                                                                |
| 🤖 <sub agent 2 name> ⣾                                                                                       |
|   │ 🔧 Used tools                                                                                              |
|   │  └ Finished with 3 tools in 2.1s                                                                           |
|   │ 🤖 <sub agent 2 name>                                                                                      |
|   │  └ Implementing the changes based on findings.                                                              |
|   │ 🔧 Editing file "/path/to/file.txt" 0.3s ⣾                                                                |
|                                                                                                                |
|                                                                                                                |

## After sub-agents finished
When a sub agent finishes, its container collapses to show the header, the agent's final answer/result (markdown-rendered), and the elapsed time below the result in **dim**.
|                                                                                                                |
|                                                                                                                |
| 🤖 <sub agent 1 name>                                                                                          |
|  └ <final answer or result text from sub-agent 1>                                                              |
|    Finished in 5.3s                                                                                            |
|                                                                                                                |
| 🤖 <sub agent 2 name>                                                                                          |
|  └ <final answer or result text from sub-agent 2>                                                              |
|    Finished in 4.1s                                                                                            |
|                                                                                                                |
|                                                                                                                |

# Questions

The user is expected to answer questions in a sequential batch, each question and answers are displayed in tabs, each tab title is a single word related to the question and the content of the tab is the question and the answers options.
The user can navigate through the tabs using the left and right arrow keys, and select an answer using the up and down arrow keys, then the user can confirm the answer using the enter key.
When the user confirms an answer, the ui moves to the next question tab, until all questions are answered.
When the user confirms the last question, the ui moves to the Confirm tab. On this tab the user can review the answers and confirm them to finish the interview.
When the user enters a custom answer in the Confirm tab it is considered as acceptance of the answers and the interview is finished with all selected answers and the custom answer.
The user can dismiss the questions UI by pressing Escape. Dismissal notifies the agent that the user rejected the questions.

### Tab notation
- `*TabName*` — active (selected) tab
- `[TabName]` — inactive tab

## Single Choice Answer
|                                                                                                                |
|                                                                                                                |
| *One* [Word] [Per] [Question] [Confirm]                                                                        |
|                                                                                                                |
|                                                                                                                |
| What is your answer for question One?                                                                          |
|                                                                                                                |
| 1. The first answer option                                                                                     |
| 2. The second answer option                                                                                    |
| 3. (Place holder for a multiple line text input, for a free form answer)                                       |
|                                                                                                                |
| ← Left tab, → Right tab, ↑ Up, ↓ Down, ↵ Confirm, Esc Dismiss                                                  |
|                                                                                                                |

## Multiple Choice Answers
|                                                                                                                |
|                                                                                                                |
| [One] *Word* [Per] [Question] [Confirm]                                                                        |
|                                                                                                                |
|                                                                                                                |
| What are your answers for question Word?                                                                       |
|                                                                                                                |
| 1. [X] The first answer option                                                                                 |
| 2. [ ] The second answer option                                                                                |
| 3. [X] The third answer option                                                                                 |
|                                                                                                                |
| ← Left tab, → Right tab, ↑ Up, ↓ Down, Space Toggle, ↵ Confirm, Esc Dismiss                                    |
|                                                                                                                |

## Questions Confirm tab
|                                                                                                                |
|                                                                                                                |
| [One] [Word] [Per] [Question] *Confirm*                                                                        |
|                                                                                                                |
|                                                                                                                |
| Confirm your answers:                                                                                          |
|                                                                                                                |
| 1. This is the question one?                                                                                   |
|  └ This is the answer to the question one.                                                                     |
| 2. This is the question word?                                                                                  |
|  └ This is the answer to the question word.                                                                    |
| 3. This is the question per?                                                                                   |
|  └ This is the answer to the question per.                                                                     |
| 4. This is the question question?                                                                              |
|  └ This is the answer to the question question.                                                                |
|                                                                                                                |
| Are you happy with your answers?                                                                               |
| 1. Yes                                                                                                         |
| 2. No                                                                                                          |
| 3. (Place holder for a multiple line text input, for a free form answer)                                       |
|                                                                                                                |
| ← Left tab, → Right tab, ↑ Up, ↓ Down, ↵ Confirm, Esc Dismiss                                                   |
|                                                                                                                |

- Selecting **Yes** sends all answers to the agent and closes the questions UI.
- Selecting **No** notifies the agent that the user rejected the answers. The agent decides how to proceed.
- Entering a **custom answer** is treated as acceptance, sending all selected answers plus the custom text to the agent.

# Config Wizard

The config wizard is a standalone TUI launched via `shelly config`. It manages Shelly's YAML configuration through a multi-screen wizard. The wizard uses a **screen stack pattern**: selecting a menu item pushes a new screen; pressing Escape or completing an action pops back to the previous screen.

Global keybinding: **Ctrl+C** quits the wizard from any screen.

## Main Menu

Displays the wizard title in **bold accent** (#8250df), a **dim** summary of loaded config items (e.g., "2 providers, 1 agent"), and a vertical list of 6 menu items. The currently selected item is highlighted with a `>` cursor prefix using the selected answer style. On success after save, displays "Config saved successfully!" and exits.

|                                                                                                                |
|                                                                                                                |
| Shelly Config Wizard                                                                                           |
|                                                                                                                |
| 2 providers, 1 agent, 1 MCP server                                                                            |
|                                                                                                                |
| > Providers                                                                                                    |
|   Agents                                                                                                       |
|   MCP Servers                                                                                                  |
|   Settings                                                                                                     |
|   Review & Save                                                                                                |
|   Quit                                                                                                         |
|                                                                                                                |
| ↑/↓: navigate  Enter: select  q: quit                                                                         |
|                                                                                                                |

When no config is loaded, the summary reads "No configuration loaded" in **dim**.

## List Screens (Providers, Agents, MCP Servers)

All three entity types share the same list screen pattern. Items are displayed in a vertical list with a `>` cursor. The last entry is always `+ Add new <entity>` in **dim** (or selected style when focused). Each item shows its name and a parenthetical detail:
- Providers: `<name> (<kind>)` (e.g., "my-anthropic (anthropic)")
- Agents: `<name> - <description>` (description truncated to 40 chars)
- MCP Servers: `<name> (<transport>)` where transport is "stdio" or "sse"

### Providers list
|                                                                                                                |
|                                                                                                                |
| Providers                                                                                                      |
|                                                                                                                |
| > my-anthropic (anthropic)                                                                                     |
|   my-openai (openai)                                                                                           |
|   + Add new provider                                                                                           |
|                                                                                                                |
| Enter: edit  d: delete  Esc: back                                                                              |
|                                                                                                                |

### Agents list
|                                                                                                                |
|                                                                                                                |
| Agents                                                                                                         |
|                                                                                                                |
| > coder - General purpose coding agent                                                                         |
|   researcher - Searches and summarizes c...                                                                    |
|   + Add new agent                                                                                              |
|                                                                                                                |
| Enter: edit  d: delete  Esc: back                                                                              |
|                                                                                                                |

### MCP Servers list
|                                                                                                                |
|                                                                                                                |
| MCP Servers                                                                                                    |
|                                                                                                                |
| > my-mcp (stdio)                                                                                               |
|   remote-mcp (sse)                                                                                             |
|   + Add new MCP server                                                                                         |
|                                                                                                                |
| Enter: edit  d: delete  Esc: back                                                                              |
|                                                                                                                |

**Keybindings** (identical for all list screens):
- **↑/k**, **↓/j**: Move cursor
- **Enter**: Edit selected item, or add new item if on the `+ Add new` entry
- **d**: Delete the selected item (cursor adjusts to stay in bounds)
- **Esc**: Return to parent screen

## Form Screens

Forms are used for editing/creating Providers, Agents, MCP Servers, and Settings. A form displays its title in **bold accent**, followed by vertically stacked labeled fields. The focused field's label is in **accent** color; unfocused labels are plain **bold**. Labels are fixed at 22 characters wide.

### Field types

- **TextField**: Single-line text input (char limit 256, display width 50). Pressing Enter advances to the next field.
- **IntField**: Single-line text input restricted to integers (char limit 20). Validation rejects non-integer values.
- **FloatField**: Single-line text input for decimal numbers (char limit 20).
- **BoolField**: Toggle displayed as `[x]` / `[ ]`. Toggled with **Enter** or **Space**.
- **SelectField**: Horizontal inline option list. Navigate with **←/h** and **→/l**. The selected option is **bold** (or accent when focused); others are dim.
- **MultiSelectField**: Vertical checkbox list. Navigate with **↑/k** and **↓/j**. Toggle with **Space**. Checked items show `[x]`, unchecked `[ ]`. The cursor item is highlighted; checked items are **bold**.
- **TextAreaField**: Multi-line text input (4 rows, char limit 4096). **Enter** inserts a newline instead of advancing.

### Form navigation
- **Tab**: Advance to the next field
- **Shift+Tab**: Return to the previous field
- **↑/↓** or **Enter**: Also advance between fields (except inside MultiSelectField and TextAreaField, where arrows and Enter are handled internally)
- **Ctrl+S**: Validate and submit the form
- **Esc**: Cancel the form and return to the list screen
- Tabbing past the last field submits the form (same as Ctrl+S)
- Validation errors are displayed in **red** (#cf222e) below the fields

### Provider form
|                                                                                                                |
|                                                                                                                |
| Add Provider                                                                                                   |
|                                                                                                                |
| Kind                    anthropic  openai  grok  gemini                                                        |
| Name                    my-anthropic                                                                           |
| API Key                 ${ANTHROPIC_API_KEY}                                                                   |
| Model                   claude-opus-4-6                                                                        |
| Base URL                                                                                                       |
| Context Window                                                                                                 |
| Rate Limit Retries                                                                                             |
| Rate Limit Delay                                                                                               |
|                                                                                                                |
| ↑/↓: navigate  Tab: next  Shift+Tab: prev  Ctrl+S: save  Esc: cancel                                         |
|                                                                                                                |

Fields: Kind (select), Name (text, required), API Key (text), Model (text), Base URL (text), Context Window (int), Rate Limit Retries (int), Rate Limit Delay (text).

### Agent form
|                                                                                                                |
|                                                                                                                |
| Add Agent                                                                                                      |
|                                                                                                                |
| Name                    coder                                                                                  |
| Description             General purpose coding agent                                                           |
| Instructions            (multi-line text area)                                                                 |
| Provider                my-anthropic                                                                           |
| Prefix                  🤖                                                                                     |
| Toolboxes               [x] coding  [ ] search  [x] mcp-server-1                                              |
| Effects                 [x] compact  [ ] trim                                                                  |
| Max Iterations                                                                                                 |
| Max Delegation Depth                                                                                           |
| Context Threshold                                                                                              |
|                                                                                                                |
| ↑/↓: navigate  Tab: next  Shift+Tab: prev  Ctrl+S: save  Esc: cancel                                         |
|                                                                                                                |

Fields: Name (text, required), Description (text), Instructions (textarea), Provider (select from configured providers), Prefix (text), Toolboxes (multi-select from built-in + MCP server names), Effects (multi-select from known effect kinds), Max Iterations (int), Max Delegation Depth (int), Context Threshold (float).

### MCP Server form
|                                                                                                                |
|                                                                                                                |
| Add MCP Server                                                                                                 |
|                                                                                                                |
| Name                    my-mcp                                                                                 |
| Transport               stdio  sse                                                                             |
| Command                 npx mcp-server                                                                         |
| Args                    --port 3000                                                                            |
| URL                                                                                                            |
|                                                                                                                |
| ↑/↓: navigate  Tab: next  Shift+Tab: prev  Ctrl+S: save  Esc: cancel                                         |
|                                                                                                                |

Fields: Name (text, required), Transport (select: stdio/sse), Command (text), Args (text, space-separated), URL (text). On save, if transport is "sse" then Command and Args are cleared and only URL is kept; if "stdio" then URL is cleared.

### Settings form
|                                                                                                                |
|                                                                                                                |
| Settings                                                                                                       |
|                                                                                                                |
| Entry Agent             coder  researcher                                                                      |
| Permissions File        permissions.json                                                                       |
| Git Work Dir            .                                                                                      |
| Browser Headless        [x]                                                                                    |
| Anthropic Context Window                                                                                       |
| OpenAI Context Window                                                                                          |
| Grok Context Window                                                                                            |
| Gemini Context Window                                                                                          |
|                                                                                                                |
| ↑/↓: navigate  Tab: next  Shift+Tab: prev  Ctrl+S: save  Esc: cancel                                         |
|                                                                                                                |

Fields: Entry Agent (select from configured agent names; shows "(no agents)" if none exist), Permissions File (text), Git Work Dir (text), Browser Headless (bool), Anthropic/OpenAI/Grok/Gemini Context Window (int each). Empty context window fields are omitted from config; set values override provider defaults.

## Review & Save Screen

Displays the current config as a **dim** YAML preview with scrolling (max 20 visible lines). Scroll position is shown as "showing lines X-Y of Z". Validation errors (from `Config.Validate()`) are displayed in **red** above the YAML preview. Action buttons are displayed horizontally at the bottom as `[ Save ]  [ Save & Quit ]  [ Back ]`. The selected action is highlighted with the selected style.

|                                                                                                                |
|                                                                                                                |
| Review & Save                                                                                                  |
|                                                                                                                |
| providers:                                                                                                     |
|   - kind: anthropic                                                                                            |
|     name: my-anthropic                                                                                         |
|     api_key: ${ANTHROPIC_API_KEY}                                                                              |
|     model: claude-opus-4-6                                                                                     |
| agents:                                                                                                        |
|   - name: coder                                                                                                |
|     ...                                                                                                        |
|                                                                                                                |
| ↑/↓: scroll  showing lines 1-12 of 25                                                                         |
|                                                                                                                |
| [ Save ]  [ Save & Quit ]  [ Back ]                                                                           |
|                                                                                                                |
| ←/→: select action  Enter: confirm  Ctrl+S: save  Esc: back                                                   |
|                                                                                                                |

### With validation errors
|                                                                                                                |
|                                                                                                                |
| Review & Save                                                                                                  |
|                                                                                                                |
| Validation: entry_agent references unknown agent "missing"                                                     |
|                                                                                                                |
| ...YAML preview...                                                                                             |
|                                                                                                                |
| Fix validation errors before saving                                                                            |
|                                                                                                                |
| ←/→: select action  Enter: confirm  Ctrl+S: save  Esc: back                                                   |
|                                                                                                                |

When validation errors exist, Save and Save & Quit are disabled; the button area shows "Fix validation errors before saving" in **red** instead.

**Keybindings:**
- **↑/k**, **↓/j**: Scroll YAML preview
- **←/h**, **→/l**: Select action button
- **Enter**: Execute selected action
- **Ctrl+S**: Save without quitting (shortcut)
- **Esc**: Return to main menu

**Actions:**
- **Save**: Writes config to file, shows "Config saved!" status message, stays on screen
- **Save & Quit**: Writes config to file and exits the wizard
- **Back**: Returns to main menu without saving

## Template Picker (`shelly init`)

An interactive template selector launched via `shelly init` (without `--template`). Displays a vertical list of available templates with name and description. The `--list` flag prints templates non-interactively and exits.

|                                                                                                                |
|                                                                                                                |
| Select a template                                                                                              |
|                                                                                                                |
| > simple-assistant  A minimal single-agent setup                                                               |
|   multi-agent       Multi-agent with delegation                                                                |
|   research          Research-focused agent team                                                                |
|                                                                                                                |
| ↑/↓: navigate  Enter: select  q: quit                                                                         |
|                                                                                                                |

The selected item shows `>` cursor, highlighted text, and the description in **dim** to its right. Unselected items show only the name (no description). On success, displays "Initialized "<name>" template in <dir>" and exits. Errors are shown in **red**.

**Keybindings:**
- **↑/k**, **↓/j**: Move cursor
- **Enter**: Apply selected template
- **q** or **Ctrl+C**: Quit without applying

When no templates are available, displays "No templates available" in **dim**.

# Put Together

A complete session walkthrough showing how all UI elements combine. The user asks Shelly to add JWT authentication to an API. The session progresses through input, processing, delegation, concurrent sub-agent work, task tracking, and the final answer.

## 1. Empty state — session start
The header region is always visible. The messages area shows the ASCII art until the first message.
```
shelly
coder · claude-opus-4-6 · my-anthropic · ● Connected · ~/.shelly/config.yaml

       __       ____
  ___ / /  ___ / / /_ __
 (_-</ _ \/ -_) / / // /
/___/_//_/\__/_/_/\_, /
                 /___/


```

## 2. User sends a message
```
🧑‍💻 User
 └ Add JWT authentication to the /api routes. Use the existing user model
   and add refresh token support.

```

## 3. Agent processing indicator
```
  ⣾ Consulting ancient scrolls...

```

## 4. Agent reasoning and first tool calls
```
🤖 shelly
 └ I'll research the existing codebase first, then implement JWT auth with
   refresh tokens. Let me delegate this to specialized agents.

🔧 Used tools
 └ Finished with 3 tools in 0.3s

```

## 5. Delegation with concurrent sub-agents and task panel
The parent agent delegates to two sub-agents. The task panel appears above user input as tasks are created.

```
🔧 Delegating to researcher, coder 4.2s ⣾
 ├─ Research JWT best practices and review the existing auth
 │  middleware in the codebase
 ├─ Implement JWT authentication with refresh tokens on the
 │  /api routes using the existing user model

🤖 researcher ⣾
  ... 2 more items
  │ 🤖 researcher
  │  └ Found the existing middleware pattern in src/middleware/auth.go.
  │    The project uses chi router with middleware chaining.
  │ 🔧 Searching for "middleware" in "src/" 0.5s ⣾

🤖 coder ⣾
  │ 🔧 Used tools
  │  └ Finished with 2 tools in 1.2s
  │ 🤖 coder
  │  └ Implementing JWT service based on researcher findings.
  │ 🔧 Editing file "src/services/jwt.go" 0.3s ⣾

Tasks  1 pending  2 in progress  1 completed
○ Write integration tests (coder)
⣾ Researching authentication patterns (researcher)
⣾ Implementing JWT auth service (coder)
✓ Analyze existing user model (researcher)
──────────────────────────────────────────────────────────────────────────────────
>
──────────────────────────────────────────────────────────────────────────────────
15.2k tokens
```

## 6. Sub-agents finish, parent resumes
```
🤖 researcher
 └ The codebase uses chi router v5 with middleware chaining. Auth middleware
   is in src/middleware/auth.go. The user model has ID, Email, and
   PasswordHash fields. Recommend using golang-jwt/jwt/v5.
   Finished in 8.1s

🤖 coder
 └ Implemented JWT auth: added src/services/jwt.go (token generation and
   validation), src/middleware/jwt.go (route protection), and updated
   src/routes/api.go with auth middleware. Refresh token rotation is stored
   in the user_sessions table.
   Finished in 12.4s

🔧 Delegating to researcher, coder
 ├─ Research JWT best practices and review the existing auth
 │  middleware in the codebase
 ├─ Implement JWT authentication with refresh tokens on the
 │  /api routes using the existing user model
 └ Done in 12.4s

```

## 7. Agent asks a follow-up question
```
*Expiry* [Storage] [Confirm]

What should the access token expiry be?

1. 15 minutes (Recommended)
2. 1 hour
3. (Place holder for a multiple line text input, for a free form answer)

← Left tab, → Right tab, ↑ Up, ↓ Down, ↵ Confirm, Esc Dismiss
```

## 8. Final answer after user confirms
```
🧑‍💻 User
 └ 15 minutes, Database

🤖 shelly
 └ Done! Here's what was added:

   **New files:**
   - `src/services/jwt.go` — Token generation, validation, and refresh rotation
   - `src/middleware/jwt.go` — Chi middleware for protected routes
   - `src/models/session.go` — Refresh token session model

   **Modified files:**
   - `src/routes/api.go` — Applied JWT middleware to /api group
   - `src/models/user.go` — Added Sessions relation

   Access tokens expire in 15 minutes, refresh tokens in 7 days. Refresh
   tokens are stored in a `user_sessions` table with rotation on each use.

   Run `go test ./src/...` to verify.

──────────────────────────────────────────────────────────────────────────────────
>
──────────────────────────────────────────────────────────────────────────────────
24.8k tokens
```

The task panel is gone because all tasks completed (see the General section: "The task list disappears once all tasks are done").
