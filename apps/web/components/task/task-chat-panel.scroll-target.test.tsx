import { act, renderHook } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { MessageListHandle } from "./chat/message-list-shared";

const { mockDockviewState } = vi.hoisted(() => {
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
  return { mockDockviewState: state };
});

vi.mock("@/lib/state/dockview-store", () => ({
  useDockviewStore: Object.assign(
    (selector: (state: typeof mockDockviewState) => unknown) => selector(mockDockviewState),
    { getState: () => mockDockviewState },
  ),
}));

import { useScrollTargetConsumption } from "./task-chat-panel";

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
  pendingFrames = [];
  vi.stubGlobal("requestAnimationFrame", (callback: () => void) => {
    pendingFrames.push(callback);
    return pendingFrames.length;
  });
  vi.stubGlobal("cancelAnimationFrame", (id: number) => {
    pendingFrames[id - 1] = undefined;
  });
  mockDockviewState.scrollTarget = null;
});

afterEach(() => {
  vi.unstubAllGlobals();
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

describe("useScrollTargetConsumption — canonical-host unmount ownership", () => {
  it("clears by the last resolved session and exact panel owner on unmount", () => {
    mockDockviewState.scrollTarget = target();
    const messageListRef = { current: scrollHandle(false) };

    const { unmount } = renderHook(() => useScrollTargetConsumption({ ...PROPS, messageListRef }));
    expect(mockDockviewState.scrollTarget).not.toBeNull();

    unmount();

    expect(mockDockviewState.clearScrollTargetForOwner).toHaveBeenCalledWith(
      "session-1",
      "panel-a",
    );
    expect(mockDockviewState.scrollTarget).toBeNull();
  });

  it("does not clear a target owned by another panel on unmount", () => {
    mockDockviewState.scrollTarget = target({ hostPanelId: "panel-a" });
    const messageListRef = { current: scrollHandle(false) };

    const { unmount } = renderHook(() =>
      useScrollTargetConsumption({ ...PROPS, panelId: "panel-b", messageListRef }),
    );
    unmount();

    expect(mockDockviewState.clearScrollTargetForOwner).toHaveBeenCalledWith(
      "session-1",
      "panel-b",
    );
    expect(mockDockviewState.scrollTarget).not.toBeNull();
  });
});
