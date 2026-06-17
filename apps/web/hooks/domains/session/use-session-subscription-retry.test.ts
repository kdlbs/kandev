import { act, cleanup, renderHook } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import {
  getUnknownSessionRetryDelay,
  shouldRetryUnknownSessionSubscription,
  useUnknownSessionSubscriptionRetry,
} from "./use-session-subscription-retry";

describe("shouldRetryUnknownSessionSubscription", () => {
  it("returns true only for a connected unknown session", () => {
    expect(
      shouldRetryUnknownSessionSubscription({
        taskSessionId: "sess-1",
        connectionStatus: "connected",
        taskSessionState: null,
      }),
    ).toBe(true);

    expect(
      shouldRetryUnknownSessionSubscription({
        taskSessionId: null,
        connectionStatus: "connected",
        taskSessionState: null,
      }),
    ).toBe(false);
    expect(
      shouldRetryUnknownSessionSubscription({
        taskSessionId: "sess-1",
        connectionStatus: "disconnected",
        taskSessionState: null,
      }),
    ).toBe(false);
    expect(
      shouldRetryUnknownSessionSubscription({
        taskSessionId: "sess-1",
        connectionStatus: "connected",
        taskSessionState: "RUNNING",
      }),
    ).toBe(false);
  });
});

describe("getUnknownSessionRetryDelay", () => {
  it("backs off retries up to a maximum delay", () => {
    expect(getUnknownSessionRetryDelay(0)).toBe(1000);
    expect(getUnknownSessionRetryDelay(1)).toBe(2000);
    expect(getUnknownSessionRetryDelay(5)).toBe(30000);
    expect(getUnknownSessionRetryDelay(20)).toBe(30000);
  });
});

describe("useUnknownSessionSubscriptionRetry", () => {
  afterEach(() => {
    cleanup();
    vi.useRealTimers();
  });

  it("advances the retry token on the backoff schedule while retrying", () => {
    vi.useFakeTimers();

    const { result } = renderHook(() =>
      useUnknownSessionSubscriptionRetry({
        taskSessionId: "sess-1",
        connectionStatus: "connected",
        taskSessionState: null,
      }),
    );

    expect(result.current).toBe(0);

    act(() => vi.advanceTimersByTime(1000));
    expect(result.current).toBe(1);

    act(() => vi.advanceTimersByTime(1999));
    expect(result.current).toBe(1);

    act(() => vi.advanceTimersByTime(1));
    expect(result.current).toBe(2);
  });

  it("resets the returned token when the session id changes", () => {
    vi.useFakeTimers();

    const { result, rerender } = renderHook(
      ({ sessionId }: { sessionId: string }) =>
        useUnknownSessionSubscriptionRetry({
          taskSessionId: sessionId,
          connectionStatus: "connected",
          taskSessionState: null,
        }),
      { initialProps: { sessionId: "sess-1" } },
    );

    act(() => vi.advanceTimersByTime(1000));
    expect(result.current).toBe(1);

    rerender({ sessionId: "sess-2" });
    expect(result.current).toBe(0);

    act(() => vi.advanceTimersByTime(1000));
    expect(result.current).toBe(1);
  });

  it("clears the scheduled retry when retrying stops", () => {
    vi.useFakeTimers();

    const { rerender } = renderHook(
      ({ taskSessionState }: { taskSessionState: null | "RUNNING" }) =>
        useUnknownSessionSubscriptionRetry({
          taskSessionId: "sess-1",
          connectionStatus: "connected",
          taskSessionState,
        }),
      { initialProps: { taskSessionState: null as null | "RUNNING" } },
    );

    expect(vi.getTimerCount()).toBe(1);

    rerender({ taskSessionState: "RUNNING" });

    expect(vi.getTimerCount()).toBe(0);
  });
});
