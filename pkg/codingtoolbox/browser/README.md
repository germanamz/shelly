# browser

Package `browser` provides tools that give agents browser automation via headless Chrome using the [chromedp](https://github.com/chromedp/chromedp) library.

## Permission Model

Each domain is gated by the shared permission store using domain trust. Users can "trust" a domain to allow all future browser access without being prompted again. Trusted domains are shared with the `http` toolbox -- trusting a domain in one unlocks it in the other.

DuckDuckGo search (`browser_search`) does not require domain trust since it is internal search infrastructure. Navigation via `browser_navigate` and post-navigation domain changes (via `browser_click` or `browser_type` with submit) do require trust.

## Chrome Lifecycle

The Chrome process is started lazily on the first tool call (not at engine boot) via `ensureBrowser()`. It runs in incognito mode with GPU disabled for stability. By default Chrome opens a visible window so users can manually handle CAPTCHAs or bot detection. Use the `WithHeadless()` option for headless mode. If the Chrome context is cancelled, the next tool call will restart Chrome automatically.

Call `Close()` to shut down the Chrome process when done.

## Exported API

### Types

- **`Browser`** -- provides browser automation tools with permission gating. Manages the Chrome lifecycle and domain trust checks.
- **`AskFunc`** -- `func(ctx context.Context, question string, options []string) (string, error)` callback for permission prompts.
- **`Option`** -- functional option for configuring Browser behaviour.

### Functions

- **`New(store *permissions.Store, askFn AskFunc, opts ...Option) *Browser`** -- creates a Browser backed by the given permissions store.
- **`WithHeadless() Option`** -- enables headless Chrome mode (no visible window).

### Methods on Browser

- **`Tools() *toolbox.ToolBox`** -- returns a ToolBox containing all 6 browser tools.
- **`Close()`** -- shuts down the Chrome process if it was started. Safe to call when Chrome was never started.

## Tools

| Tool | Description | Permission |
|------|-------------|------------|
| `browser_search` | Search the web via DuckDuckGo HTML, returns up to 30 results as `[{title, url, snippet}]` | None |
| `browser_navigate` | Navigate to URL, wait for body ready, extract clean text (100KB cap). Returns `{url, title, text}` | Domain trust |
| `browser_click` | Click element by CSS selector. Returns `{url, title}`. Checks domain trust after navigation; navigates back on denial | Post-click domain check |
| `browser_type` | Type into input field by CSS selector, optionally submit (press Enter). Returns `{url, title}`. Checks domain trust if submit causes navigation | Post-submit domain check |
| `browser_extract` | Extract clean text from current page or specific CSS selector. Scripts, styles, noscript, and SVG elements are stripped. Returns `{url, title, text}` (100KB cap) | None |
| `browser_screenshot` | PNG screenshot as base64 -- viewport (default), full page (`full_page: true`), or element (`selector`). Returns `{url, title, base64}` | None |

All operations have a 30-second timeout per tool call.

## Text Extraction

The `extractText` helper (used by `browser_navigate` and `browser_extract`) runs client-side JavaScript that:

1. Clones the target element (or `document.body`)
2. Removes `script`, `style`, `noscript`, and `svg` elements
3. Extracts `innerText`
4. Collapses multiple blank lines to a single newline
5. Truncates to 100KB with a `[content truncated]` marker

## Usage

```go
b := browser.New(permStore, askFn, browser.WithHeadless())
tb := b.Tools() // *toolbox.ToolBox with 6 browser tools
defer b.Close()
```

## Configuration

```yaml
browser:
  headless: true  # default: false (visible Chrome window)
```

Add `browser` to an agent's `toolboxes` list to enable browser tools for that agent.

## Dependencies

- `pkg/codingtoolbox/permissions` -- shared permissions store (domain trust)
- `pkg/tools/toolbox` -- Tool and ToolBox types
- `github.com/chromedp/chromedp` -- Chrome DevTools Protocol driver
