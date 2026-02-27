# ask

Package `ask` provides a tool that lets agents ask the user a question and block
until a response is received.

## Architecture

The central type is **`Responder`**, which:

1. Exposes an `ask_user` tool via `Tools()` that agents call through the normal
   tool-calling loop.
2. Blocks the tool handler until the frontend delivers a response via `Respond()`.
3. Notifies the frontend through an `OnAskFunc` callback when a new question is
   posed.
4. Provides a programmatic `Ask(ctx, text, options)` method that other packages
   (e.g. `filesystem`, `exec`) use to prompt the user without going through the
   JSON tool handler layer.

Questions can include multiple-choice options or be free-form. The user may
select one of the provided options or supply a custom text response.

## Exported API

### Types

- **`Question`** -- represents a question posed to the user. Fields: `ID`, `Text`, `Options`, `Header` (short tab label), `MultiSelect` (checkboxes vs single choice).
- **`OnAskFunc`** -- `func(ctx context.Context, q Question)` callback invoked when a new question is posed.
- **`Responder`** -- manages pending questions and their response channels. Thread-safe.

### Functions

- **`NewResponder(onAsk OnAskFunc) *Responder`** -- creates a Responder. The `onAsk` callback is invoked every time the agent asks a question, giving the frontend an opportunity to display it. If `onAsk` is nil, questions are still registered but no notification is sent.

### Methods on Responder

- **`Tools() *toolbox.ToolBox`** -- returns a ToolBox containing the `ask_user` tool.
- **`Ask(ctx context.Context, text string, options []string) (string, error)`** -- programmatically poses a question and blocks until a response or context cancellation. When both cancellation and a response arrive simultaneously, the response is preferred (drain-before-cancel semantics).
- **`Respond(questionID, response string) error`** -- delivers a user response to a pending question. Returns an error if the question ID is not found.

## Tool

| Name | Description |
|------|-------------|
| `ask_user` | Ask the user a question with optional multiple-choice options |

**Input schema:**

```json
{
  "question": "Pick a color",
  "options": ["red", "blue", "green"]
}
```

## Usage

```go
r := ask.NewResponder(func(ctx context.Context, q ask.Question) {
    // Notify the frontend about the question.
    fmt.Printf("Question %s: %s (options: %v)\n", q.ID, q.Text, q.Options)
})

// Register the toolbox with an agent.
agent.AddToolBoxes(r.Tools())

// When the frontend collects the user's answer:
r.Respond(questionID, "user's answer")

// Programmatic usage from another package:
resp, err := r.Ask(ctx, "Allow access?", []string{"yes", "no"})
```

## Dependencies

- `pkg/tools/toolbox` -- Tool and ToolBox types
