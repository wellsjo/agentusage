# agentusage

`agentusage` reads local Codex and Claude Code account limits. It gives a
normalized Go API, an HTTP handler, and a framework-agnostic Web Component.

The module has no third-party dependencies. Credentials stay inside the Go
package. They never enter the JSON response or the browser component.

## Go

```go
usage := agentusage.New()

// Use the snapshot in application code.
snapshot := usage.Snapshot(ctx)

// Find normalized providers and windows by their stable IDs.
codex, ok := snapshot.Provider(agentusage.ProviderIDCodex)
if ok {
    primary, ok := codex.Window(agentusage.CodexWindowIDPrimary)
    if ok {
        fmt.Printf("Codex primary window: %.1f%% used\n", primary.UsedPercent)
    }
}

// Or expose the normalized JSON contract.
mux.Handle("/ai/usage", agentusage.Handler(usage))

// Serve the optional browser component from the same binary.
mux.Handle("/assets/agent-usage.js", agentusageweb.Handler())
```

The fetcher caches a snapshot for two minutes. Concurrent calls share one
refresh, and the refresh runs on a detached context with its own timeout. So
a canceled request cannot block or poison other callers. A snapshot with no
data at all expires after 15 seconds, so the module recovers quickly from a
start-up failure. The module honors `Retry-After` on HTTP 429, with a cap of
24 hours.

## Browser

The component is a standards-based custom element. It does not need React, a
bundler, or an npm package.

```html
<script type="module" src="/assets/agent-usage.js"></script>

<agent-usage endpoint="/ai/usage" refresh-ms="120000"></agent-usage>
```

Attributes:

- `endpoint` — the JSON endpoint. The default is `/ai/usage`. An empty value
  turns the fetch off.
- `refresh-ms` — the poll interval in milliseconds. The default is `120000`,
  and the minimum is `10000`. A value of `0` turns the poll off. An invalid
  value falls back to the default.

The component pauses its poll while the tab is hidden. It polls once when the
tab becomes visible again.

Applications can also supply data without an HTTP request. This stops the
automatic poll, so a fetch cannot overwrite the given data:

```js
document.querySelector("agent-usage").snapshot = snapshot;
```

The Shadow DOM protects the component from application CSS. CSS custom
properties give theme control:

```css
agent-usage {
  --agentusage-columns: 2;
  --agentusage-fill: #1f883d;
  --agentusage-track: #d0d7de;
  --agentusage-text: #1f2328;
  --agentusage-muted: #59636e;
  --agentusage-danger: #cf222e;
  --agentusage-icon-codex: var(--agentusage-muted);
  --agentusage-icon-claude: #d97757;
}
```

## Providers

- Codex uses `codex app-server`. The first-party CLI owns its authentication
  and its token refresh.
- Claude Code reads its credential file or its macOS Keychain item. It
  refreshes OAuth tokens before expiry, retries HTTP 401 once, and persists
  rotations. When a save fails, the module keeps the rotated credential in
  memory and tries the save again on the next poll.

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

`windows` is always an array. `fetched_at` appears only after one successful
fetch for that provider. A failed refresh keeps the last good provider values
with `stale: true` and a sanitized `error` string.
