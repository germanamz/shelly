# User Input
The user input is displayed at the bottom of the screen. Below the user input there is the totak sum of used tokens of the session.

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
When the user types @ the file picker is displayed, the user can select a file from the file system and the file path is inserted into the user input.
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
When the user types / the command picker is displayed, the user can select a command from the command list and the command is executed.
|                                                                                                                |
|                                                                                                                |
| -------------------------------------------------------------------------------------------------------------- |
| > /command command arg                                                                                         |
| -------------------------------------------------------------------------------------------------------------- |
| 19000 tokens                                                                                                   |
|                                                                                                                |
|                                                                                                                |

## Command picker
The command picker is displayed as a list of commands, the user can select a command using the up and down arrow keys, and confirm the command using the enter key.
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
Messages can be displayes as markdown.
There must be at least 1 line of padding between messages.


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
The tool call message is displayed with the time and tokens used so far.
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
Multiple tool calls are displayed as a tree of tool calls, the root node is the tool call message and the child nodes are the tool calls of the tool call message.
Only concurrent tool executions are displayed as a tree.

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

## Sub agents
Sub agents are displayed as a list of agent containers, each container displayes agent reasoning messages and tool calls.
To be able to display the sub agents each container should limit the number of lines it displays to 4.

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
|    🤖 <sub agent 1 name>                                                                                        |
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
|    🤖 <sub agent 2 name>                                                                                        |
|    └ Lorem ipsum dolor sit amet, consectetur adipiscing elit. Pellentesque rutrum convallis risus. Pellentesque|
|    sit amet ipsum erat. Nulla aliquam elit feugiat, ornare est sed, semper augue. Suspendisse et               |
|    neque rhoncus, bibendum lacus eleifend, scelerisque augue. Nam ut est dolor. Mauris ullamcorper, neque      |
|    seddignissim ornare, neque mauris auctor ex, ac lobortis elit sem eget purus. Sed eleifend mattis sem in    |
|    fringilla. Fusce condimentum risus ut maximus feugiat.                                                      |
|                                                                                                                |
|                                                                                                                |

# Questions

The user is expected to answer questions in a sequential batch, each question and answers are displayed in tabs, each tab title is a single word related to the question and the content of the tab is the question and the answers options.
The user can navigate through the tabs using the left and right arrow keys, and select an answer using the up and down arrow keys, then the user can confirm the answer using the enter key.
When the user confirms an answer, the ui moves to the next question tab, until all questions are answered.
When the user confirms the last question, the ui moves to the Confirm tab. On this tab the user can review the answers and confirm them to finish the interview.
When the user enters a custom answer in the Confirm tab is considered as acceptance of the answers and the interview is finished with all selected answers and the custom answer.

## Single Choice Answer
|                                                                                                                |
|                                                                                                                |
| *One* [Word] [Per] [Question] [Confirm]                                                                        |
|                                                                                                                |
|                                                                                                                |
| What is your anwser for question One?                                                                          |
|                                                                                                                |
| 1. The first answer option                                                                                     |
| 2. The second answer option                                                                                    |
| 3. (Place holder for a multiple line text input, for a free form answer)                                       |
|                                                                                                                |
| ← Left tab, → Right tab, ↑ Up, ↓ Down, ↵ Confirm Answer                                                        |
|                                                                                                                |

## Multiple Choice Answers
|                                                                                                                |
|                                                                                                                |
| [One] *Word* [Per] [Question] [Confirm]                                                                        |
|                                                                                                                |
|                                                                                                                |
| What are your anwsers for question Word? ()                                                                    |
|                                                                                                                |
| 1. [X] The first answer option                                                                                 |
| 2. [ ] The second answer option                                                                                |
| 3. [X] The third answer option                                                                                 |
|                                                                                                                |
| ← Left tab, → Right tab, ↑ Up, ↓ Down, ↵ Confirm Answer                                                        |
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
| ← Left tab, → Right tab, ↑ Up, ↓ Down, ↵ Confirm Answer                                                        |
|                                                                                                                | 
