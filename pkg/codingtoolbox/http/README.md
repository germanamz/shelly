# http

Package http provides a tool that gives agents the ability to make HTTP requests.

## Permission Model

Each domain is gated by explicit user permission using domain trust. Users can "trust" a domain to allow all future requests to it without being prompted again. Trusted domains are persisted to the shared permissions file.

## Tools

| Tool | Description |
|------|-------------|
| `http_fetch` | Make an HTTP request. Returns status, headers, and body (capped at 1MB) |

## Usage

```go
h := http.New(permStore, askFn)
tb := h.Tools() // *toolbox.ToolBox with http tools
```

The `AskFunc` callback is called whenever a request is made to an untrusted domain. The user can approve once or trust the domain permanently.
