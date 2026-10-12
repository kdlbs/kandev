import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { createAppStore } from "./store";
import { getSessionReadScope, subscribeSessionReadScope } from "./session-read-coordinator";
import { setWebSocketClient } from "@/lib/ws/connection";
import type { WebSocketClient } from "@/lib/ws/client";

function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((done) => {
    resolve = done;
  });
  return { promise, resolve };
}
function setup() {
  const store = createAppStore();
  store.getState().setConnectionStatus("connected");
  const scope = getSessionReadScope(store);
  const read = (key: string, fetch = async () => ({ data: key })) =>
    scope.get({
      resource: "diff",
      key,
      environmentId: key,
      isCurrent: () => true,
      fetch,
    });
  return { store, scope, read };
}
beforeEach(() => {
  vi.useFakeTimers();
  setWebSocketClient({ request: vi.fn() } as unknown as WebSocketClient);
});
afterEach(() => {
  setWebSocketClient(null);
  vi.useRealTimers();
});

// @covers AC-UI-TASK-NAVIGATION-RESPONSIVENESS-001.2
// @covers AC-UI-TASK-NAVIGATION-RESPONSIVENESS-001.5
describe("read completion and invalidation", () => {
  it("settles and trims detached reads after their environment binding expires", async () => {
    const { scope } = setup();
    let current = true;
    const response = deferred<{ data: string }>();
    const entries = Array.from({ length: 33 }, (_, i) => {
      const entry = scope.get({
        resource: "diff",
        key: `key-${i}`,
        environmentId: `environment-${i}`,
        isCurrent: () => current,
        fetch: () => response.promise,
      });
      const release = entry.subscribe(() => {});
      entry.ensure();
      release();
      return entry;
    });
    current = false;
    response.resolve({ data: "obsolete" });
    await vi.advanceTimersByTimeAsync(0);
    expect(entries[32].getSnapshot()).toEqual({ data: undefined, loading: false, error: null });
    expect(scope.get(entries[0].options)).not.toBe(entries[0]);
  });

  it("schedules another read when invalidated during the coalesced follow-up", async () => {
    const { read } = setup();
    const first = deferred<{ data: string }>();
    const second = deferred<{ data: string }>();
    const fetch = vi
      .fn()
      .mockReturnValueOnce(first.promise)
      .mockReturnValueOnce(second.promise)
      .mockResolvedValue({ data: "latest" });
    const entry = read("key", fetch);
    const release = entry.subscribe(() => {});
    entry.ensure();
    entry.invalidate();
    first.resolve({ data: "obsolete-first" });
    await vi.advanceTimersByTimeAsync(0);
    expect(fetch).toHaveBeenCalledTimes(2);
    for (let i = 0; i < 5; i++) entry.invalidate(200);
    expect(fetch).toHaveBeenCalledTimes(2);
    second.resolve({ data: "obsolete-follow-up" });
    await vi.advanceTimersByTimeAsync(1000);
    expect(fetch).toHaveBeenCalledTimes(3);
    expect(entry.getSnapshot()).toEqual({ data: "latest", loading: false, error: null });
    release();
  });
});

describe("read coordination lifetime", () => {
  it("retains active work while evicting the oldest inactive results", async () => {
    const { read } = setup();
    const pending = deferred<{ data: string }>();
    const firstFetch = vi.fn(() => pending.promise);
    const active = read("active", firstFetch);
    const unsubscribe = active.subscribe(() => {});
    active.ensure();
    const oldest = read("oldest");
    const release = oldest.subscribe(() => {});
    oldest.ensure();
    await vi.advanceTimersByTimeAsync(0);
    release();
    for (let i = 0; i < 33; i++) {
      const next = read(`key-${i}`);
      const dispose = next.subscribe(() => {});
      next.ensure();
      await vi.advanceTimersByTimeAsync(0);
      dispose();
    }
    expect(read("oldest")).not.toBe(oldest);
    expect(read("active", firstFetch)).toBe(active);
    active.ensure();
    expect(firstFetch).toHaveBeenCalledTimes(1);
    pending.resolve({ data: "kept" });
    await vi.advanceTimersByTimeAsync(0);
    expect(active.getSnapshot().data).toBe("kept");
    unsubscribe();
  });

  it("keeps failed reads retryable without an automatic error loop", async () => {
    const { read } = setup();
    const fetch = vi
      .fn()
      .mockRejectedValueOnce(new Error("offline"))
      .mockResolvedValue({ data: "ready" });
    const entry = read("key", fetch);
    const release = entry.subscribe(() => {});
    entry.ensure();
    await vi.advanceTimersByTimeAsync(10000);
    expect(fetch).toHaveBeenCalledTimes(1);
    expect(entry.getSnapshot().loading).toBe(false);
    expect(entry.getSnapshot().error).toBeInstanceOf(Error);
    await entry.refetch();
    expect(entry.getSnapshot()).toEqual({ data: "ready", loading: false, error: null });
    release();
  });

  it("retains a dirty result after the last subscriber leaves, then refreshes on return", async () => {
    const { read } = setup();
    const fetch = vi
      .fn()
      .mockResolvedValueOnce({ data: "old" })
      .mockResolvedValue({ data: "fresh" });
    const entry = read("key", fetch);
    const release = entry.subscribe(() => {});
    entry.ensure();
    await vi.advanceTimersByTimeAsync(0);
    entry.invalidate(200);
    release();
    await vi.advanceTimersByTimeAsync(1000);
    expect(fetch).toHaveBeenCalledTimes(1);
    const releaseAgain = entry.subscribe(() => {});
    entry.ensure();
    await vi.advanceTimersByTimeAsync(0);
    expect(entry.getSnapshot().data).toBe("fresh");
    releaseAgain();
  });
});

describe("read scope retirement", () => {
  it.each(["workspace", "auth", "reconnect", "client"] as const)(
    "rejects a pending response across a %s change",
    async (transition) => {
      const { store, scope, read } = setup();
      const response = deferred<{ data: string }>();
      const entry = read("key", () => response.promise);
      const release = entry.subscribe(() => {});
      const notify = vi.fn();
      const releaseScope = subscribeSessionReadScope(store, notify);
      entry.ensure();
      if (transition === "workspace") store.setState({ workspaceContextGeneration: 1 });
      if (transition === "auth")
        store.getState().setAuthState({ mode: "enabled", authenticated: false, user: null });
      if (transition === "reconnect") {
        store.getState().setConnectionStatus("disconnected");
        store.getState().setConnectionStatus("connected");
      }
      if (transition === "client")
        setWebSocketClient({ request: vi.fn() } as unknown as WebSocketClient);
      expect(getSessionReadScope(store)).not.toBe(scope);
      response.resolve({ data: "obsolete" });
      await vi.advanceTimersByTimeAsync(0);
      expect(entry.getSnapshot().data).toBeUndefined();
      expect(notify).toHaveBeenCalled();
      release();
      releaseScope();
    },
  );
});
