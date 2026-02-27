# http

Package `http` provides a tool that gives agents the ability to make HTTP requests.

## Permission Model

Each domain is gated by explicit user permission using domain trust. Users can "trust" a domain to allow all future requests to it without being prompted again. Trusted domains are persisted to the shared permissions file. Users are prompted with three options: **yes** (single request), **trust** (permanent), and **no** (deny).

## SSRF Protection

The package includes multiple layers of protection against Server-Side Request Forgery (SSRF):

1. **Pre-request check**: `isPrivateHost()` resolves the hostname and checks all IPs against private/loopback CIDR ranges (127.0.0.0/8, 10.0.0.0/8, 172.16.0.0/12, 192.168.0.0/16, 169.254.0.0/16, ::1/128, fc00::/7, fe80::/10).
2. **Connection-time check**: A custom `safeTransport` with a guarded `DialContext` performs DNS resolution and IP validation at connection time, preventing DNS rebinding attacks where a hostname resolves to a public IP during the permission check but to a private IP when the connection is made.
3. **Redirect validation**: The `CheckRedirect` handler rejects redirects to untrusted domains and to private/internal addresses.

## Exported API

### Types

- **`HTTP`** -- provides HTTP tools with permission gating. Manages an `http.Client` with a 60-second timeout and the safe transport.
- **`AskFunc`** -- `func(ctx context.Context, question string, options []string) (string, error)` callback for permission prompts.

### Functions

- **`New(store *permissions.Store, askFn AskFunc) *HTTP`** -- creates an HTTP that checks the given permissions store for trusted domains and prompts the user when a domain is not yet trusted.

### Methods on HTTP

- **`Tools() *toolbox.ToolBox`** -- returns a ToolBox containing the `http_fetch` tool.

## Tool

| Tool | Description |
|------|-------------|
| `http_fetch` | Make an HTTP request. Returns JSON with `status`, `headers`, and `body` (capped at 1MB). |

**Input schema:**

```json
{
  "url": "https://api.example.com/data",
  "method": "POST",
  "headers": {"Content-Type": "application/json"},
  "body": "{\"key\": \"value\"}"
}
```

- `method` defaults to `GET` if omitted.
- `headers` and `body` are optional.

## Usage

```go
h := http.New(permStore, askFn)
tb := h.Tools() // *toolbox.ToolBox with http_fetch tool
```

## Dependencies

- `pkg/codingtoolbox/permissions` -- shared permissions store (domain trust)
- `pkg/tools/toolbox` -- Tool and ToolBox types
