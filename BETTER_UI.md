We should improve how messages are displayed for the user to be able to take better decisions.

User messages should display in the following way:

| 🧑‍💻 User > The actual message the user sent to the agent, and when is multi line, all lines should stay grouped |
| together without any space in between the lines.                                                               |
|                                                                                                                |
|                                                                                                                |
| 🤖 Agent name > Thinking messages from the agent in light gray, and when is multi line, all lines should stay  |
| grouped together without any space in between the lines.                                                       |
| ─ Thought for 2s, used 123 tokens                                                                              |
|                                                                                                                |
|                                                                                                                |
| 🤖 <agent name> is thinking...                                                                                 |
|                                                                                                                |
|                                                                                                                |
| 🔧 Tool name (with this arguments)                                                                              |
| └ The result of the tool call.                                                                                 |
|                                                                                                                |
|                                                                                                                |
| 🔧 Multiple Tool calls to the same tool                                                                        |
| │  Tool call 1 (with this arguments)                                                                           |
| │  Tool call 2 (with this arguments)                                                                           |
| │  Tool call N (with this arguments)                                                                           |
| └ Summary of the tool calls. Runned 2s for 2s                                                                  |
|                                                                                                                |
|                                                                                                                |
| 📝 <Agent name> plan:                                                                                          |
| The actual plan from the agent in formatted markdown.                                                          |
|                                                                                                                |
|                                                                                                                |
| 🙋 <Agent name> interview to the user:                                                                         |
|                                                                                                                |
| `A short tab title` | The second tab title | The answer confirmation tab                                       |
|                                                                                                                |
| **The agent question wirtten in bold**                                                                         |
| 1. The first answer option                                                                                     |
| 2. The second answer option                                                                                    |
| (... N. The Nth answer option)                                                                                 |
|                                                                                                                |
| -------------------------------------------------------------------------------------------------------------- |
|                                                                                                                |
| Custom answer: The user's answer to the agent's question.                                                      |
|                                                                                                                |
|                                                                                                                |
| 🦾 <sub agent name> a message of what the sub agent is working on.                                             |
| └ Summary of the sub agent's work.                                                                             |
|                                                                                                                |
| ╭────────────────────────────────────────────────────────────────────────────────────────────────────────────╮ |
| │ > this is the user input and will allways be displayed at the bottom of the screen.                        | |
| ╰────────────────────────────────────────────────────────────────────────────────────────────────────────────╯ |
| 20,123 total, 989 tokens last message, 12.3s total time                                                        |

* Agen thinking messages should be displayed in light gray.
* Tool calls should be displayed in bold.
* Tool results should be displayed in light gray.
* Multiple tool calls to the same tool should be displayed in the same way but they should display at most 4 messages at a time with new messages replacing the oldest ones.
* Sub agents should display thinking, tools calls and results in the same way but they should display at most 4 messages at a time with new messages replacing the oldest ones.

Behavior descriptions:

* User messages:
  * Are simple text messages with the prefix "🧑‍💻 User >" on blue color and the message content displayed in black.
* Agent thinking messages:
  * Are simple text messages with the prefix "🤖 <agent name> >" on light gray color and the message content displayed in black.
* Tool calls:
  * Are simple text messages with the prefix "🔧 <tool name> (with this arguments)" on bold color and the message content displayed in black.
* Tool results:
  * Are simple text messages with the prefix "└ <tool result>" on light gray color and the message content displayed in black.
* Multiple tool calls to the same tool:
  * Are simple text messages with the prefix "🔧 Multiple Tool calls to the same tool" on bold color and the message content displayed in black.
  * The tool calls should be displayed in the same way but they should display at most 4 messages at a time with new messages replacing the oldest ones.
* Sub agents:
  * Should display thinking, tools calls and results in the same way but they should display at most 4 messages at a time with new messages replacing the oldest ones.

Rational:
You could at this as comparmentalized structures each representing a different agent and messages as a component that displayes within.

Example:
--------------------------------
| Agent chain of thought       |
| Tool call                    |
| Thinking message             |
| Agent reasoning message      |
| Agent plan                   |
| Agent interview to the user  |
--------------------------------

This box represents an agent own chat with all of the available messages.

Messages need to share a base data type that standardizes how the are displayed, and special cases for each message type implement their own behavior and rendering logic.