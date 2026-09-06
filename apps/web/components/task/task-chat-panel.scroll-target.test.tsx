/* eslint-disable max-lines -- related prompt-target lifecycle regressions share one focused harness. */
import { StrictMode } from "react";
import { act, renderHook, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { MessageListHandle } from "./chat/message-list-shared";
import type { LoadMessageWindowResult } from "@/hooks/domains/session/load-message-window";

const { mockDockviewState, mockAppStoreState, mockAppStoreApi } = vi.hoisted(() => {
  const state = {
    scrollTarget: null as null | {
      sessionId: string;
      messageId: string;
      token: number;
      hostPanelId: string;
    },
    clearScrollTarget: vi.fn((token: number) => {
      if (state.scrollTarget?.token === token) state.scrollTarget = null;
    }),
    clearScrollTargetForOwner: vi.fn((sessionId: string, hostPanelId: string) => {
      if (
        state.scrollTarget?.sessionId === sessionId &&
        state.scrollTarget.hostPanelId === hostPanelId
      ) {
        state.scrollTarget = null;
      }
    }),
    clearScrollTargetForSession: vi.fn((sessionId: string) => {
      if (state.scrollTarget?.sessionId === sessionId) state.scrollTarget = null;
    }),
  };
  const appStoreState = {
    messages: { bySession: { "session-1": [{ id: "message-1" }] } },
  };
  return {
    mockDockviewState: state,
    mockAppStoreState: appStoreState,
    mockAppStoreApi: {
      getState: () => ({ messages: appStoreState.messages, mergeMessages: vi.fn() }),
    },
  };
});

vi.mock("@/lib/state/dockview-store", () => ({
  useDockviewStore: Object.assign(
    (selector: (state: typeof mockDockviewState) => unknown) => selector(mockDockviewState),
    { getState: () => mockDockviewState },
  ),
}));

vi.mock("@/components/state-provider", () => ({
  useAppStore: vi.fn(),
  useAppStoreApi: () => mockAppStoreApi,
}));

vi.mock("@/hooks/domains/session/load-message-window", () => ({
  loadMessageWindowAround: vi.fn(),
}));

import {
  usePendingMessageScroll,
  useScrollTargetConsumption,
  type PendingMessageScrollTarget,
} from "./task-chat-panel";
import { loadMessageWindowAround } from "@/hooks/domains/session/load-message-window";

let pendingFrames: Array<(() => void) | undefined> = [];

/** Runs two rounds of pending requestAnimationFrame callbacks inside act. */
async function flushFrames() {
  await act(async () => {
    const frames = pendingFrames.splice(0);
    for (const frame of frames) frame?.();
  });
  await act(async () => {
    const frames = pendingFrames.splice(0);
    for (const frame of frames) frame?.();
  });
}

type ScrollTarget = NonNullable<typeof mockDockviewState.scrollTarget>;

/** Builds a dockview scroll-target object with defaults, merged with the given overrides. */
function target(overrides: Partial<ScrollTarget> = {}): ScrollTarget {
  return {
    sessionId: "session-1",
    messageId: "message-1",
    token: 7,
    hostPanelId: "panel-a",
    ...overrides,
  };
}
const DELETED_MESSAGE_ID = "deleted";
const DELETED_TARGET_RESULT: LoadMessageWindowResult = {
  kind: "deleted-target",
  merged: false,
  current: true,
  targetFound: false,
};

/** Builds a MessageListHandle whose scrollToMessage mock returns the given value. */
function scrollHandle(returns: boolean): MessageListHandle {
  return { scrollToMessage: vi.fn(() => returns) };
}

const PROPS = {
  resolvedSessionId: "session-1",
  isVisible: true,
  panelId: "panel-a",
  isInitialMessagesLoading: false,
  renderedMessageCount: 0,
};

beforeEach(() => {
  vi.clearAllMocks();
  vi.mocked(loadMessageWindowAround).mockReset();
  pendingFrames = [];
  vi.stubGlobal("requestAnimationFrame", (callback: () => void) => {
    pendingFrames.push(callback);
    return pendingFrames.length;
  });
  vi.stubGlobal("cancelAnimationFrame", (id: number) => {
    pendingFrames[id - 1] = undefined;
  });
  mockDockviewState.scrollTarget = null;
  mockAppStoreState.messages.bySession["session-1"] = [{ id: "message-1" }];
});

afterEach(() => {
  vi.unstubAllGlobals();
  vi.useRealTimers();
});

afterEach(() => {
  act(() => {});
});

describe("useScrollTargetConsumption — success-path consumption", () => {
  it("scrolls the target row and clears by token on a successful scroll", async () => {
    mockDockviewState.scrollTarget = target();
    const messageListRef = { current: scrollHandle(true) };

    renderHook(() => useScrollTargetConsumption({ ...PROPS, messageListRef }));
    await flushFrames();

    expect(messageListRef.current?.scrollToMessage).toHaveBeenCalledWith("message-1", {
      align: "start",
    });
    expect(mockDockviewState.clearScrollTarget).toHaveBeenCalledWith(7);
    expect(mockDockviewState.scrollTarget).toBeNull();
  });

  it("does not consume when another panel owns the target (exact-owner gate)", () => {
    mockDockviewState.scrollTarget = target({ hostPanelId: "panel-b" });
    const messageListRef = { current: scrollHandle(true) };

    renderHook(() => useScrollTargetConsumption({ ...PROPS, messageListRef }));

    expect(messageListRef.current?.scrollToMessage).not.toHaveBeenCalled();
    expect(mockDockviewState.clearScrollTarget).not.toHaveBeenCalled();
  });

  it("does not let a delayed consumer of request A clear or scroll request B", async () => {
    // Task 03 supersession: B lands while A's deferred consumer is pending.
    // A's stale consumer must not scroll A's row nor clear B; the token
    // re-check in the deferred callback bails.
    mockDockviewState.scrollTarget = target(); // A: token 7
    const messageListRef = { current: scrollHandle(true) };
    const { rerender } = renderHook(() => useScrollTargetConsumption({ ...PROPS, messageListRef }));

    // B supersedes A before A's deferred frame executes.
    mockDockviewState.scrollTarget = target({ messageId: "message-b", token: 8 });
    await flushFrames();

    expect(messageListRef.current?.scrollToMessage).not.toHaveBeenCalled();
    expect(mockDockviewState.clearScrollTarget).not.toHaveBeenCalled();
    expect(mockDockviewState.scrollTarget?.token).toBe(8);

    // When the owner of B re-renders, B consumes normally.
    rerender();
    await flushFrames();
    expect(messageListRef.current?.scrollToMessage).toHaveBeenCalledWith("message-b", {
      align: "start",
    });
    expect(mockDockviewState.clearScrollTarget).toHaveBeenCalledWith(8);
    expect(mockDockviewState.scrollTarget).toBeNull();
  });

  it("lets only the exact-owner host consume when a canonical chat and a session panel are both mounted and active", async () => {
    // Task 03 race: `session:<id>` and the canonical `chat` host are mounted
    // simultaneously (a real transient layout state), both reporting
    // isVisible=true. Only the host whose panelId equals the target's
    // hostPanelId may scroll and clear; the hidden canonical duplicate must
    // never consume the target meant for the visible transcript.
    mockDockviewState.scrollTarget = target();
    const sessionHostRef = { current: scrollHandle(true) };
    const canonicalHostRef = { current: scrollHandle(true) };

    renderHook(() => useScrollTargetConsumption({ ...PROPS, messageListRef: sessionHostRef }));
    renderHook(() =>
      useScrollTargetConsumption({
        ...PROPS,
        panelId: "chat",
        messageListRef: canonicalHostRef,
      }),
    );
    await flushFrames();

    expect(sessionHostRef.current?.scrollToMessage).toHaveBeenCalledWith("message-1", {
      align: "start",
    });
    expect(mockDockviewState.clearScrollTarget).toHaveBeenCalledWith(7);
    expect(canonicalHostRef.current?.scrollToMessage).not.toHaveBeenCalled();
    expect(mockDockviewState.clearScrollTargetForSession).not.toHaveBeenCalled();
  });

  it("does not consume when the target belongs to another session", () => {
    mockDockviewState.scrollTarget = target({ sessionId: "session-2" });
    const messageListRef = { current: scrollHandle(true) };

    renderHook(() => useScrollTargetConsumption({ ...PROPS, messageListRef }));

    expect(messageListRef.current?.scrollToMessage).not.toHaveBeenCalled();
    expect(mockDockviewState.clearScrollTarget).not.toHaveBeenCalled();
  });
});

// eslint-disable-next-line max-lines-per-function -- pending-target request and timer regressions share one fixture.
describe("usePendingMessageScroll — non-Dockview target loading", () => {
  it("loads an absent target and reasserts it after the transcript merge", async () => {
    vi.useFakeTimers({ toFake: ["setTimeout", "clearTimeout"] });
    const targetMessage = { id: "target" };
    vi.mocked(loadMessageWindowAround).mockImplementationOnce(async () => {
      mockAppStoreState.messages.bySession["session-1"] = [targetMessage];
      return { kind: "merged", merged: true, current: true, targetFound: true };
    });
    const messageListRef = { current: scrollHandle(true) };
    const onConsumed = vi.fn();
    const { rerender } = renderHook(
      ({ readinessKey }) =>
        usePendingMessageScroll({
          messageListRef,
          sessionId: "session-1",
          messageId: "target",
          onConsumed,
          readinessKey,
          isInitialMessagesLoading: false,
        }),
      { initialProps: { readinessKey: "0" } },
    );

    messageListRef.current = scrollHandle(false);
    await flushFrames();
    expect(loadMessageWindowAround).toHaveBeenCalledWith(
      "session-1",
      "target",
      expect.any(Function),
      expect.anything(),
    );

    messageListRef.current = scrollHandle(true);
    rerender({ readinessKey: "1" });
    await flushFrames();
    expect(messageListRef.current?.scrollToMessage).toHaveBeenCalledWith("target", {
      align: "start",
      behavior: "auto",
    });
    expect(onConsumed).not.toHaveBeenCalled();

    await act(async () => {
      await vi.advanceTimersByTimeAsync(250);
    });
    expect(messageListRef.current?.scrollToMessage).toHaveBeenCalledTimes(2);
    expect(onConsumed).toHaveBeenCalledWith("target");
    vi.useRealTimers();
  });
  it("consumes a deleted mobile target when around loading settles before scroll", async () => {
    const pending = Promise.withResolvers<LoadMessageWindowResult>();
    vi.mocked(loadMessageWindowAround).mockReturnValue(pending.promise);
    mockAppStoreState.messages.bySession["session-1"] = [];
    const messageListRef = { current: scrollHandle(false) };
    const onConsumed = vi.fn();
    const { rerender } = renderHook(
      ({ readinessKey }) =>
        usePendingMessageScroll({
          messageListRef,
          sessionId: "session-1",
          messageId: DELETED_MESSAGE_ID,
          onConsumed,
          readinessKey,
          isInitialMessagesLoading: false,
        }),
      { initialProps: { readinessKey: "a" } },
    );

    await flushFrames();
    pending.resolve(DELETED_TARGET_RESULT);
    await act(async () => {
      await Promise.resolve();
    });
    messageListRef.current = scrollHandle(true);
    rerender({ readinessKey: "b" });
    await flushFrames();

    expect(messageListRef.current?.scrollToMessage).toHaveBeenCalledTimes(1);
    expect(onConsumed).toHaveBeenCalledWith(DELETED_MESSAGE_ID);
  });

  it("handles mobile around results that settle after the first scroll", async () => {
    const pending = Promise.withResolvers<LoadMessageWindowResult>();
    vi.mocked(loadMessageWindowAround).mockReturnValue(pending.promise);
    mockAppStoreState.messages.bySession["session-1"] = [];
    const scrollToMessage = vi.fn().mockReturnValueOnce(false).mockReturnValue(true);
    const messageListRef = { current: { scrollToMessage } };
    const onConsumed = vi.fn();
    vi.useFakeTimers({ toFake: ["setTimeout", "clearTimeout"] });
    const { rerender } = renderHook(
      ({ readinessKey }) =>
        usePendingMessageScroll({
          messageListRef,
          sessionId: "session-1",
          messageId: "target",
          onConsumed,
          readinessKey,
          isInitialMessagesLoading: false,
        }),
      { initialProps: { readinessKey: "a" } },
    );

    await flushFrames();
    messageListRef.current = { scrollToMessage };
    scrollToMessage.mockReturnValue(true);
    rerender({ readinessKey: "b" });
    await flushFrames();
    expect(scrollToMessage).toHaveBeenCalledTimes(2);

    pending.resolve({ kind: "merged", merged: true, current: true, targetFound: true });
    await act(async () => {
      await Promise.resolve();
    });
    await act(async () => {
      await vi.advanceTimersByTimeAsync(250);
    });

    expect(scrollToMessage).toHaveBeenCalledTimes(3);
    expect(onConsumed).toHaveBeenCalledWith("target");
    vi.useRealTimers();
  });

  it("consumes a deleted mobile target after the first scroll", async () => {
    const pending = Promise.withResolvers<LoadMessageWindowResult>();
    vi.mocked(loadMessageWindowAround).mockReturnValue(pending.promise);
    mockAppStoreState.messages.bySession["session-1"] = [];
    const scrollToMessage = vi.fn().mockReturnValueOnce(false).mockReturnValue(true);
    const messageListRef = { current: { scrollToMessage } };
    const onConsumed = vi.fn();
    const { rerender } = renderHook(
      ({ readinessKey }) =>
        usePendingMessageScroll({
          messageListRef,
          sessionId: "session-1",
          messageId: DELETED_MESSAGE_ID,
          onConsumed,
          readinessKey,
          isInitialMessagesLoading: false,
        }),
      { initialProps: { readinessKey: "a" } },
    );

    await flushFrames();
    messageListRef.current = { scrollToMessage };
    scrollToMessage.mockReturnValue(true);
    rerender({ readinessKey: "b" });
    await flushFrames();
    pending.resolve(DELETED_TARGET_RESULT);
    await act(async () => {
      await Promise.resolve();
    });

    expect(scrollToMessage).toHaveBeenCalledTimes(2);
    expect(onConsumed).toHaveBeenCalledWith(DELETED_MESSAGE_ID);
  });

  it("ignores an obsolete mobile around-window settlement after target replacement", async () => {
    const first = Promise.withResolvers<LoadMessageWindowResult>();
    const second = Promise.withResolvers<LoadMessageWindowResult>();
    vi.mocked(loadMessageWindowAround)
      .mockReturnValueOnce(first.promise)
      .mockReturnValueOnce(second.promise);
    mockAppStoreState.messages.bySession["session-1"] = [];
    const targetA = {
      sessionId: "session-1",
      messageId: "message-a",
      token: 1,
      hostPanelId: "mobile-chat",
    };
    const targetB = { ...targetA, messageId: "message-b", token: 2 };
    const messageListRef = { current: scrollHandle(false) };
    const onConsumed = vi.fn();
    const { rerender } = renderHook(
      ({ target, readinessKey }) =>
        usePendingMessageScroll({
          messageListRef,
          sessionId: "session-1",
          messageId: null,
          target,
          onConsumed,
          readinessKey,
          isInitialMessagesLoading: false,
        }),
      { initialProps: { target: targetA, readinessKey: "a" } },
    );

    await flushFrames();
    rerender({ target: targetB, readinessKey: "b" });
    await flushFrames();
    expect(loadMessageWindowAround).toHaveBeenCalledTimes(2);

    first.resolve({ kind: "merged", merged: true, current: true, targetFound: true });
    await act(async () => {
      await Promise.resolve();
    });
    expect(onConsumed).not.toHaveBeenCalled();

    second.resolve(DELETED_TARGET_RESULT);
    await act(async () => {
      await Promise.resolve();
    });
    expect(onConsumed).toHaveBeenCalledWith("message-b");
  });

  it("consumes a mobile target after a failed delayed reassertion", async () => {
    vi.useFakeTimers({ toFake: ["setTimeout", "clearTimeout"] });
    vi.mocked(loadMessageWindowAround).mockImplementationOnce(async () => {
      mockAppStoreState.messages.bySession["session-1"] = [{ id: "message-a" }];
      return { kind: "merged", merged: true, current: true, targetFound: true };
    });
    mockAppStoreState.messages.bySession["session-1"] = [];
    const scrollToMessage = vi
      .fn()
      .mockReturnValueOnce(false)
      .mockReturnValueOnce(true)
      .mockReturnValue(false);
    const messageListRef = { current: { scrollToMessage } };
    const onConsumed = vi.fn();
    const { rerender } = renderHook(
      ({ readinessKey, messageId }) =>
        usePendingMessageScroll({
          messageListRef,
          sessionId: "session-1",
          messageId,
          onConsumed,
          readinessKey,
          isInitialMessagesLoading: false,
        }),
      { initialProps: { readinessKey: "a", messageId: "message-a" as string | null } },
    );

    await flushFrames();
    await act(async () => {
      await Promise.resolve();
    });
    rerender({ readinessKey: "b", messageId: "message-a" });
    await flushFrames();
    expect(scrollToMessage).toHaveBeenCalledTimes(2);

    await act(async () => {
      await vi.advanceTimersByTimeAsync(250);
    });
    expect(scrollToMessage).toHaveBeenCalledTimes(3);
    expect(onConsumed).toHaveBeenCalledWith("message-a");

    rerender({ readinessKey: "c", messageId: null });
    await flushFrames();
    expect(scrollToMessage).toHaveBeenCalledTimes(3);
  });
  it("consumes an already loaded target in one pass", async () => {
    vi.useFakeTimers({ toFake: ["setTimeout", "clearTimeout"] });
    const messageListRef = { current: scrollHandle(true) };
    const onConsumed = vi.fn();

    renderHook(() =>
      usePendingMessageScroll({
        messageListRef,
        sessionId: "session-1",
        messageId: "message-1",
        onConsumed,
        readinessKey: "0",
        isInitialMessagesLoading: false,
      }),
    );
    await flushFrames();

    expect(messageListRef.current?.scrollToMessage).toHaveBeenCalledTimes(1);
    expect(onConsumed).toHaveBeenCalledWith("message-1");
    await act(async () => {
      await vi.advanceTimersByTimeAsync(250);
    });
    expect(messageListRef.current?.scrollToMessage).toHaveBeenCalledTimes(1);
  });
  it("remains active after StrictMode effect replay", async () => {
    const messageListRef = { current: scrollHandle(true) };
    const onConsumed = vi.fn();
    renderHook(
      () =>
        usePendingMessageScroll({
          messageListRef,
          sessionId: "session-1",
          messageId: "message-1",
          onConsumed,
          readinessKey: "0",
          isInitialMessagesLoading: false,
        }),
      { wrapper: StrictMode },
    );
    await flushFrames();
    expect(messageListRef.current?.scrollToMessage).toHaveBeenCalledTimes(1);
    expect(onConsumed).toHaveBeenCalledWith("message-1");
  });

  it("does not scroll or load while a pending host is hidden", async () => {
    const messageListRef = { current: scrollHandle(false) };
    const onConsumed = vi.fn();
    const { rerender } = renderHook(
      ({ isVisible }) =>
        usePendingMessageScroll({
          messageListRef,
          sessionId: "session-1",
          messageId: "missing",
          onConsumed,
          readinessKey: "0",
          isInitialMessagesLoading: false,
          isVisible,
        }),
      { initialProps: { isVisible: false } },
    );

    await flushFrames();
    expect(messageListRef.current?.scrollToMessage).not.toHaveBeenCalled();
    expect(loadMessageWindowAround).not.toHaveBeenCalled();

    messageListRef.current = scrollHandle(true);
    rerender({ isVisible: true });
    await flushFrames();
    expect(messageListRef.current?.scrollToMessage).toHaveBeenCalledWith("missing", {
      align: "start",
      behavior: "auto",
    });
  });
  it("defers a pending around request until initial transcript loading settles", async () => {
    mockAppStoreState.messages.bySession["session-1"] = [];
    vi.mocked(loadMessageWindowAround).mockReturnValue(new Promise(() => {}));
    const messageListRef = { current: scrollHandle(false) };
    const { rerender } = renderHook(
      ({ isInitialMessagesLoading }) =>
        usePendingMessageScroll({
          messageListRef,
          sessionId: "session-1",
          messageId: "missing",
          readinessKey: "0",
          onConsumed: vi.fn(),
          isInitialMessagesLoading,
        }),
      { initialProps: { isInitialMessagesLoading: true } },
    );

    await flushFrames();
    expect(loadMessageWindowAround).not.toHaveBeenCalled();

    rerender({ isInitialMessagesLoading: false });
    await flushFrames();
    expect(loadMessageWindowAround).toHaveBeenCalledTimes(1);
  });
  it("rejects a target owned by another host session", async () => {
    const messageListRef = { current: scrollHandle(false) };
    const onConsumed = vi.fn();
    const target: PendingMessageScrollTarget = {
      sessionId: "session-a",
      messageId: "message-a",
      token: 3,
      hostPanelId: "mobile-chat",
    };

    renderHook(() =>
      usePendingMessageScroll({
        messageListRef,
        sessionId: "session-b",
        messageId: null,
        target,
        onConsumed,
        readinessKey: "0",
        isInitialMessagesLoading: false,
      }),
    );
    await flushFrames();

    expect(messageListRef.current?.scrollToMessage).not.toHaveBeenCalled();
    expect(loadMessageWindowAround).not.toHaveBeenCalled();
    expect(onConsumed).toHaveBeenCalledWith("message-a");
  });

  it("cancels a failed around request without leaving a stuck mobile target", async () => {
    vi.mocked(loadMessageWindowAround).mockRejectedValueOnce(new Error("offline"));
    const messageListRef = { current: scrollHandle(false) };
    const onConsumed = vi.fn();
    renderHook(() =>
      usePendingMessageScroll({
        messageListRef,
        sessionId: "session-1",
        messageId: "target",
        onConsumed,
        readinessKey: "0",
        isInitialMessagesLoading: false,
      }),
    );

    await flushFrames();
    await waitFor(() => expect(onConsumed).toHaveBeenCalledWith("target"));
    expect(loadMessageWindowAround).toHaveBeenCalledTimes(1);
  });

  it("keeps the request guard valid when transcript readiness changes mid-request", async () => {
    const pending = Promise.withResolvers<LoadMessageWindowResult>();
    vi.mocked(loadMessageWindowAround).mockReturnValueOnce(pending.promise);
    const messageListRef = { current: scrollHandle(false) };
    const onConsumed = vi.fn();
    const { rerender } = renderHook(
      ({ readinessKey }) =>
        usePendingMessageScroll({
          messageListRef,
          sessionId: "session-1",
          messageId: "target",
          onConsumed,
          readinessKey,
          isInitialMessagesLoading: false,
        }),
      { initialProps: { readinessKey: "0" } },
    );

    await flushFrames();
    rerender({ readinessKey: "1" });
    await flushFrames();
    expect(loadMessageWindowAround).toHaveBeenCalledTimes(1);

    pending.reject(new Error("offline"));
    await waitFor(() => expect(onConsumed).toHaveBeenCalledWith("target"));
  });
});

describe("useScrollTargetConsumption — non-dockview hosts (no panelId)", () => {
  it("does not consume without a panelId (non-dockview hosts stay unchanged)", () => {
    mockDockviewState.scrollTarget = target();
    const messageListRef = { current: scrollHandle(true) };

    renderHook(() => useScrollTargetConsumption({ ...PROPS, panelId: null, messageListRef }));

    expect(messageListRef.current?.scrollToMessage).not.toHaveBeenCalled();
  });

  it("does not clear a dockview target when a non-dockview host unmounts", () => {
    // Task 03: preview/center/mobile hosts (panelId=null) mount and unmount
    // freely; their unmount cleanup must not clear a target a dockview host
    // is waiting to consume.
    mockDockviewState.scrollTarget = target();
    const messageListRef = { current: scrollHandle(false) };

    const { unmount } = renderHook(() =>
      useScrollTargetConsumption({ ...PROPS, panelId: null, messageListRef }),
    );
    unmount();

    expect(mockDockviewState.clearScrollTargetForSession).not.toHaveBeenCalled();
    expect(mockDockviewState.scrollTarget).not.toBeNull();
  });

  it("does not clear a dockview target when a non-dockview host swaps sessions", () => {
    mockDockviewState.scrollTarget = target();
    const messageListRef = { current: scrollHandle(false) };

    const { rerender } = renderHook(
      ({ resolvedSessionId }) =>
        useScrollTargetConsumption({
          ...PROPS,
          panelId: null,
          resolvedSessionId,
          messageListRef,
        }),
      { initialProps: { resolvedSessionId: "session-1" } },
    );
    rerender({ resolvedSessionId: "session-2" });

    expect(mockDockviewState.clearScrollTargetForSession).not.toHaveBeenCalled();
    expect(mockDockviewState.scrollTarget).not.toBeNull();
  });

  it("lets the dockview owner consume the retained target after non-dockview churn", async () => {
    mockDockviewState.scrollTarget = target();
    const nonDockviewRef = { current: scrollHandle(false) };
    const ownerRef = { current: scrollHandle(true) };

    const { unmount } = renderHook(() =>
      useScrollTargetConsumption({ ...PROPS, panelId: null, messageListRef: nonDockviewRef }),
    );
    unmount();

    renderHook(() => useScrollTargetConsumption({ ...PROPS, messageListRef: ownerRef }));
    await flushFrames();

    expect(ownerRef.current?.scrollToMessage).toHaveBeenCalledWith("message-1", {
      align: "start",
    });
    expect(mockDockviewState.clearScrollTarget).toHaveBeenCalledWith(7);
    expect(mockDockviewState.scrollTarget).toBeNull();
  });
});

it("reasserts a Dockview target when merged loading settles before scroll", async () => {
  vi.useFakeTimers({ toFake: ["setTimeout", "clearTimeout"] });
  mockAppStoreState.messages.bySession["session-1"] = [];
  const pending = Promise.withResolvers<LoadMessageWindowResult>();
  vi.mocked(loadMessageWindowAround).mockReturnValue(pending.promise);
  mockDockviewState.scrollTarget = target({ messageId: "target" });
  const scrollToMessage = vi.fn().mockReturnValue(false);
  const messageListRef = { current: { scrollToMessage } };
  const { rerender } = renderHook(
    ({ renderedMessageCount }) =>
      useScrollTargetConsumption({
        ...PROPS,
        renderedMessageCount,
        messageListRef,
      }),
    { initialProps: { renderedMessageCount: 0 } },
  );

  await flushFrames();
  pending.resolve({ kind: "merged", merged: true, current: true, targetFound: true });
  await act(async () => {
    await Promise.resolve();
  });
  scrollToMessage.mockReturnValue(true);
  rerender({ renderedMessageCount: 1 });
  await flushFrames();
  await act(async () => {
    await vi.advanceTimersByTimeAsync(250);
  });

  expect(scrollToMessage).toHaveBeenCalledTimes(3);
  expect(mockDockviewState.clearScrollTarget).toHaveBeenCalledWith(7);
  vi.useRealTimers();
});
it("loads a target that is cached but not rendered in the transcript", async () => {
  const pending = new Promise<LoadMessageWindowResult>(() => {});
  vi.mocked(loadMessageWindowAround).mockReturnValue(pending);
  mockDockviewState.scrollTarget = target({ messageId: "message-1" });
  const messageListRef = { current: scrollHandle(false) };

  renderHook(() =>
    useScrollTargetConsumption({
      ...PROPS,
      messageListRef,
    }),
  );
  await flushFrames();

  expect(loadMessageWindowAround).toHaveBeenCalledWith(
    "session-1",
    "message-1",
    expect.any(Function),
    expect.anything(),
  );
});
it("defers an unloaded target while initial loading is active despite a populated cache", async () => {
  const pending = new Promise<LoadMessageWindowResult>(() => {});
  vi.mocked(loadMessageWindowAround).mockReturnValue(pending);
  mockAppStoreState.messages.bySession["session-1"] = [{ id: "newest" }];
  mockDockviewState.scrollTarget = target({ messageId: "older-target" });
  const messageListRef = { current: scrollHandle(false) };
  const { rerender } = renderHook(
    ({ isInitialMessagesLoading }) =>
      useScrollTargetConsumption({
        ...PROPS,
        isInitialMessagesLoading,
        messageListRef,
      }),
    { initialProps: { isInitialMessagesLoading: true } },
  );
  await flushFrames();
  expect(loadMessageWindowAround).not.toHaveBeenCalled();

  rerender({ isInitialMessagesLoading: false });
  await flushFrames();
  expect(loadMessageWindowAround).toHaveBeenCalledTimes(1);
});

it("clears a deleted Dockview target when loading settles before scroll", async () => {
  mockAppStoreState.messages.bySession["session-1"] = [];
  const pending = Promise.withResolvers<LoadMessageWindowResult>();
  vi.mocked(loadMessageWindowAround).mockReturnValue(pending.promise);
  mockDockviewState.scrollTarget = target({ messageId: DELETED_MESSAGE_ID });
  const messageListRef = { current: scrollHandle(false) };
  renderHook(() => useScrollTargetConsumption({ ...PROPS, messageListRef }));

  await flushFrames();
  pending.resolve(DELETED_TARGET_RESULT);
  await act(async () => {
    await Promise.resolve();
  });

  expect(mockDockviewState.clearScrollTarget).toHaveBeenCalledWith(7);
  expect(messageListRef.current?.scrollToMessage).toHaveBeenCalledTimes(1);
});

it("reasserts a Dockview target when merged loading settles after scroll", async () => {
  vi.useFakeTimers({ toFake: ["setTimeout", "clearTimeout"] });
  mockAppStoreState.messages.bySession["session-1"] = [];
  const pending = Promise.withResolvers<LoadMessageWindowResult>();
  vi.mocked(loadMessageWindowAround).mockReturnValue(pending.promise);
  mockDockviewState.scrollTarget = target({ messageId: "target" });
  const scrollToMessage = vi.fn().mockReturnValueOnce(false).mockReturnValue(true);
  const messageListRef = { current: { scrollToMessage } };
  const { rerender } = renderHook(
    ({ renderedMessageCount }) =>
      useScrollTargetConsumption({
        ...PROPS,
        renderedMessageCount,
        messageListRef,
      }),
    { initialProps: { renderedMessageCount: 0 } },
  );

  await flushFrames();
  scrollToMessage.mockReturnValue(true);
  rerender({ renderedMessageCount: 1 });
  await flushFrames();
  expect(scrollToMessage).toHaveBeenCalledTimes(2);
  pending.resolve({ kind: "merged", merged: true, current: true, targetFound: true });
  await act(async () => {
    await Promise.resolve();
  });
  await act(async () => {
    await vi.advanceTimersByTimeAsync(250);
  });

  expect(scrollToMessage).toHaveBeenCalledTimes(3);
  expect(mockDockviewState.clearScrollTarget).toHaveBeenCalledWith(7);
  vi.useRealTimers();
});

it("clears a deleted Dockview target after the first scroll", async () => {
  mockAppStoreState.messages.bySession["session-1"] = [];
  const pending = Promise.withResolvers<LoadMessageWindowResult>();
  vi.mocked(loadMessageWindowAround).mockReturnValue(pending.promise);
  mockDockviewState.scrollTarget = target({ messageId: DELETED_MESSAGE_ID });
  const scrollToMessage = vi.fn().mockReturnValueOnce(false).mockReturnValue(true);
  const messageListRef = { current: { scrollToMessage } };
  const { rerender } = renderHook(
    ({ renderedMessageCount }) =>
      useScrollTargetConsumption({
        ...PROPS,
        renderedMessageCount,
        messageListRef,
      }),
    { initialProps: { renderedMessageCount: 0 } },
  );

  await flushFrames();
  scrollToMessage.mockReturnValue(true);
  rerender({ renderedMessageCount: 1 });
  await flushFrames();
  pending.resolve(DELETED_TARGET_RESULT);
  await act(async () => {
    await Promise.resolve();
  });

  expect(scrollToMessage).toHaveBeenCalledTimes(2);
  expect(mockDockviewState.clearScrollTarget).toHaveBeenCalledWith(7);
});
it("defers an absent around request until initial transcript loading settles", async () => {
  mockAppStoreState.messages.bySession["session-1"] = [];
  mockDockviewState.scrollTarget = target();
  vi.mocked(loadMessageWindowAround).mockReturnValue(new Promise(() => {}));
  const messageListRef = { current: scrollHandle(false) };
  const { rerender } = renderHook(
    ({ isInitialMessagesLoading }) =>
      useScrollTargetConsumption({
        ...PROPS,
        isInitialMessagesLoading,
        messageListRef,
      }),
    { initialProps: { isInitialMessagesLoading: true } },
  );

  await flushFrames();
  expect(messageListRef.current?.scrollToMessage).toHaveBeenCalledTimes(1);
  expect(loadMessageWindowAround).not.toHaveBeenCalled();

  rerender({ isInitialMessagesLoading: false });
  await flushFrames();
  expect(loadMessageWindowAround).toHaveBeenCalledTimes(1);
});

it("does not duplicate an in-flight around request across same-token revisions", async () => {
  mockAppStoreState.messages.bySession["session-1"] = [];
  mockDockviewState.scrollTarget = target();
  const pending = Promise.withResolvers<LoadMessageWindowResult>();
  vi.mocked(loadMessageWindowAround).mockReturnValue(pending.promise);
  const messageListRef = { current: scrollHandle(false) };
  const { rerender } = renderHook(
    ({ renderedMessageCount }) =>
      useScrollTargetConsumption({
        ...PROPS,
        renderedMessageCount,
        messageListRef,
      }),
    { initialProps: { renderedMessageCount: 0 } },
  );

  await flushFrames();
  rerender({ renderedMessageCount: 1 });
  await flushFrames();
  rerender({ renderedMessageCount: 2 });
  await flushFrames();

  expect(loadMessageWindowAround).toHaveBeenCalledTimes(1);
  pending.resolve(DELETED_TARGET_RESULT);
  await act(async () => {
    await Promise.resolve();
  });
});

it("settles stale request loading independently from the replacement target", async () => {
  mockAppStoreState.messages.bySession["session-1"] = [];
  const pendingA = Promise.withResolvers<LoadMessageWindowResult>();
  const pendingB = Promise.withResolvers<LoadMessageWindowResult>();
  vi.mocked(loadMessageWindowAround).mockImplementation((_sessionId, messageId) =>
    messageId === "message-a" ? pendingA.promise : pendingB.promise,
  );
  mockDockviewState.scrollTarget = target({ messageId: "message-a", token: 7 });
  const messageListRef = { current: scrollHandle(false) };
  const { result, rerender } = renderHook(
    ({ renderedMessageCount }) =>
      useScrollTargetConsumption({
        ...PROPS,
        renderedMessageCount,
        messageListRef,
      }),
    { initialProps: { renderedMessageCount: 0 } },
  );

  await flushFrames();
  mockDockviewState.scrollTarget = target({ messageId: "message-b", token: 8 });
  rerender({ renderedMessageCount: 1 });
  await flushFrames();
  expect(loadMessageWindowAround).toHaveBeenCalledTimes(2);
  expect(result.current).toBe(true);

  pendingA.resolve({ kind: "merged", merged: true, current: true, targetFound: true });
  await act(async () => {
    await Promise.resolve();
    await Promise.resolve();
  });
  expect(result.current).toBe(true);
  expect(messageListRef.current?.scrollToMessage).toHaveBeenCalledTimes(2);
  expect(mockDockviewState.clearScrollTarget).not.toHaveBeenCalled();
  expect(mockDockviewState.scrollTarget?.messageId).toBe("message-b");

  pendingB.resolve({ kind: "merged", merged: true, current: true, targetFound: true });
  await waitFor(() => expect(result.current).toBe(false));
});
it("ignores a stale Dockview around settlement without clearing the active target", async () => {
  mockAppStoreState.messages.bySession["session-1"] = [];
  const pending = Promise.withResolvers<LoadMessageWindowResult>();
  vi.mocked(loadMessageWindowAround).mockReturnValue(pending.promise);
  mockDockviewState.scrollTarget = target();
  const messageListRef = { current: scrollHandle(false) };
  renderHook(() => useScrollTargetConsumption({ ...PROPS, messageListRef }));

  await flushFrames();
  pending.resolve({ kind: "stale", merged: false, current: false, targetFound: false });
  await act(async () => {
    await Promise.resolve();
  });

  expect(mockDockviewState.clearScrollTarget).not.toHaveBeenCalled();
  expect(mockDockviewState.scrollTarget).not.toBeNull();
});

it("cancels the delayed pass when the Dockview host becomes hidden", async () => {
  vi.useFakeTimers({ toFake: ["setTimeout", "clearTimeout"] });
  mockAppStoreState.messages.bySession["session-1"] = [];
  vi.mocked(loadMessageWindowAround).mockImplementationOnce(async () => {
    mockAppStoreState.messages.bySession["session-1"] = [{ id: "message-1" }];
    return { kind: "merged", merged: true, current: true, targetFound: true };
  });
  mockDockviewState.scrollTarget = target();
  const messageListRef = { current: scrollHandle(false) };
  const { rerender } = renderHook(
    ({ isVisible, renderedMessageCount }) =>
      useScrollTargetConsumption({
        ...PROPS,
        isVisible,
        renderedMessageCount,
        messageListRef,
      }),
    { initialProps: { isVisible: true, renderedMessageCount: 0 } },
  );

  await flushFrames();
  await act(async () => {
    await Promise.resolve();
  });
  messageListRef.current = scrollHandle(true);
  rerender({ isVisible: true, renderedMessageCount: 1 });
  await flushFrames();
  expect(messageListRef.current?.scrollToMessage).toHaveBeenCalledTimes(1);

  rerender({ isVisible: false, renderedMessageCount: 1 });
  await act(async () => {
    await vi.advanceTimersByTimeAsync(250);
  });
  expect(messageListRef.current?.scrollToMessage).toHaveBeenCalledTimes(1);
  expect(mockDockviewState.clearScrollTarget).toHaveBeenCalledWith(7);
  expect(mockDockviewState.scrollTarget).toBeNull();
});

it("does not consume a mobile target after leaving and returning to Chat", async () => {
  vi.useFakeTimers({ toFake: ["setTimeout", "clearTimeout"] });
  mockAppStoreState.messages.bySession["session-1"] = [];
  vi.mocked(loadMessageWindowAround).mockImplementationOnce(async () => {
    mockAppStoreState.messages.bySession["session-1"] = [{ id: "mobile-target" }];
    return { kind: "merged", merged: true, current: true, targetFound: true };
  });
  const mobileTarget = {
    sessionId: "session-1",
    messageId: "mobile-target",
    token: 11,
    hostPanelId: "mobile-chat",
  };
  const messageListRef = { current: scrollHandle(false) };
  const onConsumed = vi.fn();
  const { rerender } = renderHook(
    ({ target, isVisible, readinessKey }) =>
      usePendingMessageScroll({
        messageListRef,
        sessionId: "session-1",
        messageId: null,
        target,
        onConsumed,
        readinessKey,
        isInitialMessagesLoading: false,
        isVisible,
      }),
    {
      initialProps: {
        target: mobileTarget as PendingMessageScrollTarget | null,
        isVisible: true,
        readinessKey: "0",
      },
    },
  );

  await flushFrames();
  await act(async () => {
    await Promise.resolve();
  });
  messageListRef.current = scrollHandle(true);
  rerender({ target: mobileTarget, isVisible: true, readinessKey: "1" });
  await flushFrames();
  expect(messageListRef.current?.scrollToMessage).toHaveBeenCalledTimes(1);

  rerender({ target: null, isVisible: false, readinessKey: "2" });
  await act(async () => {
    await vi.advanceTimersByTimeAsync(250);
  });
  rerender({ target: null, isVisible: true, readinessKey: "3" });
  await flushFrames();
  expect(messageListRef.current?.scrollToMessage).toHaveBeenCalledTimes(1);
  expect(onConsumed).not.toHaveBeenCalled();
});

describe("useScrollTargetConsumption — delayed-DOM retry", () => {
  it("retains the target when the scroll no-ops and consumes once the list is ready", async () => {
    mockDockviewState.scrollTarget = target();
    const messageListRef = { current: scrollHandle(false) };

    const { rerender } = renderHook(
      ({ isInitialMessagesLoading }) =>
        useScrollTargetConsumption({
          ...PROPS,
          isInitialMessagesLoading,
          messageListRef,
        }),
      { initialProps: { isInitialMessagesLoading: true } },
    );

    await flushFrames();
    expect(messageListRef.current?.scrollToMessage).toHaveBeenCalled();
    expect(mockDockviewState.clearScrollTarget).not.toHaveBeenCalled();
    expect(mockDockviewState.scrollTarget).not.toBeNull();

    // The message list renders the row; the readiness flip retries.
    messageListRef.current = scrollHandle(true);
    rerender({ isInitialMessagesLoading: false });
    await flushFrames();

    expect(mockDockviewState.clearScrollTarget).toHaveBeenCalledWith(7);
    expect(mockDockviewState.scrollTarget).toBeNull();
  });

  it("retries when rendered message count changes after loading is complete", async () => {
    mockDockviewState.scrollTarget = target();
    const messageListRef = { current: scrollHandle(false) };
    const { rerender } = renderHook(
      ({ renderedMessageCount }) =>
        useScrollTargetConsumption({
          ...PROPS,
          renderedMessageCount,
          targetRendered: true,
          messageListRef,
        }),
      { initialProps: { renderedMessageCount: 0 } },
    );

    await flushFrames();
    expect(mockDockviewState.scrollTarget).not.toBeNull();

    messageListRef.current = scrollHandle(true);
    rerender({ renderedMessageCount: 1 });
    await flushFrames();

    expect(messageListRef.current?.scrollToMessage).toHaveBeenCalledWith("message-1", {
      align: "start",
    });
    expect(mockDockviewState.clearScrollTarget).toHaveBeenCalledWith(7);
    expect(mockDockviewState.scrollTarget).toBeNull();
  });
});

describe("useScrollTargetConsumption — activation gating", () => {
  it("retains the target while inactive and consumes on activation without clearing the session", async () => {
    mockDockviewState.scrollTarget = target();
    const messageListRef = { current: scrollHandle(true) };

    const { rerender } = renderHook(
      ({ isVisible }) => useScrollTargetConsumption({ ...PROPS, isVisible, messageListRef }),
      { initialProps: { isVisible: false } },
    );

    expect(messageListRef.current?.scrollToMessage).not.toHaveBeenCalled();
    expect(mockDockviewState.clearScrollTargetForSession).not.toHaveBeenCalled();

    rerender({ isVisible: true });
    await flushFrames();

    expect(messageListRef.current?.scrollToMessage).toHaveBeenCalled();
    expect(mockDockviewState.clearScrollTarget).toHaveBeenCalledWith(7);
    expect(mockDockviewState.clearScrollTargetForSession).not.toHaveBeenCalled();
  });
});

describe("useScrollTargetConsumption — session-swap invalidation", () => {
  it("clears a delayed target for the previous session when the canonical chat swaps sessions", () => {
    mockDockviewState.scrollTarget = target();
    const messageListRef = { current: scrollHandle(false) };

    const { rerender } = renderHook(
      ({ resolvedSessionId }) =>
        useScrollTargetConsumption({ ...PROPS, resolvedSessionId, messageListRef }),
      { initialProps: { resolvedSessionId: "session-1" } },
    );
    expect(mockDockviewState.scrollTarget).not.toBeNull();

    // Canonical chat resolves session B: the A-target must be invalidated.
    rerender({ resolvedSessionId: "session-2" });

    expect(mockDockviewState.clearScrollTargetForOwner).toHaveBeenCalledWith(
      "session-1",
      "panel-a",
    );
    expect(mockDockviewState.scrollTarget).toBeNull();

    // Returning to A must NOT re-scroll (the target was cleared).
    messageListRef.current = scrollHandle(true);

    rerender({ resolvedSessionId: "session-1" });

    expect(messageListRef.current?.scrollToMessage).not.toHaveBeenCalled();
  });
});
it("retains a pre-existing owned target through StrictMode replay", async () => {
  mockDockviewState.scrollTarget = target();
  const messageListRef = { current: scrollHandle(true) };

  renderHook(() => useScrollTargetConsumption({ ...PROPS, messageListRef }), {
    wrapper: StrictMode,
  });
  await flushFrames();

  expect(messageListRef.current?.scrollToMessage).toHaveBeenCalledWith("message-1", {
    align: "start",
  });
});

// eslint-disable-next-line max-lines-per-function -- Canonical-host lifecycle regressions share one focused harness.
describe("useScrollTargetConsumption — canonical-host unmount ownership", () => {
  it("clears by the last resolved session and exact panel owner on unmount", async () => {
    vi.useFakeTimers({ toFake: ["setTimeout", "clearTimeout"] });
    mockDockviewState.scrollTarget = target();
    const messageListRef = { current: scrollHandle(false) };

    const { unmount } = renderHook(() => useScrollTargetConsumption({ ...PROPS, messageListRef }));
    expect(mockDockviewState.scrollTarget).not.toBeNull();

    unmount();
    await act(async () => {
      await vi.advanceTimersByTimeAsync(0);
    });

    expect(mockDockviewState.clearScrollTarget).toHaveBeenCalledWith(7);
    expect(mockDockviewState.scrollTarget).toBeNull();
  });
  it("preserves the same target when its Dockview owner immediately remounts", async () => {
    vi.useFakeTimers({ toFake: ["setTimeout", "clearTimeout"] });
    mockDockviewState.scrollTarget = target();
    const messageListRef = { current: scrollHandle(false) };
    const first = renderHook(() => useScrollTargetConsumption({ ...PROPS, messageListRef }));
    first.unmount();

    const second = renderHook(() => useScrollTargetConsumption({ ...PROPS, messageListRef }));
    await act(async () => {
      await vi.advanceTimersByTimeAsync(0);
    });
    expect(mockDockviewState.clearScrollTarget).not.toHaveBeenCalled();
    expect(mockDockviewState.scrollTarget).not.toBeNull();

    second.unmount();
    await act(async () => {
      await vi.advanceTimersByTimeAsync(0);
    });
    expect(mockDockviewState.clearScrollTarget).toHaveBeenCalledWith(7);
  });
  it("does not let deferred cleanup clear a replacement target", async () => {
    vi.useFakeTimers({ toFake: ["setTimeout", "clearTimeout"] });
    mockDockviewState.scrollTarget = target({ token: 7 });
    const first = renderHook(() =>
      useScrollTargetConsumption({ ...PROPS, messageListRef: { current: scrollHandle(false) } }),
    );
    first.unmount();

    mockDockviewState.scrollTarget = target({ token: 8, messageId: "replacement" });
    renderHook(() =>
      useScrollTargetConsumption({ ...PROPS, messageListRef: { current: scrollHandle(false) } }),
    );
    await act(async () => {
      await vi.advanceTimersByTimeAsync(0);
    });

    expect(mockDockviewState.clearScrollTarget).toHaveBeenCalledWith(7);
    expect(mockDockviewState.scrollTarget?.token).toBe(8);
  });

  it("does not clear a target owned by another panel on unmount", async () => {
    vi.useFakeTimers({ toFake: ["setTimeout", "clearTimeout"] });
    mockDockviewState.scrollTarget = target({ hostPanelId: "panel-a" });
    const messageListRef = { current: scrollHandle(false) };

    const { unmount } = renderHook(() =>
      useScrollTargetConsumption({ ...PROPS, panelId: "panel-b", messageListRef }),
    );
    unmount();
    await act(async () => {
      await vi.advanceTimersByTimeAsync(0);
    });

    expect(mockDockviewState.clearScrollTarget).not.toHaveBeenCalled();
    expect(mockDockviewState.scrollTarget).not.toBeNull();
  });

  it("reasserts an unloaded target once after the initial around-window placement", async () => {
    vi.useFakeTimers({ toFake: ["setTimeout", "clearTimeout"] });
    vi.mocked(loadMessageWindowAround).mockImplementationOnce(async () => {
      mockAppStoreState.messages.bySession["session-1"] = [{ id: "message-1" }];
      return { kind: "merged", merged: true, current: true, targetFound: true };
    });
    mockAppStoreState.messages.bySession["session-1"] = [];
    mockDockviewState.scrollTarget = target();
    const messageListRef = { current: scrollHandle(false) };
    const { rerender } = renderHook(
      ({ renderedMessageCount }) =>
        useScrollTargetConsumption({
          ...PROPS,
          renderedMessageCount,
          messageListRef,
        }),
      { initialProps: { renderedMessageCount: 0 } },
    );

    await flushFrames();
    expect(loadMessageWindowAround).toHaveBeenCalledTimes(1);
    await act(async () => {
      await Promise.resolve();
    });

    messageListRef.current = scrollHandle(true);
    rerender({ renderedMessageCount: 1 });
    await flushFrames();

    expect(messageListRef.current?.scrollToMessage).toHaveBeenCalledTimes(1);
    expect(mockDockviewState.scrollTarget).not.toBeNull();
    // Transcript revisions must not create a second delayed pass.
    rerender({ renderedMessageCount: 2 });
    await flushFrames();
    rerender({ renderedMessageCount: 3 });
    await flushFrames();

    await act(async () => {
      await vi.advanceTimersByTimeAsync(249);
    });
    expect(messageListRef.current?.scrollToMessage).toHaveBeenCalledTimes(1);

    await act(async () => {
      await vi.advanceTimersByTimeAsync(1);
    });
    expect(messageListRef.current?.scrollToMessage).toHaveBeenCalledTimes(2);
    expect(mockDockviewState.clearScrollTarget).toHaveBeenCalledWith(7);
    expect(mockDockviewState.scrollTarget).toBeNull();
    vi.useRealTimers();
  });
  it("does not retry a failed delayed reassertion after later transcript revisions", async () => {
    vi.useFakeTimers({ toFake: ["setTimeout", "clearTimeout"] });
    vi.mocked(loadMessageWindowAround).mockImplementationOnce(async () => {
      mockAppStoreState.messages.bySession["session-1"] = [{ id: "message-1" }];
      return { kind: "merged", merged: true, current: true, targetFound: true };
    });
    mockAppStoreState.messages.bySession["session-1"] = [];
    mockDockviewState.scrollTarget = target();
    const scrollToMessage = vi
      .fn()
      .mockReturnValueOnce(false)
      .mockReturnValueOnce(true)
      .mockReturnValue(false);
    const messageListRef = { current: { scrollToMessage } };
    const { rerender } = renderHook(
      ({ renderedMessageCount }) =>
        useScrollTargetConsumption({
          ...PROPS,
          renderedMessageCount,
          messageListRef,
        }),
      { initialProps: { renderedMessageCount: 0 } },
    );

    await flushFrames();
    await act(async () => {
      await Promise.resolve();
    });
    rerender({ renderedMessageCount: 1 });
    await flushFrames();
    expect(scrollToMessage).toHaveBeenCalledTimes(2);

    await act(async () => {
      await vi.advanceTimersByTimeAsync(250);
    });
    expect(scrollToMessage).toHaveBeenCalledTimes(3);

    rerender({ renderedMessageCount: 2 });
    await flushFrames();
    expect(scrollToMessage).toHaveBeenCalledTimes(3);
    expect(mockDockviewState.clearScrollTarget).toHaveBeenCalledWith(7);
    expect(mockDockviewState.scrollTarget).toBeNull();
  });
});
