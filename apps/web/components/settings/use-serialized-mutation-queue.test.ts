import { act, renderHook } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { useSerializedMutationQueue } from "./use-serialized-mutation-queue";

function deferred() {
  let resolve!: () => void;
  let reject!: (error: Error) => void;
  const promise = new Promise<void>((resolvePromise, rejectPromise) => {
    resolve = resolvePromise;
    reject = rejectPromise;
  });
  return { promise, resolve, reject };
}

describe("useSerializedMutationQueue", () => {
  it("runs mutations in the order they were queued", async () => {
    const first = deferred();
    const calls: string[] = [];
    const { result } = renderHook(() => useSerializedMutationQueue(vi.fn()));

    let firstResult!: Promise<boolean>;
    let secondResult!: Promise<boolean>;
    act(() => {
      firstResult = result.current.run(async () => {
        calls.push("first:start");
        await first.promise;
        calls.push("first:end");
      }, "first failed");
      secondResult = result.current.run(async () => {
        calls.push("second");
      }, "second failed");
    });

    expect(calls).toEqual(["first:start"]);
    await act(async () => first.resolve());
    await expect(firstResult).resolves.toBe(true);
    await expect(secondResult).resolves.toBe(true);
    expect(calls).toEqual(["first:start", "first:end", "second"]);
  });

  it("pauses later mutations and retries the exact failed operation", async () => {
    const calls: string[] = [];
    let firstAttempt = true;
    const onError = vi.fn();
    const { result } = renderHook(() => useSerializedMutationQueue(onError));

    let failedResult!: Promise<boolean>;
    let queuedResult!: Promise<boolean>;
    act(() => {
      failedResult = result.current.run(async () => {
        calls.push("first");
        if (firstAttempt) {
          firstAttempt = false;
          throw new Error("offline");
        }
      }, "first failed");
      queuedResult = result.current.run(async () => {
        calls.push("second");
      }, "second failed");
    });

    let saved: boolean | undefined;
    await act(async () => {
      saved = await failedResult;
    });
    expect(saved).toBe(false);
    expect(result.current.status).toBe("error");
    expect(calls).toEqual(["first"]);

    await act(async () => {
      await result.current.retry();
    });
    await expect(queuedResult).resolves.toBe(true);
    expect(calls).toEqual(["first", "first", "second"]);
    expect(onError).toHaveBeenCalledOnce();
  });
});
