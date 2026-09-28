import { act, cleanup } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { setWebSocketClient } from "@/lib/ws/connection";
import type { WebSocketClient } from "@/lib/ws/client";
import { useCumulativeDiff, invalidateCumulativeDiffCache } from "./use-cumulative-diff";
import { deferred, renderSessionRead } from "./session-read-test-helpers";

const request = vi.fn();
const cached = {
  session_id: "session",
  base_commit: "base",
  head_commit: "head",
  total_commits: 1,
  files: {},
};
beforeEach(() => {
  vi.useFakeTimers();
  request.mockReset().mockResolvedValue({ cumulative_diff: cached });
  setWebSocketClient({ request } as unknown as WebSocketClient);
});
afterEach(() => {
  cleanup();
  setWebSocketClient(null);
  vi.useRealTimers();
});
const advance = (ms = 0) =>
  act(async () => {
    await vi.advanceTimersByTimeAsync(ms);
  });

// @covers AC-UI-TASK-NAVIGATION-RESPONSIVENESS-001.2
describe("cumulative diff invalidation", () => {
  it("does not retain a response that settled after the session changed environments", async () => {
    const old = deferred<unknown>();
    request.mockReturnValueOnce(old.promise);
    const { result } = renderSessionRead(() => useCumulativeDiff("session"), undefined);
    act(() => result.current.store.setState({ environmentIdBySessionId: { session: "new-env" } }));
    await advance();
    await act(async () => old.resolve({ cumulative_diff: { ...cached, head_commit: "obsolete" } }));
    act(() =>
      result.current.store.setState({ environmentIdBySessionId: { session: "environment" } }),
    );
    await advance();
    expect(request).toHaveBeenCalledTimes(3);
    expect(result.current.value.diff).toEqual(cached);
  });

  it("coalesces a burst into one refresh for all consumers", async () => {
    const { result } = renderSessionRead(
      () => [useCumulativeDiff("session"), useCumulativeDiff("session")],
      undefined,
    );
    await advance();
    expect(request).toHaveBeenCalledTimes(1);
    act(() => {
      for (let i = 0; i < 5; i++)
        invalidateCumulativeDiffCache(result.current.store, "environment");
    });
    expect(request).toHaveBeenCalledTimes(1);
    await advance(250);
    expect(request).toHaveBeenCalledTimes(2);
    expect(result.current.value.map((v) => v.diff)).toEqual([cached, cached]);
  });

  it("does not refresh after the last subscriber unmounts", async () => {
    const { result, unmount } = renderSessionRead(() => useCumulativeDiff("session"), undefined);
    await advance();
    act(() => invalidateCumulativeDiffCache(result.current.store, "environment"));
    unmount();
    await advance(250);
    expect(request).toHaveBeenCalledTimes(1);
  });

  it("drains a mid-flight invalidation without a second trailing timer", async () => {
    const response = deferred<unknown>();
    request.mockReturnValueOnce(response.promise);
    const { result } = renderSessionRead(() => useCumulativeDiff("session"), undefined);
    act(() => invalidateCumulativeDiffCache(result.current.store, "environment"));
    await act(async () =>
      response.resolve({ cumulative_diff: { ...cached, head_commit: "obsolete" } }),
    );
    await advance(250);
    expect(request).toHaveBeenCalledTimes(2);
    expect(result.current.value.diff).toEqual(cached);
  });

  it("uses independent timers for unrelated environments", async () => {
    const { result } = renderSessionRead(
      () => [useCumulativeDiff("session"), useCumulativeDiff("other")],
      undefined,
    );
    await advance();
    act(() => {
      invalidateCumulativeDiffCache(result.current.store, "environment");
      invalidateCumulativeDiffCache(result.current.store, "other-environment");
    });
    await advance(250);
    expect(request).toHaveBeenCalledTimes(4);
  });

  it("preserves the last known diff on terminal response and later subscription", async () => {
    request
      .mockResolvedValueOnce({ cumulative_diff: cached })
      .mockResolvedValue({ ready: false, reason: "session_terminal", cumulative_diff: null });
    const { result, rerender } = renderSessionRead(
      (second: string | null) => [useCumulativeDiff("session"), useCumulativeDiff(second)],
      null as string | null,
    );
    await advance();
    act(() => invalidateCumulativeDiffCache(result.current.store, "environment"));
    await advance(250);
    rerender("session");
    await advance(5000);
    expect(result.current.value.map((v) => v.diff)).toEqual([cached, cached]);
    expect(request).toHaveBeenCalledTimes(2);
  });

  it("isolates caches in separate app stores", async () => {
    request
      .mockResolvedValueOnce({ cumulative_diff: cached })
      .mockResolvedValueOnce({ cumulative_diff: { ...cached, head_commit: "other-store" } });
    const first = renderSessionRead(() => useCumulativeDiff("session"), undefined);
    const second = renderSessionRead(() => useCumulativeDiff("session"), undefined);
    await advance();
    expect(first.result.current.value.diff?.head_commit).toBe("head");
    expect(second.result.current.value.diff?.head_commit).toBe("other-store");
  });
});
