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

## Agent thinking message
A loading state shown while waiting for the LLM response. Replaced by a reasoning message once content starts streaming.
|                                                                                                                |
|                                                                                                                |
| 🤖 <agent name> is thinking...                                                                                 |
|                                                                                                                |
|                                                                                                                |

## Agent reasoning message
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

## Tool call message
### While running
The tool call message is displayed with the time and tokens used so far. Tool arguments are truncated to the terminal width.
|                                                                                                                |
|                                                                                                                |
| 🔧 <tool name>(with, arguments, separated, by, commas) 2s, 900 tokens                                          |
|                                                                                                                |
|                                                                                                                |

### After finished
The tool call message is displayed with the result of the tool call.
|                                                                                                                |
|                                                                                                                |
| 🔧 <tool name>(with, arguments, separated, by, commas)                                                         |
|  └ Read 200 lines from file /path/to/file.txt in 2s, 900 tokens                                                |
|                                                                                                                |
|                                                                                                                |

## Multiple tool calls
### While running
Concurrent tool calls are grouped under a "Using tools" parent and displayed as a tree. Sequential tool calls are displayed individually (flat), one after another.

|                                                                                                                |
|                                                                                                                |
| 🔧 Using tools                                                                                                 |
| ├─ <tool name>(with, arguments, separated, by, commas)                                                         |
| │  └ Read 200 lines from file /path/to/file.txt in 2s, 900 tokens                                              |
| ├─ <another tool name>(with, arguments, separated, by, commas) 0.1s, 150 tokens                                |
| └ 1050 tokens                                                                                                  |
|                                                                                                                |
|                                                                                                                |

### After finished
The tree of tool calls is collapsed and the total time and tokens are displayed.
|                                                                                                                |
|                                                                                                                |
| 🔧 Used tools                                                                                                  |
| └ Finished with 2 tools in 2.2s, 1250 tokens                                                                   |
|                                                                                                                |
|                                                                                                                |

## Error states
When a tool call fails or a network error occurs, the raw error response is displayed as a string in the tool result.
|                                                                                                                |
|                                                                                                                |
| 🔧 <tool name>(with, arguments, separated, by, commas)                                                         |
|  └ Error: connection refused: could not reach api.example.com in 2s, 900 tokens                                 |
|                                                                                                                |
|                                                                                                                |

# Sub agents
Sub agents are displayed as a list of agent containers, each container displays agent reasoning messages and tool calls.
Each container is a scrollable box limited to 4 visible lines of text. Content auto-scrolls to the bottom as new output arrives.

## Delegation to sub agent
The delegate tool call displays the agent name as a title on the first line. The full task description is shown below, word-wrapped to the terminal width, using tree connectors for indentation.

### While delegating
|                                                                                                                |
|                                                                                                                |
| 🔧 Delegating to <agent name> 2s ⣾                                                                            |
|  ├─ Lorem ipsum dolor sit amet, consectetur adipiscing elit. Pellentesque rutrum convallis risus. Pellentesque |
|  │  sit amet ipsum erat. Nulla aliquam elit feugiat, ornare est sed, semper augue. Suspendisse et              |
|  │  neque rhoncus, bibendum lacus eleifend, scelerisque augue.                                                 |
|                                                                                                                |
|                                                                                                                |

### After delegation finished
|                                                                                                                |
|                                                                                                                |
| 🔧 Delegating to <agent name>                                                                                  |
|  ├─ Lorem ipsum dolor sit amet, consectetur adipiscing elit. Pellentesque rutrum convallis risus. Pellentesque |
|  │  sit amet ipsum erat. Nulla aliquam elit feugiat, ornare est sed, semper augue. Suspendisse et              |
|  │  neque rhoncus, bibendum lacus eleifend, scelerisque augue.                                                 |
|  └ Finished in 5.3s                                                                                           |
|                                                                                                                |
|                                                                                                                |

## While sub agent is running
|                                                                                                                |
|                                                                                                                |
| 🤖 <sub agent 1 name>                                                                                           |
|   🔧 Used tools                                                                                                |
|    └ Finished with 2 tools in 2.2s, 1250 tokens                                                                |
|                                                                                                                |
|   🔧 Using tools                                                                                               |
|    ├─ <tool name>(with, arguments, separated, by, commas)                                                      |
|    │  └ Read 200 lines from file /path/to/file.txt in 2s, 900 tokens                                           |
|    ├─ <another tool name>(with, arguments, separated, by, commas) 0.1s, 150 tokens                             |
|    └ 1050 tokens                                                                                               |
|                                                                                                                |
|    🤖 <sub agent 1 name> (reasoning)                                                                           |
|    └ Lorem ipsum dolor sit amet, consectetur adipiscing elit. Pellentesque rutrum convallis risus. Pellentesque|
|    sit amet ipsum erat. Nulla aliquam elit feugiat, ornare est sed, semper augue. Suspendisse et               |
|    neque rhoncus, bibendum lacus eleifend, scelerisque augue. Nam ut est dolor. Mauris ullamcorper, neque      |
|    seddignissim ornare, neque mauris auctor ex, ac lobortis elit sem eget purus. Sed eleifend mattis sem in    |
|    fringilla. Fusce condimentum risus ut maximus feugiat.                                                      |
|                                                                                                                |
|                                                                                                                |
| 🤖 <sub agent 2 name>                                                                                          |
|   🔧 Used tools                                                                                                |
|    └ Finished with 2 tools in 2.2s, 1250 tokens                                                                |
|                                                                                                                |
|   🔧 Using tools                                                                                               |
|    ├─ <tool name>(with, arguments, separated, by, commas)                                                      |
|    │  └ Read 200 lines from file /path/to/file.txt in 2s, 900 tokens                                           |
|    ├─ <another tool name>(with, arguments, separated, by, commas) 0.1s, 150 tokens                             |
|    └ 1050 tokens                                                                                               |
|                                                                                                                |
|    🤖 <sub agent 2 name> (reasoning)                                                                            |
|    └ Lorem ipsum dolor sit amet, consectetur adipiscing elit. Pellentesque rutrum convallis risus. Pellentesque|
|    sit amet ipsum erat. Nulla aliquam elit feugiat, ornare est sed, semper augue. Suspendisse et               |
|    neque rhoncus, bibendum lacus eleifend, scelerisque augue. Nam ut est dolor. Mauris ullamcorper, neque      |
|    seddignissim ornare, neque mauris auctor ex, ac lobortis elit sem eget purus. Sed eleifend mattis sem in    |
|    fringilla. Fusce condimentum risus ut maximus feugiat.                                                      |
|                                                                                                                |
|                                                                                                                |

## After sub agent finished
When a sub agent finishes, its container collapses to show the total time and tokens.
|                                                                                                                |
|                                                                                                                |
| 🤖 <sub agent 1 name>                                                                                          |
| └ Finished in 5.3s, 3200 tokens                                                                                |
|                                                                                                                |
| 🤖 <sub agent 2 name>                                                                                          |
| └ Finished in 4.1s, 2800 tokens                                                                                |
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
