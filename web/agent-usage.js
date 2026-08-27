const ICON_PATHS = {
  codex: "M83.773 42.809a20.28 20.28 0 0 0-23.418-26.036A20.3 20.3 0 0 0 26.102 24.01a20.28 20.28 0 0 0-10.85 33.323 20.28 20.28 0 0 0 23.419 26.036A20.3 20.3 0 0 0 72.94 76.052a20.28 20.28 0 0 0 10.833-33.243ZM53.7 84.836a14.93 14.93 0 0 1-9.588-3.47l16.4-9.462a2.63 2.63 0 0 0 1.31-2.271V47.177l6.733 3.895a.24.24 0 0 1 .126.174v18.608A15.02 15.02 0 0 1 53.7 84.836ZM21.498 71.084a14.93 14.93 0 0 1-1.782-10.045l16.416 9.477a2.6 2.6 0 0 0 2.602 0L58.21 59.288v7.775a.24.24 0 0 1-.11.205l-16.133 9.304a15.02 15.02 0 0 1-20.47-5.488ZM17.303 36.39a15.02 15.02 0 0 1 7.885-6.576v18.924a2.6 2.6 0 0 0 1.293 2.255l19.381 11.181-6.734 3.895a.24.24 0 0 1-.236 0l-16.101-9.288a15.02 15.02 0 0 1-5.488-20.47v.079Zm55.321 12.853L53.18 37.951l6.718-3.88a.24.24 0 0 1 .237 0l16.1 9.305a15.02 15.02 0 0 1-2.255 27.014V51.466a2.6 2.6 0 0 0-1.356-2.223Zm6.702-10.077-16.385-9.557a2.6 2.6 0 0 0-2.618 0L40.863 40.837v-7.774a.24.24 0 0 1 .095-.205l16.1-9.289a15.02 15.02 0 0 1 22.268 15.596ZM37.189 52.948l-6.734-3.879a.24.24 0 0 1-.126-.19V30.319a15.02 15.02 0 0 1 24.112-11.244l-15.928 9.194a2.63 2.63 0 0 0-1.309 2.27l-.015 22.41Zm3.658-7.885 8.674-4.999 8.689 5v9.997l-8.658 5-8.689-5-.016-9.998Z",
  claude: "m25.715 63.215 15.724-8.823.264-.77-.264-.424h-.768l-28.86-1.539-1.78-2.347.181-1.174 1.6-1.072 29.595 2.246h.296l.182-.526-.79-.648-24.427-17.364-.526-3.36 2.186-2.408 2.934.203 19.59 14.53.486-.344.06-.243-12.587-22.868-.344-2.429 2.49-3.38 1.375-.445 3.32.446 1.396 1.214 12.912 28.01h.83v-.486l2.247-23.698 1.254-3.036 2.49-1.64 1.942.931 1.599 2.287-4.25 23.8h.708l15.28-19.226h3.44l2.53 3.764-1.133 3.886-13.317 18.497.243.364.627-.06 23.557-4.007 2.773 1.296.303 1.315-1.093 2.692-25.844 5.97-.142.1.162.203 22.828 1.356 2.631 1.74 1.579 2.126-.263 1.618-4.048 2.065-23.193-5.424h-.607v.364L81.146 74.124l.425 1.922-1.073 1.518-1.133-.162-16.595-13.418h-.425v.567l9.288 13.903.405 3.603-.566 1.174-2.023.708-2.227-.405-13.094-20.116-.465.263-2.247 24.184-1.052 1.235-2.428.93-2.024-1.538-1.073-2.489 4.938-24.872-.04-.142-.466.061-17.79 22.525-1.375.546-2.388-1.235.222-2.206 17.182-21.978-.02-.526h-.182L23.792 71.898l-3.765.485-1.619-1.517.203-2.49 7.104-5.16Z",
};

const STYLES = `
  :host {
    --agentusage-columns: 2;
    --agentusage-fill: #1f883d;
    --agentusage-track: #d8dee4;
    --agentusage-text: #1f2328;
    --agentusage-muted: #59636e;
    --agentusage-danger: #cf222e;
    --agentusage-icon-codex: var(--agentusage-muted);
    --agentusage-icon-claude: #d97757;
    container-type: inline-size;
    display: block;
    color: var(--agentusage-text);
    font: 13px/1.4 -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif;
  }
  .providers {
    display: grid;
    grid-template-columns: repeat(var(--agentusage-columns), minmax(0, 1fr));
    gap: 30px;
  }
  .provider { min-width: 0; }
  .provider h3 {
    display: flex;
    align-items: center;
    gap: 7px;
    margin: 0 0 10px;
    color: var(--agentusage-muted);
    font-size: 11px;
    font-weight: 600;
    letter-spacing: .14em;
    line-height: 16px;
    text-transform: uppercase;
  }
  .icon { width: 14px; height: 14px; flex: 0 0 14px; fill: currentColor; }
  .icon--codex { color: var(--agentusage-icon-codex); }
  .icon--claude { color: var(--agentusage-icon-claude); }
  .window + .window { margin-top: 12px; }
  .meta {
    display: grid;
    grid-template-columns: minmax(0, 1fr) auto;
    min-width: 0;
    margin-bottom: 5px;
    color: var(--agentusage-muted);
    font-size: 12px;
    line-height: 16px;
  }
  .description {
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .used { margin-left: 8px; color: var(--agentusage-text); white-space: nowrap; }
  .track {
    position: relative;
    height: 6px;
    overflow: visible;
    border-radius: 6px;
    background: var(--agentusage-track);
  }
  .fill {
    display: block;
    height: 100%;
    border-radius: inherit;
    background: var(--agentusage-fill);
  }
  .marker {
    position: absolute;
    top: -2px;
    width: 1px;
    height: 10px;
    background: var(--agentusage-text);
    transform: translateX(-1px);
  }
  .status, .empty { color: var(--agentusage-muted); font-size: 12px; }
  .status { margin-top: 8px; }
  .providers > .status { grid-column: 1 / -1; }
  .status.error { color: var(--agentusage-danger); }
  @container (max-width: 540px) {
    .providers { grid-template-columns: 1fr; gap: 22px; }
  }
  @media (prefers-color-scheme: dark) {
    :host {
      --agentusage-fill: #3fb950;
      --agentusage-track: #30363d;
      --agentusage-text: #f0f6fc;
      --agentusage-muted: #8b949e;
      --agentusage-danger: #f85149;
    }
  }
`;

export class AgentUsage extends HTMLElement {
  static observedAttributes = ["endpoint", "refresh-ms"];

  constructor() {
    super();
    this._snapshot = null;
    this._error = "";
    this._controller = null;
    this._refreshTimer = null;
    this._clockTimer = null;
    this._connected = false;
    this._manual = false;
    this._restartQueued = false;
    this._refreshWhenVisible = false;
    this._onVisibilityChange = () => this._handleVisibilityChange();
    const shadow = this.attachShadow({ mode: "open" });
    const style = document.createElement("style");
    style.textContent = STYLES;
    this._root = document.createElement("div");
    this._root.className = "providers";
    this._root.setAttribute("part", "providers");
    shadow.append(style, this._root);
  }

  connectedCallback() {
    this._connected = true;
    document.addEventListener("visibilitychange", this._onVisibilityChange);
    this._render();
    this._startTimers();
    if (this.endpoint && !this._manual) this.refresh();
  }

  disconnectedCallback() {
    this._connected = false;
    document.removeEventListener("visibilitychange", this._onVisibilityChange);
    this._stopTimers();
    this._controller?.abort();
    this._controller = null;
  }

  // A guard on the internal connected flag skips the callbacks that run
  // during a custom-element upgrade, before connectedCallback. The microtask
  // queue merges several attribute changes into one restart and one fetch.
  attributeChangedCallback() {
    this._manual = false;
    this._scheduleRestart();
  }

  get endpoint() {
    return this.hasAttribute("endpoint") ? this.getAttribute("endpoint") : "/ai/usage";
  }

  set endpoint(value) {
    if (value === null || value === undefined) this.removeAttribute("endpoint");
    else this.setAttribute("endpoint", String(value));
  }

  // refreshMs returns the poll interval. An absent, empty, or invalid
  // attribute falls back to the default; 0 turns the poll off.
  get refreshMs() {
    const raw = this.getAttribute("refresh-ms");
    if (raw === null || raw.trim() === "") return 120000;
    const value = Number(raw);
    return Number.isFinite(value) && value >= 0 ? value : 120000;
  }

  set refreshMs(value) {
    this.setAttribute("refresh-ms", String(value));
  }

  get snapshot() {
    return this._snapshot;
  }

  // The snapshot setter puts the element in manual data mode: it stops the
  // automatic poll, so a later fetch cannot overwrite the given data. A
  // change to an observed attribute starts the poll again.
  set snapshot(value) {
    this._manual = true;
    this._controller?.abort();
    this._controller = null;
    this._stopRefreshTimer();
    this._snapshot = value && typeof value === "object" ? value : null;
    this._error = "";
    this._render();
  }

  async refresh() {
    const endpoint = this.endpoint;
    if (!endpoint) return;
    this._controller?.abort();
    const controller = new AbortController();
    this._controller = controller;
    try {
      const response = await fetch(endpoint, {
        headers: { Accept: "application/json" },
        signal: controller.signal,
      });
      if (!response.ok) throw new Error(`${response.status} ${response.statusText}`);
      this._snapshot = await response.json();
      this._error = "";
      this._render();
      this.dispatchEvent(new CustomEvent("agentusage-load", {
        bubbles: true,
        composed: true,
        detail: { snapshot: this._snapshot },
      }));
    } catch (error) {
      if (error?.name === "AbortError") return;
      this._error = error instanceof Error ? error.message : String(error);
      this._render();
      this.dispatchEvent(new CustomEvent("agentusage-error", {
        bubbles: true,
        composed: true,
        detail: { error: this._error },
      }));
    } finally {
      if (this._controller === controller) this._controller = null;
    }
  }

  _startTimers() {
    this._stopTimers();
    const refreshMs = this.refreshMs;
    if (refreshMs > 0 && this.endpoint && !this._manual) {
      this._refreshTimer = setInterval(() => this._timedRefresh(), Math.max(10000, refreshMs));
    }
    this._clockTimer = setInterval(() => {
      if (!document.hidden) this._render();
    }, 15000);
  }

  _stopRefreshTimer() {
    clearInterval(this._refreshTimer);
    this._refreshTimer = null;
  }

  _stopTimers() {
    this._stopRefreshTimer();
    clearInterval(this._clockTimer);
    this._clockTimer = null;
  }

  // A hidden tab defers the poll. The visibilitychange handler runs the
  // deferred poll when the tab becomes visible again.
  _timedRefresh() {
    if (document.hidden) {
      this._refreshWhenVisible = true;
      return;
    }
    this.refresh();
  }

  _handleVisibilityChange() {
    if (document.hidden) return;
    this._render();
    if (this._refreshWhenVisible) {
      this._refreshWhenVisible = false;
      this.refresh();
    }
  }

  _scheduleRestart() {
    if (this._restartQueued) return;
    this._restartQueued = true;
    queueMicrotask(() => {
      this._restartQueued = false;
      if (!this._connected) return;
      this._startTimers();
      if (this.endpoint && !this._manual) this.refresh();
    });
  }

  _render() {
    if (!this._root) return;
    this._root.replaceChildren();
    for (const provider of this._snapshot?.providers ?? []) {
      this._root.append(this._providerView(provider));
    }
    if (!this._root.childElementCount) {
      const empty = document.createElement("div");
      empty.className = this._error ? "status error" : "empty";
      empty.setAttribute("part", this._error ? "error" : "empty");
      empty.textContent = this._error || "No usage providers available.";
      this._root.append(empty);
    } else if (this._error) {
      const status = document.createElement("div");
      status.className = "status error";
      status.setAttribute("part", "error");
      status.textContent = this._error;
      this._root.append(status);
    }
  }

  _providerView(provider) {
    const section = document.createElement("section");
    section.className = "provider";
    section.setAttribute("part", "provider");

    const title = document.createElement("h3");
    const icon = iconView(provider?.id);
    if (icon) title.append(icon);
    title.append(document.createTextNode(provider?.name || provider?.id || "Provider"));
    section.append(title);

    for (const window of provider?.windows ?? []) {
      section.append(this._windowView(window));
    }

    if (!provider?.windows?.length || provider?.error) {
      const status = document.createElement("div");
      status.className = `status${provider?.error ? " error" : ""}`;
      status.setAttribute("part", provider?.error ? "error" : "status");
      const prefix = provider?.stale ? "Stale data · " : "";
      status.textContent = prefix + (provider?.error || "No usage windows available.");
      section.append(status);
    }
    return section;
  }

  _windowView(usageWindow) {
    const item = document.createElement("div");
    item.className = "window";
    item.setAttribute("part", "window");

    const meta = document.createElement("div");
    meta.className = "meta";
    const description = document.createElement("span");
    description.className = "description";
    const label = document.createElement("span");
    label.textContent = `${usageWindow?.label || "Usage"}${usageWindow?.scope ? ` · ${usageWindow.scope}` : ""}`;
    const reset = document.createElement("span");
    reset.className = "reset";
    reset.textContent = resetText(usageWindow?.resets_at);
    const used = document.createElement("span");
    used.className = "used";
    used.textContent = `${formatPercent(usageWindow?.used_percent)} used`;
    description.append(label, reset);
    meta.append(description, used);

    const track = document.createElement("div");
    track.className = "track";
    track.setAttribute("part", "track");
    track.setAttribute("role", "progressbar");
    track.setAttribute("aria-valuemin", "0");
    track.setAttribute("aria-valuemax", "100");
    track.setAttribute("aria-valuenow", String(clamp(usageWindow?.used_percent)));
    track.setAttribute("aria-label", `${usageWindow?.label || "Usage"} used`);
    const fill = document.createElement("span");
    fill.className = "fill";
    fill.setAttribute("part", "fill");
    fill.style.width = `${clamp(usageWindow?.used_percent)}%`;
    const marker = document.createElement("span");
    marker.className = "marker";
    marker.setAttribute("aria-hidden", "true");
    marker.style.left = `${elapsedPercent(usageWindow)}%`;
    track.append(fill, marker);
    item.append(meta, track);
    return item;
  }
}

function iconView(id) {
  if (!Object.hasOwn(ICON_PATHS, id ?? "")) return null;
  const pathValue = ICON_PATHS[id];
  const svg = document.createElementNS("http://www.w3.org/2000/svg", "svg");
  svg.classList.add("icon", `icon--${id}`);
  svg.setAttribute("part", "icon");
  svg.setAttribute("viewBox", "0 0 100 100");
  svg.setAttribute("aria-hidden", "true");
  const path = document.createElementNS("http://www.w3.org/2000/svg", "path");
  path.setAttribute("d", pathValue);
  svg.append(path);
  return svg;
}

function elapsedPercent(window) {
  const reset = Date.parse(window?.resets_at);
  const duration = Number(window?.window_seconds) * 1000;
  if (!Number.isFinite(reset) || !Number.isFinite(duration) || duration <= 0) return 0;
  return clamp(((Date.now() - (reset - duration)) / duration) * 100);
}

function resetText(value) {
  const reset = Date.parse(value);
  if (!Number.isFinite(reset)) return "";
  return ` · resets in ${durationText(Math.max(0, reset - Date.now()))}`;
}

function durationText(milliseconds) {
  const totalMinutes = Math.max(0, Math.ceil(milliseconds / 60000));
  const days = Math.floor(totalMinutes / 1440);
  const hours = Math.floor((totalMinutes % 1440) / 60);
  const minutes = totalMinutes % 60;
  if (days > 0) return hours > 0 ? `${days}d ${hours}h` : `${days}d`;
  if (hours > 0) return minutes > 0 ? `${hours}h ${minutes}m` : `${hours}h`;
  return `${minutes}m`;
}

function formatPercent(value) {
  const used = clamp(value);
  return `${Number.isInteger(used) ? used : used.toFixed(1)}%`;
}

function clamp(value) {
  const number = Number(value);
  if (!Number.isFinite(number)) return 0;
  return Math.max(0, Math.min(100, number));
}

if (!customElements.get("agent-usage")) {
  customElements.define("agent-usage", AgentUsage);
}
