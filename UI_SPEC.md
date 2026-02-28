# General
All multi-line text wraps using pre-word wrapping at the terminal width.
The UI resizes dynamically with the terminal. Minimum supported terminal width is 80 columns.
Color theme follows the GitHub terminal light theme.

# User Input
The user input is displayed at the bottom of the screen. Below the user input there is the total sum of used tokens of the session. Large token counts are formatted (e.g., 1.2k, 15k).
Enter submits the message. Multi-line input is supported by pressing Shift+Enter or Alt+Enter to insert a newline.
The token counter is hidden when a picker (file or command) is open.
The input remains enabled while the agent is working. Messages sent during agent processing are queued and delivered when the agent is ready. Press Escape to interrupt the agent.
Escape is consumed by the innermost active context first: an open picker is dismissed before the agent is interrupted.

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
When the user types / the command picker is displayed. The picker works the same as the file picker: the list filters as the user types, the user can select a command using the up and down arrow keys, and confirm the command using the enter key. Press Escape to dismiss the picker. The currently selected item is displayed in **bold and underlined**. The picker shows a maximum of 4 visible items; the rest are accessible by scrolling with the up and down arrow keys.

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

### After command selected
|                                                                                                                |
|                                                                                                                |
| -------------------------------------------------------------------------------------------------------------- |
| > /command command arg                                                                                         |
| -------------------------------------------------------------------------------------------------------------- |
| 19000 tokens                                                                                                   |
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

When the agent finishes, the container collapses to a summary with the agent name and elapsed time in **dim**:
|                                                                                                                |
|                                                                                                                |
| 🤖 <agent name>                                                                                                |
| └ Finished in 5.3s                                                                                             |
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
Sub-agent containers are nested inside their parent's container. While active, the header shows the agent name in **magenta** (#8250df) with a braille spinner. All items inside the container are indented with `  │ ` (2 spaces + tree pipe). The container shows a maximum of 4 visible items; older items are hidden behind a "... N more items" indicator in **dim**. Items inside the container use the same rendering as top-level items (reasoning, tool calls, groups) but at a reduced width.

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
|                                                                                                                |
|                                                                                                                |
| 🤖 <sub agent name>                                                                                            |
| └ Finished in 5.3s                                                                                             |
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
When a sub agent finishes, its container collapses to show only the header and a summary line with elapsed time in **dim**.
|                                                                                                                |
|                                                                                                                |
| 🤖 <sub agent 1 name>                                                                                          |
| └ Finished in 5.3s                                                                                             |
|                                                                                                                |
| 🤖 <sub agent 2 name>                                                                                          |
| └ Finished in 4.1s                                                                                             |
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
