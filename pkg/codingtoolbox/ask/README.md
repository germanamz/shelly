# ask

Package `ask` provides a tool that lets agents ask the user a question and block
until a response is received.

## Architecture

The central type is **Responder**, which:

1. Exposes an `ask_user` tool via `Tools()` that agents call through the normal
   tool-calling loop.
2. Blocks the tool handler until the frontend delivers a response via `Respond()`.
3. Notifies the frontend through an `OnAskFunc` callback when a new question is
   posed.

Questions can include multiple-choice options or be free-form. The user may
select one of the provided options or supply a custom text response.

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
```

## Dependencies

- `pkg/tools/toolbox` — Tool and ToolBox types
