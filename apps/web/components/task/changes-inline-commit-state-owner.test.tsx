import { useCallback } from "react";
import { act, cleanup, renderHook } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { CommitDetailTarget } from "@/lib/state/diff-target-types";
import type { FileInfo } from "@/lib/state/store";
import {
  ChangesInlineCommitState,
  useChangesInlineCommitState,
} from "./changes-inline-commit-state";

afterEach(cleanup);

const target: CommitDetailTarget = { source: "local", sha: "commit-a", repo: "backend" };
const TASK_A_CONTEXT = "task-a/session-a/environment-a";
const TASK_B_CONTEXT = "task-b/session-b/environment-b";

function filePayload(): Record<string, FileInfo> {
  return {
    "src/obsolete.ts": {
      path: "src/obsolete.ts",
      status: "modified",
      staged: false,
      additions: 1,
      deletions: 0,
    } as FileInfo,
  };
}

function useInlineStateOwner(
  contextKey: string,
  request: (target: CommitDetailTarget) => Promise<Record<string, FileInfo>>,
  onError: (target: CommitDetailTarget, error: unknown) => void,
) {
  const createState = useCallback(
    (key: string) => new ChangesInlineCommitState({ contextKey: key, request, onError }),
    [onError, request],
  );
  return useChangesInlineCommitState(contextKey, createState);
}

function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (error: unknown) => void;
  const promise = new Promise<T>((resolvePromise, rejectPromise) => {
    resolve = resolvePromise;
    reject = rejectPromise;
  });
  return { promise, resolve, reject };
}

describe("useChangesInlineCommitState owner lifecycle", () => {
  it("ignores obsolete success after the mounted React owner changes context", async () => {
    const response = deferred<Record<string, FileInfo>>();
    const request = vi.fn(() => response.promise);
    const onError = vi.fn();
    const hook = renderHook(({ contextKey }) => useInlineStateOwner(contextKey, request, onError), {
      initialProps: { contextKey: TASK_A_CONTEXT },
    });
    const retiredState = hook.result.current;
    const publish = vi.fn();
    retiredState.subscribe(publish);
    let pending!: Promise<void>;
    act(() => {
      pending = retiredState.expand(target);
    });
    publish.mockClear();

    hook.rerender({ contextKey: TASK_B_CONTEXT });
    const replacementState = hook.result.current;
    expect(replacementState).not.toBe(retiredState);

    await act(async () => {
      response.resolve(filePayload());
      await pending;
    });

    expect(retiredState.get(target)).toMatchObject({ status: "idle", expanded: false });
    expect(replacementState.get(target)).toMatchObject({ status: "idle", expanded: false });
    expect(publish).not.toHaveBeenCalled();
    expect(onError).not.toHaveBeenCalled();
  });

  it("ignores obsolete rejection after the React owner unmounts", async () => {
    const response = deferred<Record<string, FileInfo>>();
    const request = vi.fn(() => response.promise);
    const onError = vi.fn();
    const hook = renderHook(() => useInlineStateOwner(TASK_A_CONTEXT, request, onError));
    let pending!: Promise<void>;
    act(() => {
      pending = hook.result.current.expand(target);
    });

    hook.unmount();
    await act(async () => {
      response.reject(new Error("obsolete request failure"));
      await pending;
    });

    expect(onError).not.toHaveBeenCalled();
  });

  it("keeps the controller and coalesces requests across same-context rerenders", async () => {
    const response = deferred<Record<string, FileInfo>>();
    const request = vi.fn(() => response.promise);
    const onError = vi.fn();
    const hook = renderHook(({ contextKey }) => useInlineStateOwner(contextKey, request, onError), {
      initialProps: { contextKey: TASK_A_CONTEXT },
    });
    const state = hook.result.current;
    let first!: Promise<void>;
    let second!: Promise<void>;
    act(() => {
      first = state.expand(target);
      second = state.expand(target);
    });
    hook.rerender({ contextKey: TASK_A_CONTEXT });

    expect(hook.result.current).toBe(state);
    expect(request).toHaveBeenCalledTimes(1);
    await act(async () => {
      response.resolve(filePayload());
      await Promise.all([first, second]);
    });
    expect(state.get(target)).toMatchObject({ status: "loaded", expanded: true });
  });
});
