# browser

Package browser provides tools that give agents browser automation via headless Chrome (chromedp).

## Permission Model

Each domain is gated by the shared permission store. Users can "trust" a domain to allow all future browser access without being prompted again. Trusted domains are shared with the `http` toolbox — trusting a domain in one unlocks it in the other.

## Chrome Lifecycle

The Chrome process is started lazily on the first tool call (not at engine boot). It runs in incognito mode to avoid leaking cookies, history, or user profiles. By default Chrome opens a visible window so users can manually handle CAPTCHAs or bot detection. Set `headless: true` in the browser config for headless mode.

## Tools

| Tool | Description | Permission |
|------|-------------|------------|
| `web_search` | Search via DuckDuckGo HTML, returns `[{title, url, snippet}]` | None |
| `web_navigate` | Navigate to URL, extract clean text (100KB cap) | Domain trust |
| `web_click` | Click element by CSS selector | Post-click domain check |
| `web_type` | Type into input field, optionally submit | None |
| `web_extract` | Extract text from current page or CSS selector | None |
| `web_screenshot` | PNG screenshot as base64 (viewport, full page, or element) | None |

## Usage

```go
b := browser.New(permStore, askFn, browser.WithHeadless())
tb := b.Tools() // *toolbox.ToolBox with browser tools
defer b.Close()
```

## Configuration

```yaml
browser:
  headless: true  # default: false (visible Chrome window)
```

Add `browser` to an agent's `toolboxes` list to enable browser tools for that agent.
