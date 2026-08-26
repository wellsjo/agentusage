# agentusage

`agentusage` reads local Codex and Claude Code account limits. It provides a
normalized Go API, an HTTP handler, and a framework-agnostic Web Component.

The module has no third-party dependencies. Credentials stay inside the Go
package and never enter the JSON response or browser component.

## Go

```go
usage := agentusage.New()

// Use the snapshot in application code.
snapshot := usage.Snapshot(ctx)

// Or expose the normalized JSON contract.
mux.Handle("GET /ai/usage", agentusage.Handler(usage))

// Serve the optional browser component from the same binary.
mux.Handle("GET /assets/agent-usage.js", agentusageweb.Handler())
```

## Browser

The component is a standards-based custom element. It does not require React,
a bundler, or an npm package.

```html
<script type="module" src="/assets/agent-usage.js"></script>

<agent-usage endpoint="/ai/usage" refresh-ms="120000"></agent-usage>
```

Applications can also supply data without an HTTP request:

```js
document.querySelector("agent-usage").snapshot = snapshot;
```

The Shadow DOM protects the component from application CSS. CSS custom
properties provide theme control:

```css
agent-usage {
  --agentusage-columns: 2;
  --agentusage-fill: #1f883d;
  --agentusage-track: #d0d7de;
  --agentusage-text: #1f2328;
  --agentusage-muted: #59636e;
  --agentusage-danger: #cf222e;
}
```

## Providers

- Codex uses `codex app-server`. The first-party CLI owns its authentication
  and token refresh.
- Claude Code reads its credential file or macOS Keychain item. It refreshes
  OAuth tokens before expiry, retries HTTP 401 once, and persists rotations.

## JSON contract

```json
{
  "providers": [
    {
      "id": "codex",
      "name": "Codex",
      "windows": [
        {
          "id": "primary",
          "label": "5h window",
          "used_percent": 12,
          "window_seconds": 18000,
          "resets_at": "2026-08-26T18:00:00Z"
        }
      ],
      "fetched_at": "2026-08-26T17:00:00Z"
    }
  ],
  "updated_at": "2026-08-26T17:00:00Z"
}
```

Failed refreshes retain the last good provider values with `stale: true` and a
sanitized `error` string.
