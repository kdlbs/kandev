import { describe, expect, it, vi } from "vitest";
import type { StoreApi } from "zustand";
import type { AppState } from "@/lib/state/store";
import { getTaskSessionReads } from "./task-session-reads";

const TASK_ID = "task-1";

function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((innerResolve) => {
    resolve = innerResolve;
  });
  return { promise, resolve };
}

function storeFor(userId: string): StoreApi<AppState> {
  return {
    getState: () =>
      ({
        auth: { mode: "optional", authenticated: true, user: { id: userId } },
        workspaceContextGeneration: 0,
      }) as unknown as AppState,
  } as StoreApi<AppState>;
}

describe("task session read owner", () => {
  it("shares one request and one trailing refresh across consumers", async () => {
    const owner = getTaskSessionReads(storeFor("shared-user"));
    const releaseFirst = owner.retain(TASK_ID);
    const releaseSecond = owner.retain(TASK_ID);
    const first = deferred<{ sessions: never[]; total: number }>();
    const calls = vi
      .fn()
      .mockReturnValueOnce(first.promise)
      .mockResolvedValue({ sessions: [], total: 0 });
    const firstRead = owner.read(TASK_ID, calls);
    const joinedRead = owner.read(TASK_ID, calls);
    const forcedRead = owner.read(TASK_ID, calls, { refresh: true });
    owner.read(TASK_ID, calls, { refresh: true });

    await Promise.resolve();
    expect(calls).toHaveBeenCalledTimes(1);
    expect(joinedRead).toBe(firstRead);

    first.resolve({ sessions: [], total: 0 });
    await Promise.all([firstRead, forcedRead]);
    expect(calls).toHaveBeenCalledTimes(2);

    releaseFirst();
    releaseSecond();
    owner.dispose();
  });

  it("cancels only after the final consumer releases the request", async () => {
    const owner = getTaskSessionReads(storeFor("cancel-user"));
    const releaseFirst = owner.retain(TASK_ID);
    const releaseSecond = owner.retain(TASK_ID);
    let requestSignal: AbortSignal | undefined;
    const request = deferred<{ sessions: never[]; total: number }>();
    const read = owner.read(TASK_ID, (signal) => {
      requestSignal = signal;
      return request.promise;
    });
    await Promise.resolve();

    releaseFirst();
    await Promise.resolve();
    expect(requestSignal?.aborted).toBe(false);

    releaseSecond();
    await Promise.resolve();
    expect(requestSignal?.aborted).toBe(true);
    request.resolve({ sessions: [], total: 0 });
    await expect(read).rejects.toMatchObject({ name: "AbortError" });
    owner.dispose();
  });

  it.each(["user", "workspace", "dispose"])(
    "rejects an ignored-abort response after %s retirement",
    async (change) => {
      let userId = "original";
      let generation = 0;
      const store = {
        getState: () => ({
          auth: { mode: "optional", authenticated: true, user: { id: userId } },
          workspaceContextGeneration: generation,
        }),
      } as unknown as StoreApi<AppState>;
      const owner = getTaskSessionReads(store);
      const release = owner.retain(TASK_ID);
      const response = deferred<{ sessions: never[]; total: number }>();
      const read = owner.read(TASK_ID, () => response.promise);
      const rejection = expect(read).rejects.toMatchObject({ name: "AbortError" });
      if (change === "user") userId = "successor";
      if (change === "workspace") generation++;
      if (change === "dispose") owner.dispose();
      response.resolve({ sessions: [], total: 0 });
      await rejection;
      release();
    },
  );

  it("does not fulfill a released attempt with a replacement consumer's result", async () => {
    const owner = getTaskSessionReads(storeFor("rejoin-user"));
    const oldResponse = deferred<{ sessions: never[]; total: number }>();
    const releaseOld = owner.retain(TASK_ID);
    const oldRead = owner.read(TASK_ID, () => oldResponse.promise);
    const rejectedOldRead = expect(oldRead).rejects.toMatchObject({ name: "AbortError" });
    releaseOld();
    const releaseNew = owner.retain(TASK_ID);
    const newRead = owner.read(TASK_ID, async () => ({ sessions: [], total: 2 }));
    oldResponse.resolve({ sessions: [], total: 1 });
    await rejectedOldRead;
    await expect(newRead).resolves.toEqual({ sessions: [], total: 2 });
    releaseNew();
  });

  it("keeps caches isolated by store", () => {
    const firstStore = storeFor("account-a");
    const secondStore = storeFor("account-a");
    expect(getTaskSessionReads(firstStore)).toBe(getTaskSessionReads(firstStore));
    expect(getTaskSessionReads(firstStore)).not.toBe(getTaskSessionReads(secondStore));
  });
});
