import { act, renderHook } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { GitLabMRFeedback } from "@/lib/types/gitlab";
import type { ReloadResult } from "./use-mr-feedback";
import {
  POST_READY_MAX_RELOADS,
  POST_READY_RELOAD_DELAY_MS,
  usePostReadyWait,
} from "./use-post-ready-wait";

type MR = GitLabMRFeedback["mr"];
const ok = (status: string): ReloadResult => ({
  outcome: "ok",
  mr: { detailed_merge_status: status } as MR,
});
const SCOPE = { identityKey: "a", revision: 0 };

async function flush() {
  await act(async () => {
    await Promise.resolve();
    await Promise.resolve();
  });
}

describe("usePostReadyWait", () => {
  beforeEach(() => vi.useFakeTimers());
  afterEach(() => vi.useRealTimers());

  it("reloads immediately, holds while transient, and stops at the first settled status", async () => {
    const reload = vi
      .fn<() => Promise<ReloadResult>>()
      .mockResolvedValueOnce(ok("checking"))
      .mockResolvedValueOnce(ok("mergeable"));
    const { result } = renderHook(() => usePostReadyWait(SCOPE, reload));
    expect(result.current.holding).toBe(false);

    await act(async () => {
      await result.current.trackReady(async () => true);
    });
    expect(reload).toHaveBeenCalledTimes(1);
    expect(result.current.holding).toBe(true);

    await act(async () => {
      await vi.advanceTimersByTimeAsync(POST_READY_RELOAD_DELAY_MS);
    });
    expect(reload).toHaveBeenCalledTimes(2);
    expect(result.current.holding).toBe(false);
    await act(async () => {
      await vi.advanceTimersByTimeAsync(POST_READY_RELOAD_DELAY_MS * 3);
    });
    expect(reload).toHaveBeenCalledTimes(2);
  });

  it("stops after the reload cap, counting failed reads", async () => {
    const reload = vi.fn<() => Promise<ReloadResult>>().mockResolvedValue({ outcome: "failed" });
    const { result } = renderHook(() => usePostReadyWait(SCOPE, reload));
    await act(async () => {
      await result.current.trackReady(async () => true);
    });
    await act(async () => {
      await vi.advanceTimersByTimeAsync(POST_READY_RELOAD_DELAY_MS * 10);
    });
    expect(reload).toHaveBeenCalledTimes(POST_READY_MAX_RELOADS);
    expect(result.current.holding).toBe(false);
  });

  it("ends the wait when a reload is superseded", async () => {
    const reload = vi
      .fn<() => Promise<ReloadResult>>()
      .mockResolvedValue({ outcome: "superseded" });
    const { result } = renderHook(() => usePostReadyWait(SCOPE, reload));
    await act(async () => {
      await result.current.trackReady(async () => true);
    });
    await flush();
    expect(result.current.holding).toBe(false);
    expect(reload).toHaveBeenCalledTimes(1);
  });

  it("ends the wait and clears the timer on a manual refresh", async () => {
    const reload = vi.fn<() => Promise<ReloadResult>>().mockResolvedValue(ok("checking"));
    const { result, rerender } = renderHook(({ scope }) => usePostReadyWait(scope, reload), {
      initialProps: { scope: SCOPE },
    });
    await act(async () => {
      await result.current.trackReady(async () => true);
    });
    expect(result.current.holding).toBe(true);
    rerender({ scope: { ...SCOPE, revision: 1 } });
    expect(result.current.holding).toBe(false);
    await act(async () => {
      await vi.advanceTimersByTimeAsync(POST_READY_RELOAD_DELAY_MS * 2);
    });
    expect(reload).toHaveBeenCalledTimes(1);
  });

  it("ends the wait on an identity change and on unmount", async () => {
    const reload = vi.fn<() => Promise<ReloadResult>>().mockResolvedValue(ok("checking"));
    const { result, rerender, unmount } = renderHook(
      ({ scope }) => usePostReadyWait(scope, reload),
      { initialProps: { scope: SCOPE } },
    );
    await act(async () => {
      await result.current.trackReady(async () => true);
    });
    rerender({ scope: { identityKey: "b", revision: 0 } });
    expect(result.current.holding).toBe(false);
    unmount();
    await vi.advanceTimersByTimeAsync(POST_READY_RELOAD_DELAY_MS * 2);
    expect(reload).toHaveBeenCalledTimes(1);
  });
});

describe("usePostReadyWait requests in flight", () => {
  beforeEach(() => vi.useFakeTimers());
  afterEach(() => vi.useRealTimers());

  it("starts no wait when the identity changed while the request was in flight", async () => {
    const reload = vi.fn<() => Promise<ReloadResult>>().mockResolvedValue(ok("checking"));
    const { result, rerender } = renderHook(({ scope }) => usePostReadyWait(scope, reload), {
      initialProps: { scope: SCOPE },
    });
    let finish!: () => void;
    const request = new Promise<void>((resolve) => {
      finish = resolve;
    });
    let tracked!: Promise<void>;
    act(() => {
      tracked = result.current.trackReady(() => request);
    });
    rerender({ scope: { identityKey: "b", revision: 0 } });
    await act(async () => {
      finish();
      await tracked;
    });
    expect(reload).not.toHaveBeenCalled();
    expect(result.current.holding).toBe(false);
  });

  it("starts no wait when a refresh happened or the panel unmounted during the request", async () => {
    const reload = vi.fn<() => Promise<ReloadResult>>().mockResolvedValue(ok("checking"));
    const { result, rerender, unmount } = renderHook(
      ({ scope }) => usePostReadyWait(scope, reload),
      { initialProps: { scope: SCOPE } },
    );
    let finish!: () => void;
    const request = new Promise<void>((resolve) => {
      finish = resolve;
    });
    let tracked!: Promise<void>;
    act(() => {
      tracked = result.current.trackReady(() => request);
    });
    rerender({ scope: { ...SCOPE, revision: 1 } });
    await act(async () => {
      finish();
      await tracked;
    });
    expect(reload).not.toHaveBeenCalled();

    let finishSecond!: () => void;
    const second = new Promise<void>((resolve) => {
      finishSecond = resolve;
    });
    let trackedSecond!: Promise<void>;
    act(() => {
      trackedSecond = result.current.trackReady(() => second);
    });
    unmount();
    await act(async () => {
      finishSecond();
      await trackedSecond;
    });
    expect(reload).not.toHaveBeenCalled();
  });

  it("a newer request wins over an older one still in flight", async () => {
    const reload = vi.fn<() => Promise<ReloadResult>>().mockResolvedValue(ok("mergeable"));
    const { result } = renderHook(() => usePostReadyWait(SCOPE, reload));
    let finishFirst!: () => void;
    const first = new Promise<void>((resolve) => {
      finishFirst = resolve;
    });
    let trackedFirst!: Promise<void>;
    act(() => {
      trackedFirst = result.current.trackReady(() => first);
    });
    await act(async () => {
      await result.current.trackReady(async () => true);
    });
    expect(reload).toHaveBeenCalledTimes(1);
    await act(async () => {
      finishFirst();
      await trackedFirst;
    });
    expect(reload).toHaveBeenCalledTimes(1);
  });
});

describe("usePostReadyWait scope ownership", () => {
  it("does not start when the identity changes away and back during the request", async () => {
    const reload = vi.fn<() => Promise<ReloadResult>>().mockResolvedValue(ok("checking"));
    const { result, rerender } = renderHook(({ scope }) => usePostReadyWait(scope, reload), {
      initialProps: { scope: SCOPE },
    });
    let finish!: () => void;
    const request = new Promise<void>((resolve) => {
      finish = resolve;
    });
    let tracked!: Promise<void>;
    act(() => {
      tracked = result.current.trackReady(() => request);
    });
    rerender({ scope: { identityKey: "b", revision: 0 } });
    rerender({ scope: SCOPE });
    await act(async () => {
      finish();
      await tracked;
    });
    expect(reload).not.toHaveBeenCalled();
    expect(result.current.holding).toBe(false);
  });
});
