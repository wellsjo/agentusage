import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { runInNewContext } from "node:vm";
import test from "node:test";

const source = readFileSync(new URL("./agent-usage.js", import.meta.url), "utf8");
const healthy = { providers: [{ id: "codex", windows: [{}] }, { id: "claude", windows: [{}] }] };
const failed = { providers: [{ id: "codex", windows: [{}] }, { id: "claude", windows: [], error: "expired" }] };
const settle = () => new Promise(setImmediate);

// Only the browser boundary is stubbed: the component's real lifecycle,
// requests, scheduling, and error transitions run with a deterministic clock.
function harness(respond = () => healthy, attributes = {}) {
  let now = 0, nextID = 0, calls = 0, Component;
  const timers = new Map();
  const listeners = new Map();
  const setTimer = (fn, delay, repeat) => {
    const id = ++nextID;
    timers.set(id, { fn, delay, repeat, at: now + delay });
    return id;
  };
  const node = () => ({ append() {}, setAttribute() {} });
  const document = {
    hidden: false,
    createElement: node,
    addEventListener: (event, fn) => listeners.set(event, fn),
    removeEventListener: (event) => listeners.delete(event),
  };
  class HTMLElement {
    constructor() { this.attributes = new Map(Object.entries(attributes)); }
    attachShadow() { return node(); }
    getAttribute(name) { return this.attributes.get(name) ?? null; }
    hasAttribute(name) { return this.attributes.has(name); }
    setAttribute(name, value) { this.attributes.set(name, String(value)); this.attributeChangedCallback(); }
    removeAttribute(name) { this.attributes.delete(name); this.attributeChangedCallback(); }
    dispatchEvent() {}
  }
  // Strip only the module export so this unchanged browser script can run in
  // an isolated VM without a DOM package or global test-state pollution.
  runInNewContext(source.replace("export class AgentUsage", "class AgentUsage"), {
    HTMLElement, document, AbortController, queueMicrotask,
    CustomEvent: class { constructor(type, detail) { this.type = type; this.detail = detail; } },
    customElements: { get() {}, define(_name, value) { Component = value; } },
    setInterval: (fn, delay) => setTimer(fn, delay, true),
    clearInterval: (id) => timers.delete(id),
    setTimeout: (fn, delay) => setTimer(fn, delay, false),
    clearTimeout: (id) => timers.delete(id),
    fetch: async (_url, options) => {
      calls++;
      const data = await respond(calls, options.signal);
      return { ok: true, json: async () => data };
    },
  });
  const element = new Component();
  element._render = () => {}; // Rendering is unrelated to the poll scheduler.
  return {
    element, document, get calls() { return calls; },
    async connect() { element.connectedCallback(); await settle(); },
    async visibility(hidden) {
      document.hidden = hidden;
      listeners.get("visibilitychange")?.();
      await settle();
    },
    async advance(ms) {
      const end = now + ms;
      for (;;) {
        const due = [...timers].filter(([, timer]) => timer.at <= end).sort((a, b) => a[1].at - b[1].at)[0];
        if (!due) break;
        const [id, timer] = due;
        now = timer.at;
        if (timer.repeat) timer.at += timer.delay;
        else timers.delete(id);
        timer.fn();
        await settle();
      }
      now = end;
    },
  };
}

test("provider errors poll after five seconds and recover to normal cadence", async () => {
  const h = harness((call) => call === 1 ? failed : healthy, { "refresh-ms": "120000" });
  await h.connect();
  await h.advance(4999);
  assert.equal(h.calls, 1);
  await h.advance(1);
  assert.equal(h.calls, 2, "expired Claude should retry at five seconds despite healthy Codex");
  assert.equal(h.element.snapshot.providers[1].error, undefined);
  await h.advance(119999);
  assert.equal(h.calls, 2, "healthy results should return to the normal interval");
  await h.advance(1);
  assert.equal(h.calls, 3);
  h.element.disconnectedCallback();
});

for (const [name, response] of [
  ["stale provider", () => ({ providers: [{ windows: [{}], stale: true, error: "unavailable" }] })],
  ["endpoint failure", () => { throw new Error("offline"); }],
]) {
  test(`${name} uses fast recovery polling`, async () => {
    const h = harness(response);
    await h.connect();
    await h.advance(10000);
    assert.equal(h.calls, 3);
    h.element.disconnectedCallback();
  });
}

test("hidden tabs defer recovery until visible; disconnect cancels polling", async () => {
  const h = harness(() => failed);
  await h.connect();
  await h.visibility(true);
  await h.advance(20000);
  assert.equal(h.calls, 1);
  await h.visibility(false);
  assert.equal(h.calls, 2);
  await h.advance(5000);
  assert.equal(h.calls, 3);
  h.element.disconnectedCallback();
  await h.advance(120000);
  assert.equal(h.calls, 3);
});

test("disabled and manual modes do not enable recovery polling", async () => {
  const disabled = harness(() => failed, { "refresh-ms": "0" });
  await disabled.connect();
  await disabled.advance(120000);
  assert.equal(disabled.calls, 1);
  disabled.element.disconnectedCallback();
  const manual = harness(() => failed);
  await manual.connect();
  manual.element.snapshot = failed;
  await manual.advance(120000);
  assert.equal(manual.calls, 1);
  manual.element.disconnectedCallback();
});

test("slow recovery requests finish without being aborted or overlapped", async () => {
  let finish, signal;
  const h = harness((call, requestSignal) => {
    if (call === 1) return failed;
    signal = requestSignal;
    return new Promise((resolve) => { finish = resolve; });
  });
  await h.connect();
  await h.advance(5000);
  assert.equal(h.calls, 2);
  await h.advance(15000);
  assert.equal(h.calls, 2);
  assert.equal(signal.aborted, false);
  finish(healthy);
  await settle();
  await h.advance(119999);
  assert.equal(h.calls, 2);
  await h.advance(1);
  assert.equal(h.calls, 3);
  h.element.disconnectedCallback();
  finish(healthy);
  await settle();
  await h.advance(120000);
  assert.equal(h.calls, 3, "late completion must not restart disconnected polling");
});
