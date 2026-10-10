import { afterEach, describe, expect, it, vi } from "vitest";
import { createAppStore } from "@/lib/state/store";
import { invalidateIntegrationAvailability } from "@/lib/integrations/integration-availability-events";
import { getIntegrationHealthResourceScope } from "./integration-health-resource";

function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((resolvePromise) => {
    resolve = resolvePromise;
  });
  return { promise, resolve };
}

function setupScope() {
  const store = createAppStore();
  store.getState().setActiveWorkspace("ws-1");
  const scope = getIntegrationHealthResourceScope(store);
  return { store, scope };
}

afterEach(() => vi.useRealTimers());

describe("integration health resource", () => {
  it("lets concurrent null results settle once and reuses the settled result", async () => {
    const { scope } = setupScope();
    const key = scope.key("jira", "ws-1");
    const fetch = vi.fn(async () => null);
    const releaseFirst = scope.subscribe(key, vi.fn(), fetch, 1_000);
    const releaseSecond = scope.subscribe(key, vi.fn(), fetch, 1_000);

    await vi.waitFor(() => expect(scope.getSnapshot(key).loaded).toBe(true));
    expect(fetch).toHaveBeenCalledTimes(1);
    expect(scope.getSnapshot(key)).toMatchObject({ value: null, loading: false, error: null });
    releaseFirst();
    releaseSecond();

    const releaseAgain = scope.subscribe(key, vi.fn(), fetch, 1_000);
    expect(fetch).toHaveBeenCalledTimes(1);
    releaseAgain();
  });

  it("refreshes active health consumers after credential invalidation", async () => {
    const { scope } = setupScope();
    const key = scope.key("linear", "ws-1");
    const fetch = vi
      .fn<() => Promise<{ hasSecret: boolean; lastOk: boolean } | null>>()
      .mockResolvedValueOnce(null)
      .mockResolvedValueOnce({ hasSecret: true, lastOk: true });
    const release = scope.subscribe(key, vi.fn(), fetch, 10_000);

    await vi.waitFor(() => expect(fetch).toHaveBeenCalledTimes(1));
    invalidateIntegrationAvailability();
    await vi.waitFor(() => expect(fetch).toHaveBeenCalledTimes(2));

    await vi.waitFor(() =>
      expect(scope.getSnapshot(key).value).toEqual({ hasSecret: true, lastOk: true }),
    );
    release();
  });

  it("stops the polling timer when the last consumer releases", async () => {
    vi.useFakeTimers();
    const { scope } = setupScope();
    const key = scope.key("azure-devops", "ws-1");
    const fetch = vi.fn(async () => null);
    const release = scope.subscribe(key, vi.fn(), fetch, 100);
    await Promise.resolve();
    await Promise.resolve();
    expect(fetch).toHaveBeenCalledTimes(1);

    release();
    await vi.advanceTimersByTimeAsync(500);

    expect(fetch).toHaveBeenCalledTimes(1);
  });

  it("rejects an old health response after the workspace changes", async () => {
    const { store, scope } = setupScope();
    const key = scope.key("jira", "ws-1");
    const request = deferred<{ hasSecret: boolean; lastOk: boolean } | null>();
    const release = scope.subscribe(key, vi.fn(), () => request.promise, 10_000);
    await Promise.resolve();

    store.getState().setActiveWorkspace("ws-2");
    request.resolve({ hasSecret: true, lastOk: true });
    await Promise.resolve();
    await Promise.resolve();

    const currentScope = getIntegrationHealthResourceScope(store);
    expect(currentScope).not.toBe(scope);
    expect(currentScope.getSnapshot(key)).toMatchObject({ value: null, loaded: false });
    release();
  });
});
