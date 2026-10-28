import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { SwWindowClient } from "./sw-types";

type Handler = (event: unknown) => void;

const ORIGIN = "https://kandev.example";

function makeClient(url: string, focused: boolean, visibilityState = "visible") {
  return {
    url,
    focused,
    visibilityState,
    focus: vi.fn().mockResolvedValue(undefined),
    postMessage: vi.fn(),
  } satisfies SwWindowClient;
}

/** Loads the worker entry against a fake scope and returns its registered listeners. */
async function loadWorker(windows: SwWindowClient[]) {
  const handlers: Record<string, Handler> = {};
  const scope = {
    location: new URL(ORIGIN),
    caches: { open: vi.fn(), keys: vi.fn().mockResolvedValue([]), match: vi.fn() },
    clients: {
      claim: vi.fn().mockResolvedValue(undefined),
      matchAll: vi.fn().mockResolvedValue(windows),
      openWindow: vi.fn().mockResolvedValue(undefined),
    },
    registration: { showNotification: vi.fn().mockResolvedValue(undefined) },
    skipWaiting: vi.fn().mockResolvedValue(undefined),
    addEventListener: (type: string, handler: Handler) => {
      handlers[type] = handler;
    },
  };
  vi.stubGlobal("self", scope);
  vi.stubGlobal("__KANDEV_SW_BUILD_ID__", "test-build");
  vi.stubGlobal("__KANDEV_SW_PRECACHE__", ["/assets/index-abc.js"]);
  vi.resetModules();
  await import("./service-worker");
  return { handlers, scope };
}

function waitUntilEvent(extra: Record<string, unknown> = {}) {
  const pending: Promise<unknown>[] = [];
  const event = { ...extra, waitUntil: (p: Promise<unknown>) => pending.push(p) };
  return { event, settled: () => Promise.all(pending) };
}

describe("service worker push handling", () => {
  beforeEach(() => vi.unstubAllGlobals());
  afterEach(() => vi.unstubAllGlobals());

  it("shows a notification with the tag and deep link when no window is visible", async () => {
    const { handlers, scope } = await loadWorker([]);
    const { event, settled } = waitUntilEvent({
      data: {
        text: () =>
          JSON.stringify({ title: "Agent needs permission", body: "b", tag: "g", url: "/t/1" }),
      },
    });
    handlers.push(event);
    await settled();
    expect(scope.registration.showNotification).toHaveBeenCalledWith(
      "Agent needs permission",
      expect.objectContaining({
        body: "b",
        tag: "g",
        data: expect.objectContaining({ url: "/t/1" }),
      }),
    );
  });

  it("stays silent while a Kandev window is focused and visible", async () => {
    const { handlers, scope } = await loadWorker([makeClient(`${ORIGIN}/`, true)]);
    const { event, settled } = waitUntilEvent({ data: { text: () => '{"title":"x"}' } });
    handlers.push(event);
    await settled();
    expect(scope.registration.showNotification).not.toHaveBeenCalled();
  });

  it("still shows a generic notification for a malformed payload", async () => {
    const { handlers, scope } = await loadWorker([makeClient(`${ORIGIN}/`, false, "hidden")]);
    const { event, settled } = waitUntilEvent({ data: { text: () => "garbage" } });
    handlers.push(event);
    await settled();
    expect(scope.registration.showNotification).toHaveBeenCalledTimes(1);
  });
});

describe("service worker notification clicks", () => {
  afterEach(() => vi.unstubAllGlobals());

  function click(url: string) {
    const close = vi.fn();
    return waitUntilEvent({ notification: { close, data: { url } } });
  }

  it("focuses an open window and asks the app to navigate", async () => {
    const existing = makeClient(`${ORIGIN}/tasks`, true);
    const { handlers, scope } = await loadWorker([existing]);
    const { event, settled } = click("/t/1?sessionId=s");
    handlers.notificationclick(event);
    await settled();
    expect(existing.focus).toHaveBeenCalled();
    expect(existing.postMessage).toHaveBeenCalledWith({
      type: "kandev:navigate",
      url: "/t/1?sessionId=s",
    });
    expect(scope.clients.openWindow).not.toHaveBeenCalled();
  });

  it("opens the deep link when no window exists", async () => {
    const { handlers, scope } = await loadWorker([]);
    const { event, settled } = click("/t/9");
    handlers.notificationclick(event);
    await settled();
    expect(scope.clients.openWindow).toHaveBeenCalledWith("/t/9");
  });

  it("refuses to open an off-origin target", async () => {
    const { handlers, scope } = await loadWorker([]);
    const { event, settled } = click("https://evil.example/");
    handlers.notificationclick(event);
    await settled();
    expect(scope.clients.openWindow).toHaveBeenCalledWith("/");
  });
});

describe("service worker fetch handling", () => {
  afterEach(() => vi.unstubAllGlobals());

  it("does not respond to API requests, leaving them to the network", async () => {
    const { handlers } = await loadWorker([]);
    const respondWith = vi.fn();
    handlers.fetch({
      request: { method: "GET", mode: "cors", url: `${ORIGIN}/api/v1/tasks` },
      respondWith,
    });
    expect(respondWith).not.toHaveBeenCalled();
  });

  it("answers a failed navigation with the cached offline page", async () => {
    const { handlers, scope } = await loadWorker([]);
    const offline = new Response("offline");
    scope.caches.match.mockResolvedValue(offline);
    vi.stubGlobal("fetch", vi.fn().mockRejectedValue(new TypeError("network down")));
    const respondWith = vi.fn();
    handlers.fetch({
      request: { method: "GET", mode: "navigate", url: `${ORIGIN}/t/1` },
      respondWith,
    });
    expect(respondWith).toHaveBeenCalledTimes(1);
    await expect(respondWith.mock.calls[0][0]).resolves.toBe(offline);
  });

  it("passes an access-proxy redirect through untouched and never caches it", async () => {
    const { handlers, scope } = await loadWorker([]);
    const redirect = { type: "opaqueredirect", status: 0, ok: false } as unknown as Response;
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(redirect));
    const respondWith = vi.fn();
    handlers.fetch({
      request: { method: "GET", mode: "navigate", url: `${ORIGIN}/` },
      respondWith,
    });
    await expect(respondWith.mock.calls[0][0]).resolves.toBe(redirect);
    expect(scope.caches.open).not.toHaveBeenCalled();
  });

  it("claims clients and deletes stale caches on activate", async () => {
    const { handlers, scope } = await loadWorker([]);
    const del = vi.fn().mockResolvedValue(true);
    scope.caches = {
      ...scope.caches,
      keys: vi.fn().mockResolvedValue(["kandev-shell-old"]),
      delete: del,
    } as never;
    const { event, settled } = waitUntilEvent();
    handlers.activate(event);
    await settled();
    expect(del).toHaveBeenCalledWith("kandev-shell-old");
    expect(scope.clients.claim).toHaveBeenCalled();
  });
});
