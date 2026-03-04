# basetextarea

Shared auto-growing textarea component for the TUI.

Wraps `charm.land/bubbles/v2/textarea` with:

- **Shared defaults**: no prompt, no line numbers, no char limit, cleared styles
- **Auto-grow**: automatically adjusts height based on visual line count (hard newlines + soft wraps)
- **Configurable**: placeholder text, min/max height, width

## Usage

```go
ta := basetextarea.New("Type here...", 1, 5)
```

The `Update()` method handles the pre-set-max-height, update, then shrink-to-content pattern automatically. Callers that need to bypass auto-grow (e.g., for custom key handling before forwarding) can access the underlying `TA` field directly.
