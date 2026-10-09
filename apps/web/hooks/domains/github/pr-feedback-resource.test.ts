import { describe, expect, it, vi } from "vitest";
import type { PRFeedback } from "@/lib/types/github";
import { createAppStore } from "@/lib/state/store";
import { getPRFeedbackResourceScope } from "./pr-feedback-resource";

function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (reason: unknown) => void;
  const promise = new Promise<T>((resolvePromise, rejectPromise) => {
    resolve = resolvePromise;
    reject = rejectPromise;
  });
  return { promise, resolve, reject };
}

function makeFeedback(body: string): PRFeedback {
  return { comments: [{ body }] } as unknown as PRFeedback;
}

function setupScope() {
  const store = createAppStore();
  store.getState().setActiveWorkspace("ws-1");
  const scope = getPRFeedbackResourceScope(store);
  const key = scope.key({ workspaceId: "ws-1", owner: "acme", repo: "app", prNumber: 7 });
  return { store, scope, key };
}

describe("PR feedback resource", () => {
  it("lets three consumers share one deferred request", async () => {
    const { scope, key } = setupScope();
    const request = deferred<PRFeedback>();
    const fetch = vi.fn(() => request.promise);
    const releases = [
      scope.subscribe(key, vi.fn()),
      scope.subscribe(key, vi.fn()),
      scope.subscribe(key, vi.fn()),
    ];

    const first = scope.ensure(key, fetch);
    const second = scope.ensure(key, fetch);
    const third = scope.ensure(key, fetch);

    await Promise.resolve();
    expect(fetch).toHaveBeenCalledTimes(1);
    expect(scope.getSnapshot(key).loading).toBe(true);
    request.resolve(makeFeedback("shared"));
    await Promise.all([first, second, third]);
    expect(scope.getSnapshot(key).feedback?.comments[0]?.body).toBe("shared");
    releases.forEach((release) => release());
  });

  it("joins repeated consumers of one update token and refreshes on a newer token", async () => {
    const { scope, key } = setupScope();
    const release = scope.subscribe(key, vi.fn());
    const fetch = vi.fn(async () => makeFeedback("updated"));

    await scope.ensure(key, fetch, "version-1");
    await scope.ensure(key, fetch, "version-1");
    expect(fetch).toHaveBeenCalledTimes(1);

    await scope.ensure(key, fetch, "version-2");
    expect(fetch).toHaveBeenCalledTimes(2);
    release();
  });

  it("queues one follow-up when invalidated during an outstanding read", async () => {
    const { scope, key } = setupScope();
    const first = deferred<PRFeedback>();
    const second = deferred<PRFeedback>();
    const fetch = vi.fn().mockReturnValueOnce(first.promise).mockReturnValueOnce(second.promise);

    const initial = scope.ensure(key, fetch);
    const refreshOne = scope.invalidate(key, fetch);
    const refreshTwo = scope.invalidate(key, fetch);
    await Promise.resolve();
    expect(fetch).toHaveBeenCalledTimes(1);

    first.resolve(makeFeedback("initial"));
    await initial;
    await vi.waitFor(() => expect(fetch).toHaveBeenCalledTimes(2));
    second.resolve(makeFeedback("fresh"));
    await Promise.all([refreshOne, refreshTwo]);

    expect(scope.getSnapshot(key).feedback?.comments[0]?.body).toBe("fresh");
    expect(scope.getSnapshot(key).loading).toBe(false);
  });

  it("rejects an old-workspace response after the workspace changes", async () => {
    const { store, scope, key } = setupScope();
    const request = deferred<PRFeedback>();
    const pending = scope.ensure(key, () => request.promise);

    store.getState().setActiveWorkspace("ws-2");
    request.resolve(makeFeedback("old workspace"));
    await pending;

    expect(getPRFeedbackResourceScope(store)).not.toBe(scope);
    expect(store.getState().prFeedbackCache.byKey[key]).toBeUndefined();
  });

  it("retains same-PR feedback when a refresh fails", async () => {
    const { scope, key } = setupScope();
    await scope.ensure(key, async () => makeFeedback("usable feedback"));

    await scope.invalidate(key, async () => {
      throw new Error("temporary failure");
    });

    expect(scope.getSnapshot(key).feedback?.comments[0]?.body).toBe("usable feedback");
    expect(scope.getSnapshot(key).error).toBeInstanceOf(Error);
    expect(scope.getSnapshot(key).loading).toBe(false);
  });
});
