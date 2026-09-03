/* eslint-disable max-lines -- native scroll behavior and regression cases share one harness. */
import { useLayoutEffect, useRef } from "react";
import { act, cleanup, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { Message } from "@/lib/types/http";
import type { RenderItem } from "@/hooks/use-processed-messages";

const sharedSentinelCalls = vi.hoisted(() => [] as unknown[][]);
const sharedSentinelUserGesture = vi.hoisted(() => vi.fn());
const sharedSentinelRetry = vi.hoisted(() => vi.fn());
const sharedSentinelRecheck = vi.hoisted(() => vi.fn());
const transcriptScrollTopBySessionId = vi.hoisted(() => new Map<string, number>());
const transcriptScrollTopWrites = vi.hoisted(() =>
  vi.fn((sessionId: string, scrollTop: number) => {
    transcriptScrollTopBySessionId.set(sessionId, scrollTop);
  }),
);
const mockDockviewState = vi.hoisted(
  () =>
    ({
      pendingChatScrollTop: null,
      scrollTarget: null,
    }) as {
      pendingChatScrollTop: number | null;
      scrollTarget: { sessionId: string } | null;
    },
);

vi.mock("@/hooks/use-lazy-load-sentinel", () => ({
  useLazyLoadSentinel: (...args: unknown[]) => {
    sharedSentinelCalls.push(args);
    return {
      sentinelRef: () => {},
      onUserGesture: sharedSentinelUserGesture,
      retry: sharedSentinelRetry,
      recheck: sharedSentinelRecheck,
    };
  },
}));

vi.mock("@/lib/state/dockview-store", () => ({
  useDockviewStore: Object.assign(
    (selector: (state: typeof mockDockviewState) => unknown) => selector(mockDockviewState),
    { getState: () => mockDockviewState },
  ),
}));

vi.mock("@/components/state-provider", () => ({
  useAppStoreApi: () => ({
    getState: () => ({
      setTranscriptScrollTop: transcriptScrollTopWrites,
      transcriptAutoScroll: {
        scrollTopBySessionId: Object.fromEntries(transcriptScrollTopBySessionId),
      },
    }),
  }),
}));

import { useScrollToDividerOrBottom } from "./message-list-native";
import {
  isElementInPreloadRegion,
  resolvePaginationStopReason,
  TRANSCRIPT_SENTINEL_ROOT_MARGIN,
  useAutoScroll,
  useNativeScrollManagement,
  useScrollToMessage,
} from "./message-list-native-scroll";

const DIVIDER_KEY = "m2";
const DIVIDER_SCROLL_CONTAINER_TEST_ID = "divider-scroll-container";
const SCROLL_TO_MESSAGE_ROOT = "scroll-to-message-root";
const MISSING_SCROLL_CONTAINER_ERROR = "scroll container did not render";
const TARGET_MESSAGE_ID = "target";
const HANDLE_RENDER_ERROR = "handle did not render";
const HARNESS_RENDER_ERROR = "harness did not render";
const NATIVE_SCROLL_MANAGEMENT_TEST_ID = "native-scroll-management-container";
const AUTO_SCROLL_CONTAINER_TEST_ID = "auto-scroll-container";
const TEST_MESSAGES = [{} as Message];
/** Always returns false: the harness never locks programmatic scrolling. */
const NEVER_LOCKED = () => false;

function touchEvent(type: "touchstart" | "touchmove", clientY: number): TouchEvent {
  const event = new Event(type) as TouchEvent;
  Object.defineProperty(event, "touches", { value: [{ clientY }] });
  return event;
}

/** Renders a scroll container wired to useScrollToDividerOrBottom with mocked divider geometry. */
function Harness({
  itemCount,
  anchoredBarOffsetPx,
  dividerKey = DIVIDER_KEY,
  onDividerScroll,
  scrollLayoutKey = "initial",
  dividerDocumentTop = 250,
  isVisible = true,
  scrollHeight = 0,
  enabled = true,
  sessionId = null,
  isProgrammaticScrollLocked = NEVER_LOCKED,
}: {
  itemCount: number;
  anchoredBarOffsetPx: number;
  dividerKey?: string | null;
  onDividerScroll?: () => void;
  scrollLayoutKey?: string;
  dividerDocumentTop?: number;
  isVisible?: boolean;
  scrollHeight?: number;
  enabled?: boolean;
  sessionId?: string | null;
  isProgrammaticScrollLocked?: () => boolean;
}) {
  const scrollRef = useRef<HTMLDivElement>(null);
  useLayoutEffect(() => {
    const scrollContainer = scrollRef.current;
    if (!scrollContainer) return;
    Object.defineProperty(scrollContainer, "scrollHeight", {
      configurable: true,
      value: scrollHeight,
    });
    Object.defineProperty(scrollContainer, "getBoundingClientRect", {
      configurable: true,
      value: () => createRect(100, 400),
    });
    const divider = scrollContainer.querySelector<HTMLElement>(`[id="msg-${DIVIDER_KEY}"]`);
    if (!divider) return;
    Object.defineProperty(divider, "getBoundingClientRect", {
      configurable: true,
      value: () => createRect(dividerDocumentTop - scrollContainer.scrollTop, 20),
    });
  }, [dividerDocumentTop]);
  const scrollOptions = {
    onDividerScroll,
    scrollLayoutKey,
    enabled,
    sessionId,
    isProgrammaticScrollLocked,
    isVisible,
  } as Parameters<typeof useScrollToDividerOrBottom>[4];
  useScrollToDividerOrBottom(scrollRef, itemCount, dividerKey, anchoredBarOffsetPx, scrollOptions);
  return (
    <div ref={scrollRef} data-testid={DIVIDER_SCROLL_CONTAINER_TEST_ID}>
      <div id="msg-m1" />
      <div id={`msg-${DIVIDER_KEY}`} />
    </div>
  );
}

/** Builds a DOMRect-like object positioned at the given top with the given height. */
function createRect(top: number, height: number): DOMRect {
  return {
    x: 0,
    y: top,
    top,
    bottom: top + height,
    left: 0,
    right: 100,
    width: 100,
    height,
    toJSON: () => ({}),
  } as DOMRect;
}

/** Renders a scroll container running useAutoScroll and exposes its markNotNearBottom callback. */
function AutoScrollHarness({
  isWorking,
  hasUnreadDivider,
  messages = TEST_MESSAGES,
  markRef,
  enabled = true,
  isVisible = true,
  sessionId = null,
  metrics,
}: {
  isWorking: boolean;
  hasUnreadDivider: boolean;
  messages?: Message[];
  markRef?: { current?: () => void };
  enabled?: boolean;
  isVisible?: boolean;
  sessionId?: string | null;
  metrics?: NativeScrollMetrics;
}) {
  const scrollRef = useRef<HTMLDivElement>(null);
  const useAutoScrollWithVisibility = useAutoScroll as unknown as (
    params: Parameters<typeof useAutoScroll>[0] & { isVisible: boolean },
  ) => ReturnType<typeof useAutoScroll>;
  const { markNotNearBottom } = useAutoScrollWithVisibility({
    scrollRef,
    messages,
    isWorking,
    sessionId,
    enabled,
    hasUnreadDivider,
    isProgrammaticScrollLocked: NEVER_LOCKED,
    isVisible,
  });
  useNativeScrollMetrics(scrollRef, metrics);
  if (markRef) markRef.current = markNotNearBottom;
  return <div ref={scrollRef} data-testid={AUTO_SCROLL_CONTAINER_TEST_ID} />;
}

/** Stubs scrollHeight (1000) and clientHeight (400) onto an element. */
function setScrollMetrics(element: HTMLElement) {
  Object.defineProperty(element, "scrollHeight", { configurable: true, value: 1000 });
  Object.defineProperty(element, "clientHeight", { configurable: true, value: 400 });
}

afterEach(() => {
  cleanup();
  mockDockviewState.pendingChatScrollTop = null;
  mockDockviewState.scrollTarget = null;
  sharedSentinelCalls.length = 0;
  sharedSentinelUserGesture.mockReset();
  sharedSentinelRetry.mockReset();
  sharedSentinelRecheck.mockReset();
  transcriptScrollTopBySessionId.clear();
  transcriptScrollTopWrites.mockClear();
  vi.restoreAllMocks();
});

function transcriptMessage(id: string): RenderItem {
  return { type: "message", message: { id } as Message };
}

function transcriptActivity(id: string, turnId = id): RenderItem {
  return { type: "turn_group", id, turnId, messages: [] };
}

type NativeScrollMetrics = {
  scrollHeight: number;
  scrollTop: number;
  clientHeight: number;
};

function useNativeScrollMetrics(
  scrollRef: { current: HTMLDivElement | null },
  metrics?: NativeScrollMetrics,
) {
  useLayoutEffect(() => {
    const element = scrollRef.current;
    if (!element || !metrics) return;
    Object.defineProperties(element, {
      scrollHeight: { configurable: true, get: () => metrics.scrollHeight },
      clientHeight: { configurable: true, get: () => metrics.clientHeight },
      scrollTop: {
        configurable: true,
        get: () => metrics.scrollTop,
        set: (value: number) => {
          metrics.scrollTop = value;
        },
      },
    });
  }, [metrics, scrollRef]);
}

function NativeScrollManagementHarness({
  items,
  metrics,
  loadMore = async () => 0,
  sessionId = null,
  isLoadingMore = false,
  recoveryRef,
  isVisible = true,
  enabled = false,
}: {
  items: RenderItem[];
  metrics?: NativeScrollMetrics;
  loadMore?: () => Promise<number>;
  sessionId?: string | null;
  isLoadingMore?: boolean;
  recoveryRef?: { current: boolean };
  isVisible?: boolean;
  enabled?: boolean;
}) {
  const scrollRef = useRef<HTMLDivElement>(null);
  useNativeScrollMetrics(scrollRef, metrics);
  const { sentinelRef, showRecovery } = useNativeScrollManagement({
    scrollRef,
    items,
    messages: [],
    isWorking: false,
    sessionId,
    enabled,
    hasUnreadDivider: false,
    messagesLoading: false,
    hasMore: true,
    isLoadingMore,
    loadMore,
    isVisible,
  });
  if (recoveryRef) recoveryRef.current = showRecovery;
  return (
    <div data-testid={NATIVE_SCROLL_MANAGEMENT_TEST_ID} ref={scrollRef}>
      <div ref={sentinelRef} />
    </div>
  );
}

describe("resolvePaginationStopReason", () => {
  it.each([
    { boundaryUnchanged: true, hasMore: true, expected: "visible-boundary-unchanged" },
    { boundaryUnchanged: false, hasMore: true, expected: "visible-boundary-added" },
    { boundaryUnchanged: true, hasMore: false, expected: "exhausted" },
    { boundaryUnchanged: false, hasMore: false, expected: "exhausted" },
  ])(
    "resolves pagination stop reason for $expected",
    ({ boundaryUnchanged, hasMore, expected }) => {
      expect(resolvePaginationStopReason(boundaryUnchanged, hasMore)).toBe(expected);
    },
  );
});

describe("isElementInPreloadRegion", () => {
  it("uses current root and sentinel geometry instead of a stale intersection", () => {
    const root = document.createElement("div");
    const sentinel = document.createElement("div");
    Object.defineProperty(root, "getBoundingClientRect", {
      configurable: true,
      value: () => createRect(100, 400),
    });
    Object.defineProperty(sentinel, "getBoundingClientRect", {
      configurable: true,
      value: () => createRect(-102, 1),
    });

    expect(isElementInPreloadRegion(root, sentinel, TRANSCRIPT_SENTINEL_ROOT_MARGIN)).toBe(false);

    Object.defineProperty(sentinel, "getBoundingClientRect", {
      configurable: true,
      value: () => createRect(-50, 1),
    });
    expect(isElementInPreloadRegion(root, sentinel, TRANSCRIPT_SENTINEL_ROOT_MARGIN)).toBe(true);
  });
});

// eslint-disable-next-line max-lines-per-function -- pagination invariants share one fixture and lifecycle.
describe("useNativeScrollManagement transcript pagination", () => {
  it("defers the native initial placement until a hidden transcript is active", () => {
    const frames: Array<FrameRequestCallback> = [];
    vi.stubGlobal("requestAnimationFrame", (callback: FrameRequestCallback) => {
      frames.push(callback);
      return frames.length;
    });
    const metrics = { scrollHeight: 1000, scrollTop: 250, clientHeight: 400 };
    try {
      const { rerender } = render(
        <NativeScrollManagementHarness
          items={[transcriptMessage("initial-message")]}
          metrics={metrics}
          enabled
          isVisible={false}
        />,
      );
      const scrollContainer = screen.getByTestId(NATIVE_SCROLL_MANAGEMENT_TEST_ID);
      expect(scrollContainer.scrollTop).toBe(250);

      rerender(
        <NativeScrollManagementHarness
          items={[transcriptMessage("initial-message")]}
          metrics={metrics}
          enabled
          isVisible
        />,
      );
      metrics.scrollTop = 250;
      act(() => {
        for (let frame = frames.shift(); frame; frame = frames.shift()) frame(0);
      });

      expect(scrollContainer.scrollTop).toBe(1000);
    } finally {
      vi.unstubAllGlobals();
    }
  });

  it("rechecks restored geometry once when a hidden transcript becomes visible", () => {
    const requestAnimationFrame = vi
      .spyOn(window, "requestAnimationFrame")
      .mockImplementation((callback) => {
        callback(0);
        return 1;
      });
    const { rerender } = render(<NativeScrollManagementHarness items={[]} isVisible={false} />);

    rerender(<NativeScrollManagementHarness items={[]} isVisible />);

    expect(sharedSentinelRecheck).toHaveBeenCalledTimes(1);
    requestAnimationFrame.mockRestore();
  });

  it("rechecks pagination on upward input at the hard top without a scroll event", () => {
    const metrics = { scrollHeight: 1000, scrollTop: 0, clientHeight: 400 };
    render(<NativeScrollManagementHarness items={[]} metrics={metrics} />);
    const scroller = screen.getByTestId(NATIVE_SCROLL_MANAGEMENT_TEST_ID);

    act(() => {
      scroller.dispatchEvent(new WheelEvent("wheel", { deltaY: -1 }));
    });

    expect(sharedSentinelRecheck).toHaveBeenCalledTimes(1);
  });

  it("rechecks pagination for every upward keyboard command at the hard top", () => {
    const metrics = { scrollHeight: 1000, scrollTop: 0, clientHeight: 400 };
    render(<NativeScrollManagementHarness items={[]} metrics={metrics} />);
    const scroller = screen.getByTestId(NATIVE_SCROLL_MANAGEMENT_TEST_ID);

    for (const key of ["ArrowUp", "PageUp", "Home"]) {
      act(() => {
        scroller.dispatchEvent(new KeyboardEvent("keydown", { key }));
      });
    }

    expect(sharedSentinelRecheck).toHaveBeenCalledTimes(3);
  });

  it("ignores keyboard input away from the hard top and non-upward commands", () => {
    const metrics = { scrollHeight: 1000, scrollTop: 50, clientHeight: 400 };
    render(<NativeScrollManagementHarness items={[]} metrics={metrics} />);
    const scroller = screen.getByTestId(NATIVE_SCROLL_MANAGEMENT_TEST_ID);

    act(() => {
      scroller.dispatchEvent(new KeyboardEvent("keydown", { key: "ArrowUp" }));
      metrics.scrollTop = 0;
      scroller.dispatchEvent(new KeyboardEvent("keydown", { key: "ArrowDown" }));
    });

    expect(sharedSentinelRecheck).not.toHaveBeenCalled();
  });

  it("rechecks once for a directional touch gesture at the hard top", () => {
    const metrics = { scrollHeight: 1000, scrollTop: 0, clientHeight: 400 };
    render(<NativeScrollManagementHarness items={[]} metrics={metrics} />);
    const scroller = screen.getByTestId(NATIVE_SCROLL_MANAGEMENT_TEST_ID);

    act(() => {
      scroller.dispatchEvent(touchEvent("touchstart", 300));
      scroller.dispatchEvent(touchEvent("touchmove", 350));
      scroller.dispatchEvent(touchEvent("touchmove", 400));
    });

    expect(sharedSentinelRecheck).toHaveBeenCalledTimes(1);
  });

  it("ignores non-upward touch input and touch input away from the hard top", () => {
    const metrics = { scrollHeight: 1000, scrollTop: 0, clientHeight: 400 };
    render(<NativeScrollManagementHarness items={[]} metrics={metrics} />);
    const scroller = screen.getByTestId(NATIVE_SCROLL_MANAGEMENT_TEST_ID);

    act(() => {
      scroller.dispatchEvent(touchEvent("touchstart", 300));
      scroller.dispatchEvent(touchEvent("touchmove", 250));
      metrics.scrollTop = 50;
      scroller.dispatchEvent(touchEvent("touchstart", 300));
      scroller.dispatchEvent(touchEvent("touchmove", 350));
    });

    expect(sharedSentinelRecheck).not.toHaveBeenCalled();
  });

  it("does not recheck a restored transcript while explicit recovery is active", () => {
    const requestAnimationFrame = vi
      .spyOn(window, "requestAnimationFrame")
      .mockImplementation((callback) => {
        callback(0);
        return 1;
      });
    const recoveryRef = { current: false };
    const { rerender } = render(
      <NativeScrollManagementHarness
        items={[]}
        sessionId="session-1"
        recoveryRef={recoveryRef}
        isVisible={false}
      />,
    );
    const options = sharedSentinelCalls.at(-1)?.[5] as {
      onLoadSettled: (result: {
        count: number;
        rejected: boolean;
        continuation: "no-progress";
      }) => void;
    };
    act(() => {
      options.onLoadSettled({ count: 0, rejected: false, continuation: "no-progress" });
    });
    sharedSentinelRecheck.mockClear();

    rerender(
      <NativeScrollManagementHarness
        items={[]}
        sessionId="session-1"
        recoveryRef={recoveryRef}
        isVisible
      />,
    );

    expect(recoveryRef.current).toBe(true);
    expect(sharedSentinelRecheck).not.toHaveBeenCalled();
    requestAnimationFrame.mockRestore();
  });

  it("retries a disarmed short page on the next upward scroll", () => {
    const metrics = { scrollHeight: 1000, scrollTop: 100, clientHeight: 400 };
    render(<NativeScrollManagementHarness items={[]} metrics={metrics} />);
    const scroller = screen.getByTestId(NATIVE_SCROLL_MANAGEMENT_TEST_ID);

    act(() => {
      metrics.scrollTop = 80;
      scroller.dispatchEvent(new Event("scroll"));
    });
    expect(sharedSentinelUserGesture).toHaveBeenCalledTimes(1);

    act(() => {
      metrics.scrollTop = 120;
      scroller.dispatchEvent(new Event("scroll"));
    });
    expect(sharedSentinelUserGesture).toHaveBeenCalledTimes(1);
  });

  it("re-arms the native sentinel with a current-geometry continuation decision", () => {
    render(<NativeScrollManagementHarness items={[]} />);

    const options = sharedSentinelCalls[0]?.[5];
    expect(options).toEqual({
      rootMargin: "200px 0px 0px 0px",
      rearmWhileIntersecting: true,
      shouldContinueWhileIntersecting: expect.any(Function),
      isCurrentGeometryEligible: expect.any(Function),
      onLoadSettled: expect.any(Function),
      isRequestCurrent: expect.any(Function),
    });
  });

  it("shows recovery only after no progress and clears it after success or session change", () => {
    const recoveryRef = { current: false };
    const { rerender } = render(
      <NativeScrollManagementHarness items={[]} sessionId="session-1" recoveryRef={recoveryRef} />,
    );
    const options = sharedSentinelCalls.at(-1)?.[5] as {
      onLoadSettled: (result: {
        count: number;
        rejected: boolean;
        continuation: "no-progress" | "continued";
      }) => void;
    };

    act(() => {
      options.onLoadSettled({ count: 0, rejected: false, continuation: "no-progress" });
    });
    expect(recoveryRef.current).toBe(true);

    act(() => {
      options.onLoadSettled({ count: 20, rejected: false, continuation: "continued" });
    });
    expect(recoveryRef.current).toBe(false);

    act(() => {
      options.onLoadSettled({ count: 0, rejected: true, continuation: "no-progress" });
    });
    expect(recoveryRef.current).toBe(true);
    rerender(
      <NativeScrollManagementHarness items={[]} sessionId="session-2" recoveryRef={recoveryRef} />,
    );
    expect(recoveryRef.current).toBe(false);
  });

  it("ignores a settlement from the previous session epoch", async () => {
    const recoveryRef = { current: false };
    let resolveLoad: (value: number) => void = () => {};
    const loadMore = vi.fn(
      () =>
        new Promise<number>((resolve) => {
          resolveLoad = resolve;
        }),
    );
    const { rerender } = render(
      <NativeScrollManagementHarness
        items={[]}
        sessionId="session-1"
        loadMore={loadMore}
        recoveryRef={recoveryRef}
      />,
    );
    const initialCall = sharedSentinelCalls.at(-1);
    const loadPage = initialCall?.[4] as () => Promise<number>;
    const oldOptions = initialCall?.[5] as {
      onLoadSettled: (result: {
        count: number;
        rejected: boolean;
        continuation: "rejected";
      }) => void;
    };
    const pendingLoad = loadPage();

    rerender(
      <NativeScrollManagementHarness
        items={[]}
        sessionId="session-2"
        loadMore={loadMore}
        recoveryRef={recoveryRef}
      />,
    );
    act(() => {
      oldOptions.onLoadSettled({ count: 0, rejected: true, continuation: "rejected" });
    });
    expect(recoveryRef.current).toBe(false);

    resolveLoad(0);
    await pendingLoad;
  });

  it("continues while the sentinel remains in preload even when the visible boundary changes", async () => {
    const loadMore = vi.fn(async () => 20);
    const newest = transcriptMessage("newest");
    const { rerender } = render(
      <NativeScrollManagementHarness
        items={[transcriptActivity("activity-before", "turn-1"), newest]}
        loadMore={loadMore}
      />,
    );
    const wrappedLoadMore = sharedSentinelCalls.at(-1)?.[4] as () => Promise<number>;

    await wrappedLoadMore();
    rerender(
      <NativeScrollManagementHarness
        items={[transcriptActivity("activity-after", "turn-1"), newest]}
        loadMore={loadMore}
      />,
    );
    let options = sharedSentinelCalls.at(-1)?.[5] as {
      shouldContinueWhileIntersecting: () => boolean;
    };
    expect(options.shouldContinueWhileIntersecting()).toBe(true);

    await wrappedLoadMore();
    rerender(
      <NativeScrollManagementHarness
        items={[transcriptActivity("activity-new", "turn-2"), newest]}
        loadMore={loadMore}
      />,
    );
    options = sharedSentinelCalls.at(-1)?.[5] as {
      shouldContinueWhileIntersecting: () => boolean;
    };
    expect(options.shouldContinueWhileIntersecting()).toBe(true);
  });

  it("anchors a prepend below a fixed task description row", () => {
    const metrics = { scrollHeight: 100, scrollTop: 40, clientHeight: 50 };
    const taskDescription = transcriptMessage("task-description");
    const newest = transcriptMessage("newest");
    const { rerender } = render(
      <NativeScrollManagementHarness items={[taskDescription, newest]} metrics={metrics} />,
    );

    act(() => {
      metrics.scrollTop = 40;
      screen.getByTestId(NATIVE_SCROLL_MANAGEMENT_TEST_ID).dispatchEvent(new Event("scroll"));
    });
    metrics.scrollHeight = 200;
    rerender(
      <NativeScrollManagementHarness
        items={[taskDescription, transcriptMessage("older"), newest]}
        metrics={metrics}
      />,
    );

    expect(metrics.scrollTop).toBe(140);
  });

  it("anchors a stored prompt replacing the synthetic task description", () => {
    const metrics = { scrollHeight: 100, scrollTop: 40, clientHeight: 50 };
    const taskDescription = transcriptMessage("task-description");
    const newest = transcriptMessage("newest");
    const { rerender } = render(
      <NativeScrollManagementHarness items={[taskDescription, newest]} metrics={metrics} />,
    );

    act(() => {
      metrics.scrollTop = 40;
      screen.getByTestId(NATIVE_SCROLL_MANAGEMENT_TEST_ID).dispatchEvent(new Event("scroll"));
    });
    metrics.scrollHeight = 200;
    rerender(
      <NativeScrollManagementHarness
        items={[transcriptMessage("stored-prompt"), newest]}
        metrics={metrics}
      />,
    );

    expect(metrics.scrollTop).toBe(140);
  });

  it("anchors each committed prepend while an accumulated load remains active", () => {
    const metrics = { scrollHeight: 100, scrollTop: 40, clientHeight: 50 };
    const activity = transcriptActivity("activity-old", "turn-1");
    const newest = transcriptMessage("newest");
    const { rerender } = render(
      <NativeScrollManagementHarness items={[activity, newest]} metrics={metrics} />,
    );

    act(() => {
      metrics.scrollTop = 40;
      screen.getByTestId(NATIVE_SCROLL_MANAGEMENT_TEST_ID).dispatchEvent(new Event("scroll"));
    });
    rerender(
      <NativeScrollManagementHarness items={[activity, newest]} metrics={metrics} isLoadingMore />,
    );
    metrics.scrollHeight = 200;
    rerender(
      <NativeScrollManagementHarness
        items={[transcriptActivity("activity-new", "turn-1"), newest]}
        metrics={metrics}
        isLoadingMore
      />,
    );

    expect(metrics.scrollTop).toBe(140);
  });

  it("freezes the prepend baseline synchronously when an older load starts", async () => {
    const page = Promise.withResolvers<number>();
    const metrics = { scrollHeight: 100, scrollTop: 40, clientHeight: 50 };
    const activity = transcriptActivity("activity-old", "turn-1");
    const newest = transcriptMessage("newest");
    const { rerender } = render(
      <NativeScrollManagementHarness
        items={[activity, newest]}
        metrics={metrics}
        loadMore={() => page.promise}
      />,
    );

    act(() => {
      metrics.scrollTop = 40;
      screen.getByTestId(NATIVE_SCROLL_MANAGEMENT_TEST_ID).dispatchEvent(new Event("scroll"));
    });
    const wrappedLoadMore = sharedSentinelCalls.at(-1)?.[4] as () => Promise<number>;
    const pendingLoad = wrappedLoadMore();
    act(() => {
      metrics.scrollTop = 80;
      screen.getByTestId(NATIVE_SCROLL_MANAGEMENT_TEST_ID).dispatchEvent(new Event("scroll"));
    });
    metrics.scrollHeight = 200;
    rerender(
      <NativeScrollManagementHarness
        items={[transcriptActivity("activity-new", "turn-1"), newest]}
        metrics={metrics}
        loadMore={() => page.promise}
        isLoadingMore
      />,
    );

    expect(metrics.scrollTop).toBe(140);
    page.resolve(20);
    await pendingLoad;
  });
});

// eslint-disable-next-line max-lines-per-function -- this suite keeps the related scroll invariants together.
describe("useScrollToDividerOrBottom — anchored-bar offset", () => {
  it("waits for an inactive transcript to become visible before placing the initial view", () => {
    const frames: Array<FrameRequestCallback> = [];
    vi.stubGlobal("requestAnimationFrame", (callback: FrameRequestCallback) => {
      frames.push(callback);
      return frames.length;
    });
    try {
      const { rerender } = render(
        <Harness
          itemCount={2}
          anchoredBarOffsetPx={0}
          dividerKey={null}
          isVisible={false}
          scrollHeight={1000}
        />,
      );
      const scrollContainer = screen.getByTestId(DIVIDER_SCROLL_CONTAINER_TEST_ID) as HTMLElement;

      expect(scrollContainer.scrollTop).toBe(0);

      rerender(
        <Harness
          itemCount={2}
          anchoredBarOffsetPx={0}
          dividerKey={null}
          isVisible
          scrollHeight={1000}
        />,
      );
      // Model SessionPanelContent restoring its saved absolute offset after
      // the panel becomes measurable. The activation placement must happen
      // after that restore, not before it.
      scrollContainer.scrollTop = 250;
      expect(scrollContainer.scrollTop).toBe(250);

      act(() => frames.shift()?.(0));
      expect(scrollContainer.scrollTop).toBe(250);

      act(() => frames.shift()?.(0));
      expect(scrollContainer.scrollTop).toBe(1000);
    } finally {
      vi.unstubAllGlobals();
    }
  });

  it("starts and bounds the divider settling window at hidden-session activation", () => {
    vi.useFakeTimers();
    let currentTime = 0;
    vi.spyOn(Date, "now").mockImplementation(() => currentTime);
    const frames: Array<FrameRequestCallback> = [];
    vi.stubGlobal("requestAnimationFrame", (callback: FrameRequestCallback) => {
      frames.push(callback);
      return frames.length;
    });
    try {
      const { rerender } = render(
        <Harness itemCount={2} anchoredBarOffsetPx={0} isVisible={false} scrollHeight={1000} />,
      );
      const scrollContainer = screen.getByTestId(DIVIDER_SCROLL_CONTAINER_TEST_ID) as HTMLElement;

      vi.advanceTimersByTime(4001);
      currentTime = 4001;

      rerender(<Harness itemCount={2} anchoredBarOffsetPx={0} isVisible scrollHeight={1000} />);
      act(() => {
        let frame = frames.shift();
        while (frame) {
          frame(0);
          frame = frames.shift();
        }
      });
      // The deadline starts at activation, so the divider still wins even
      // though the persistent hidden portal was mounted for more than 4s.
      expect(scrollContainer.scrollTop).toBe(150);

      rerender(
        <Harness
          itemCount={2}
          anchoredBarOffsetPx={76}
          isVisible
          scrollHeight={1000}
          scrollLayoutKey="activation-window"
        />,
      );
      expect(scrollContainer.scrollTop).toBe(74);

      vi.advanceTimersByTime(4001);
      currentTime = 8002;
      rerender(
        <Harness
          itemCount={2}
          anchoredBarOffsetPx={100}
          isVisible
          scrollHeight={1000}
          scrollLayoutKey="settled"
        />,
      );
      expect(scrollContainer.scrollTop).toBe(74);
    } finally {
      vi.unstubAllGlobals();
      vi.useRealTimers();
    }
  });

  it("does not replace a pending layout restore with activation placement", () => {
    const frames: Array<FrameRequestCallback> = [];
    vi.stubGlobal("requestAnimationFrame", (callback: FrameRequestCallback) => {
      frames.push(callback);
      return frames.length;
    });
    mockDockviewState.pendingChatScrollTop = 42;
    try {
      const { rerender } = render(
        <Harness
          itemCount={2}
          anchoredBarOffsetPx={0}
          dividerKey={null}
          isVisible={false}
          scrollHeight={1000}
        />,
      );
      const scrollContainer = screen.getByTestId(DIVIDER_SCROLL_CONTAINER_TEST_ID);
      scrollContainer.scrollTop = 250;

      rerender(
        <Harness
          itemCount={2}
          anchoredBarOffsetPx={0}
          dividerKey={null}
          isVisible
          scrollHeight={1000}
        />,
      );
      act(() => {
        for (const frame of frames.splice(0)) frame(0);
      });

      expect(scrollContainer.scrollTop).toBe(250);
      mockDockviewState.pendingChatScrollTop = null;
      rerender(
        <Harness
          itemCount={2}
          anchoredBarOffsetPx={0}
          dividerKey={null}
          isVisible
          scrollHeight={1000}
          scrollLayoutKey="restored"
        />,
      );
      expect(scrollContainer.scrollTop).toBe(250);
    } finally {
      vi.unstubAllGlobals();
    }
  });

  it("does not replace an explicit message target with activation placement", () => {
    const frames: Array<FrameRequestCallback> = [];
    vi.stubGlobal("requestAnimationFrame", (callback: FrameRequestCallback) => {
      frames.push(callback);
      return frames.length;
    });
    mockDockviewState.scrollTarget = { sessionId: "session-1" };
    try {
      const { rerender } = render(
        <Harness
          itemCount={2}
          anchoredBarOffsetPx={0}
          dividerKey={null}
          sessionId="session-1"
          isVisible={false}
          scrollHeight={1000}
        />,
      );
      const scrollContainer = screen.getByTestId(DIVIDER_SCROLL_CONTAINER_TEST_ID);
      scrollContainer.scrollTop = 250;

      rerender(
        <Harness
          itemCount={2}
          anchoredBarOffsetPx={0}
          dividerKey={null}
          sessionId="session-1"
          isVisible
          scrollHeight={1000}
        />,
      );
      act(() => {
        for (const frame of frames.splice(0)) frame(0);
      });

      expect(scrollContainer.scrollTop).toBe(250);
      mockDockviewState.scrollTarget = null;
      rerender(
        <Harness
          itemCount={2}
          anchoredBarOffsetPx={0}
          dividerKey={null}
          sessionId="session-1"
          isVisible
          scrollHeight={1000}
          scrollLayoutKey="target-consumed"
        />,
      );
      expect(scrollContainer.scrollTop).toBe(250);
    } finally {
      vi.unstubAllGlobals();
    }
  });

  it("does not bottom-place a disabled transcript during activation", () => {
    const frames: Array<FrameRequestCallback> = [];
    vi.stubGlobal("requestAnimationFrame", (callback: FrameRequestCallback) => {
      frames.push(callback);
      return frames.length;
    });
    try {
      const { rerender } = render(
        <Harness
          itemCount={2}
          anchoredBarOffsetPx={0}
          dividerKey={null}
          enabled={false}
          isVisible={false}
          scrollHeight={1000}
        />,
      );
      const scrollContainer = screen.getByTestId(DIVIDER_SCROLL_CONTAINER_TEST_ID);
      scrollContainer.scrollTop = 250;

      rerender(
        <Harness
          itemCount={2}
          anchoredBarOffsetPx={0}
          dividerKey={null}
          enabled={false}
          isVisible
          scrollHeight={1000}
        />,
      );
      act(() => {
        for (const frame of frames.splice(0)) frame(0);
      });

      expect(scrollContainer.scrollTop).toBe(250);
    } finally {
      vi.unstubAllGlobals();
    }
  });

  it("cancels pending activation placement when the transcript becomes inactive again", () => {
    const frames: Array<FrameRequestCallback> = [];
    vi.stubGlobal("requestAnimationFrame", (callback: FrameRequestCallback) => {
      frames.push(callback);
      return frames.length;
    });
    try {
      const { rerender } = render(
        <Harness
          itemCount={2}
          anchoredBarOffsetPx={0}
          dividerKey={null}
          isVisible={false}
          scrollHeight={1000}
        />,
      );
      const scrollContainer = screen.getByTestId(DIVIDER_SCROLL_CONTAINER_TEST_ID) as HTMLElement;
      scrollContainer.scrollTop = 250;

      rerender(
        <Harness
          itemCount={2}
          anchoredBarOffsetPx={0}
          dividerKey={null}
          isVisible
          scrollHeight={1000}
        />,
      );
      rerender(
        <Harness
          itemCount={2}
          anchoredBarOffsetPx={0}
          dividerKey={null}
          isVisible={false}
          scrollHeight={1000}
        />,
      );

      act(() => {
        for (const frame of frames.splice(0)) frame(0);
      });
      expect(scrollContainer.scrollTop).toBe(250);
    } finally {
      vi.unstubAllGlobals();
    }
  });

  it("re-scrolls the divider when the anchored bar's measured height arrives", () => {
    const { rerender } = render(<Harness itemCount={2} anchoredBarOffsetPx={0} />);
    const scrollContainer = document.querySelector<HTMLElement>(
      `[data-testid="${DIVIDER_SCROLL_CONTAINER_TEST_ID}"]`,
    );
    if (!scrollContainer) throw new Error(MISSING_SCROLL_CONTAINER_ERROR);
    expect(scrollContainer.scrollTop).toBe(150);

    rerender(<Harness itemCount={2} anchoredBarOffsetPx={76} />);

    expect(scrollContainer.scrollTop).toBe(74);
  });

  it("reasserts the divider after a loading layout shift with the same item count", () => {
    const { rerender } = render(
      <Harness itemCount={2} anchoredBarOffsetPx={0} scrollLayoutKey="loading" />,
    );
    const scrollContainer = document.querySelector<HTMLElement>(
      `[data-testid="${DIVIDER_SCROLL_CONTAINER_TEST_ID}"]`,
    );
    if (!scrollContainer) throw new Error(MISSING_SCROLL_CONTAINER_ERROR);
    expect(scrollContainer.scrollTop).toBe(150);

    rerender(
      <Harness
        itemCount={2}
        anchoredBarOffsetPx={0}
        scrollLayoutKey="settled"
        dividerDocumentTop={166}
      />,
    );

    expect(scrollContainer.scrollTop).toBe(66);
  });

  it("resynchronizes auto-scroll state after placing the divider", () => {
    const onDividerScroll = vi.fn();

    render(<Harness itemCount={2} anchoredBarOffsetPx={0} onDividerScroll={onDividerScroll} />);

    expect(onDividerScroll).toHaveBeenCalledTimes(1);
  });

  it("does not follow the bottom when work starts with an unread divider", () => {
    const { rerender } = render(<AutoScrollHarness isWorking={false} hasUnreadDivider={true} />);
    const scrollContainer = document.querySelector<HTMLElement>(
      `[data-testid="${AUTO_SCROLL_CONTAINER_TEST_ID}"]`,
    );
    if (!scrollContainer) throw new Error("auto-scroll container did not render");
    setScrollMetrics(scrollContainer);
    scrollContainer.scrollTop = 123;

    rerender(<AutoScrollHarness isWorking={true} hasUnreadDivider={true} />);

    expect(scrollContainer.scrollTop).toBe(123);
  });

  it("does not follow appended messages after the divider scroll marks the reader away from bottom", () => {
    const markRef: { current?: () => void } = {};
    const { rerender } = render(
      <AutoScrollHarness isWorking={false} hasUnreadDivider={true} markRef={markRef} />,
    );
    const scrollContainer = document.querySelector<HTMLElement>(
      `[data-testid="${AUTO_SCROLL_CONTAINER_TEST_ID}"]`,
    );
    if (!scrollContainer) throw new Error("auto-scroll container did not render");
    setScrollMetrics(scrollContainer);
    scrollContainer.scrollTop = 123;
    markRef.current?.();

    rerender(
      <AutoScrollHarness
        isWorking={false}
        hasUnreadDivider={true}
        messages={[...TEST_MESSAGES, {} as Message]}
        markRef={markRef}
      />,
    );

    expect(scrollContainer.scrollTop).toBe(123);
  });

  it("uses a WebKit-safe maximum offset for pinned appends", () => {
    const { rerender } = render(
      <AutoScrollHarness isWorking={false} hasUnreadDivider={false} messages={TEST_MESSAGES} />,
    );
    const scrollContainer = screen.getByTestId(AUTO_SCROLL_CONTAINER_TEST_ID);
    let scrollTop = 600;
    let writes = 0;
    Object.defineProperties(scrollContainer, {
      scrollHeight: {
        configurable: true,
        get: () => {
          throw new Error("pinned append must not read scrollHeight");
        },
      },
      clientHeight: { configurable: true, value: 400 },
      scrollTop: {
        configurable: true,
        get: () => scrollTop,
        set: (value: number) => {
          writes += 1;
          scrollTop = value;
        },
      },
    });

    expect(() => {
      rerender(
        <AutoScrollHarness
          isWorking={false}
          hasUnreadDivider={false}
          messages={[...TEST_MESSAGES, {} as Message]}
        />,
      );
    }).not.toThrow();
    expect(writes).toBe(1);
    expect(scrollTop).toBe(2_147_483_647);
  });

  it("restores the disabled offset after a transient layout clamp", () => {
    const { rerender } = render(
      <AutoScrollHarness isWorking={false} hasUnreadDivider={false} enabled />,
    );
    const scrollContainer = document.querySelector<HTMLElement>(
      `[data-testid="${AUTO_SCROLL_CONTAINER_TEST_ID}"]`,
    );
    if (!scrollContainer) throw new Error("auto-scroll container did not render");
    setScrollMetrics(scrollContainer);
    scrollContainer.scrollTop = 600;

    rerender(<AutoScrollHarness isWorking={false} hasUnreadDivider={false} enabled={false} />);
    // Model the browser temporarily reducing the maximum scroll offset while
    // the composer clears, before the appended transcript row is committed.
    scrollContainer.scrollTop = 568;
    rerender(
      <AutoScrollHarness
        isWorking
        hasUnreadDivider={false}
        enabled={false}
        messages={[...TEST_MESSAGES, {} as Message]}
      />,
    );

    expect(scrollContainer.scrollTop).toBe(600);
  });

  it("persists the last visible disabled offset through hide, unmount, and remount", () => {
    const metrics = { scrollHeight: 1000, scrollTop: 300, clientHeight: 400 };
    const sessionId = "session-1";
    const { rerender, unmount } = render(
      <AutoScrollHarness
        isWorking={false}
        hasUnreadDivider={false}
        enabled={false}
        isVisible
        sessionId={sessionId}
        metrics={metrics}
      />,
    );
    const scrollContainer = screen.getByTestId(AUTO_SCROLL_CONTAINER_TEST_ID);
    metrics.scrollTop = 275;
    scrollContainer.dispatchEvent(new Event("scroll"));

    rerender(
      <AutoScrollHarness
        isWorking={false}
        hasUnreadDivider={false}
        enabled={false}
        isVisible={false}
        sessionId={sessionId}
        metrics={metrics}
      />,
    );
    unmount();

    expect(transcriptScrollTopWrites).toHaveBeenLastCalledWith(sessionId, 275);

    metrics.scrollTop = 0;
    render(
      <NativeScrollManagementHarness
        items={[transcriptMessage("restored-message")]}
        metrics={metrics}
        sessionId={sessionId}
        enabled={false}
      />,
    );

    expect(metrics.scrollTop).toBe(275);
  });

  it("does not write hidden appended content and catches up after activation", () => {
    const frames: Array<FrameRequestCallback> = [];
    vi.stubGlobal("requestAnimationFrame", (callback: FrameRequestCallback) => {
      frames.push(callback);
      return frames.length;
    });
    const metrics = { scrollHeight: 1000, scrollTop: 600, clientHeight: 400 };
    try {
      const { rerender } = render(
        <AutoScrollHarness isWorking={false} hasUnreadDivider={false} metrics={metrics} />,
      );
      const scrollContainer = screen.getByTestId(AUTO_SCROLL_CONTAINER_TEST_ID);
      metrics.scrollTop = 600;

      rerender(
        <AutoScrollHarness
          isWorking={false}
          hasUnreadDivider={false}
          messages={[...TEST_MESSAGES, {} as Message]}
          isVisible={false}
          metrics={metrics}
        />,
      );
      expect(scrollContainer.scrollTop).toBe(600);

      rerender(
        <AutoScrollHarness
          isWorking={false}
          hasUnreadDivider={false}
          messages={[...TEST_MESSAGES, {} as Message]}
          isVisible
          metrics={metrics}
        />,
      );
      metrics.scrollTop = 600;
      act(() => {
        for (let frame = frames.shift(); frame; frame = frames.shift()) frame(0);
      });

      expect(scrollContainer.scrollTop).toBe(2_147_483_647);
    } finally {
      vi.unstubAllGlobals();
    }
  });

  it.each([
    { label: "disabled", enabled: false },
    { label: "manually away while enabled", enabled: true },
  ])("preserves the $label reader position on activation", ({ enabled }) => {
    const frames: Array<FrameRequestCallback> = [];
    vi.stubGlobal("requestAnimationFrame", (callback: FrameRequestCallback) => {
      frames.push(callback);
      return frames.length;
    });
    const metrics = { scrollHeight: 1000, scrollTop: 300, clientHeight: 400 };
    try {
      const markRef: { current?: () => void } = {};
      const { rerender } = render(
        <AutoScrollHarness
          isWorking={false}
          hasUnreadDivider={false}
          enabled={enabled}
          markRef={markRef}
          metrics={metrics}
        />,
      );
      const scrollContainer = screen.getByTestId(AUTO_SCROLL_CONTAINER_TEST_ID);
      metrics.scrollTop = 300;
      scrollContainer.dispatchEvent(new Event("scroll"));
      if (!enabled) markRef.current?.();

      rerender(
        <AutoScrollHarness
          isWorking={false}
          hasUnreadDivider={false}
          enabled={enabled}
          markRef={markRef}
          isVisible={false}
          messages={[...TEST_MESSAGES, {} as Message]}
          metrics={metrics}
        />,
      );
      metrics.scrollTop = 300;
      rerender(
        <AutoScrollHarness
          isWorking={false}
          hasUnreadDivider={false}
          enabled={enabled}
          markRef={markRef}
          isVisible
          messages={[...TEST_MESSAGES, {} as Message]}
          metrics={metrics}
        />,
      );
      metrics.scrollTop = 300;
      act(() => {
        for (const frame of frames.splice(0)) frame(0);
      });

      expect(scrollContainer.scrollTop).toBe(300);
    } finally {
      vi.unstubAllGlobals();
    }
  });

  it("never re-scrolls once the reader has started scrolling, even if the anchored bar's height changes afterward", () => {
    const { rerender, container } = render(<Harness itemCount={2} anchoredBarOffsetPx={0} />);
    const scrollContainer = container.querySelector<HTMLElement>(
      `[data-testid="${DIVIDER_SCROLL_CONTAINER_TEST_ID}"]`,
    );
    if (!scrollContainer) throw new Error(MISSING_SCROLL_CONTAINER_ERROR);
    expect(scrollContainer.scrollTop).toBe(150);

    scrollContainer.dispatchEvent(new Event("wheel", { bubbles: true }));

    rerender(<Harness itemCount={2} anchoredBarOffsetPx={76} />);

    expect(scrollContainer.scrollTop).toBe(150);
  });

  it("never scrolls the divider when there is no unread boundary, regardless of anchored-bar height changes", () => {
    const { rerender } = render(
      <Harness itemCount={2} anchoredBarOffsetPx={0} dividerKey={null} />,
    );
    const scrollContainer = document.querySelector<HTMLElement>(
      `[data-testid="${DIVIDER_SCROLL_CONTAINER_TEST_ID}"]`,
    );
    if (!scrollContainer) throw new Error(MISSING_SCROLL_CONTAINER_ERROR);
    expect(scrollContainer.scrollTop).toBe(0);

    rerender(<Harness itemCount={2} anchoredBarOffsetPx={76} dividerKey={null} />);

    expect(scrollContainer.scrollTop).toBe(0);
  });

  it("stops re-scrolling once the settling window has elapsed, even without any user interaction", () => {
    vi.useFakeTimers();

    const { rerender } = render(<Harness itemCount={2} anchoredBarOffsetPx={0} />);
    const scrollContainer = document.querySelector<HTMLElement>(
      `[data-testid="${DIVIDER_SCROLL_CONTAINER_TEST_ID}"]`,
    );
    if (!scrollContainer) throw new Error(MISSING_SCROLL_CONTAINER_ERROR);
    expect(scrollContainer.scrollTop).toBe(150);

    // Past the 4s settling window (e.g. a scrollbar drag with no
    // wheel/touch/key event to catch — the correction must freeze anyway).
    vi.advanceTimersByTime(4001);
    rerender(<Harness itemCount={2} anchoredBarOffsetPx={76} />);

    expect(scrollContainer.scrollTop).toBe(150);
    vi.useRealTimers();
  });
});

type ScrollToMessageHandle = (
  messageId: string,
  options?: { align?: "start" | "center" },
) => boolean;

/** Renders a scroll root containing the given message rows and reports the useScrollToMessage handle. */
function ScrollToMessageHarness({
  rows,
  onHandle,
}: {
  rows: string[];
  onHandle: (handle: ScrollToMessageHandle) => void;
}) {
  const scrollRef = useRef<HTMLDivElement>(null);
  const handle = useScrollToMessage(scrollRef, (performScroll) => performScroll());
  useLayoutEffect(() => {
    onHandle(handle);
  }, [handle, onHandle]);
  return (
    <div ref={scrollRef} data-testid={SCROLL_TO_MESSAGE_ROOT}>
      {rows.map((id) => (
        <div key={id} id={`msg-${id}`} />
      ))}
    </div>
  );
}

describe("useScrollToMessage — root-scoped row lookup", () => {
  let scrollIntoView: ReturnType<typeof vi.spyOn>;
  let scrollReceivers: HTMLElement[];

  beforeEach(() => {
    scrollReceivers = [];
    scrollIntoView = vi.spyOn(HTMLElement.prototype, "scrollIntoView").mockImplementation(function (
      this: HTMLElement,
    ) {
      scrollReceivers.push(this);
    });
  });

  afterEach(() => {
    scrollIntoView.mockRestore();
  });

  /** Renders a ScrollToMessageHarness for the given rows and returns its scroll handle. */
  function renderHandle(rows: string[]) {
    const boxed: { current: ScrollToMessageHandle | null } = { current: null };
    render(
      <ScrollToMessageHarness
        rows={rows}
        onHandle={(next) => {
          boxed.current = next;
        }}
      />,
    );
    const handle = boxed.current;
    if (!handle) throw new Error("scroll handle did not render");
    return handle;
  }

  it("returns false and does not scroll when the row is absent from the owning root", () => {
    const handle = renderHandle(["other"]);

    expect(handle("missing")).toBe(false);
    expect(scrollIntoView).not.toHaveBeenCalled();
  });

  it("returns true and scrolls the row into view with the requested alignment", () => {
    const handle = renderHandle([TARGET_MESSAGE_ID]);

    expect(handle(TARGET_MESSAGE_ID, { align: "start" })).toBe(true);
    expect(scrollIntoView).toHaveBeenCalledWith({
      block: "start",
      behavior: "smooth",
    });

    scrollIntoView.mockClear();
    expect(handle(TARGET_MESSAGE_ID)).toBe(true);
    expect(scrollIntoView).toHaveBeenCalledWith({
      block: "center",
      behavior: "smooth",
    });
  });

  it("resolves and scrolls only the matching row when duplicate ids exist across mounted lists", () => {
    const firstHandle: { current: ScrollToMessageHandle | null } = { current: null };
    const secondHandle: { current: ScrollToMessageHandle | null } = { current: null };
    const results = render(
      <>
        <ScrollToMessageHarness
          rows={["dup"]}
          onHandle={(next) => {
            firstHandle.current = next;
          }}
        />
        <ScrollToMessageHarness
          rows={["dup"]}
          onHandle={(next) => {
            secondHandle.current = next;
          }}
        />
      </>,
    );
    const h1 = firstHandle.current;
    const h2 = secondHandle.current;
    if (!h1 || !h2) throw new Error("handles did not render");
    const rows = results.container.querySelectorAll("#msg-dup");

    expect(h1("dup")).toBe(true);
    expect(h2("dup")).toBe(true);
    // Each handle scrolled its own list's row. A document-global
    // implementation would resolve both handles to the SAME (first) row and
    // still call scrollIntoView twice — pin the receivers instead: the first
    // handle's target must live in the first root and the second handle's in
    // the second root.
    const roots = results.container.querySelectorAll(`[data-testid="${SCROLL_TO_MESSAGE_ROOT}"]`);
    expect(roots).toHaveLength(2);
    expect(scrollIntoView).toHaveBeenCalledTimes(2);
    expect(scrollReceivers).toHaveLength(2);
    expect(roots[0].contains(scrollReceivers[0])).toBe(true);
    expect(roots[0].contains(scrollReceivers[1])).toBe(false);
    expect(roots[1].contains(scrollReceivers[1])).toBe(true);
    expect(roots[1].contains(scrollReceivers[0])).toBe(false);
    expect(rows).toHaveLength(2);
  });
});

// eslint-disable-next-line max-lines-per-function -- keeps both canceled-scroll regressions together.
describe("useScrollToMessage — canceled-scroll landing", () => {
  it("force-lands the alignment when the smooth scroll is canceled without moving", () => {
    // Round-12 regression: the verifier must NOT exit on the first frame of
    // movement (or no movement). A dockview restore that cancels the smooth
    // scroll leaves the row misaligned; the verifier watches a bounded frame
    // window and lands the requested alignment.
    const frames: Array<() => void> = [];
    vi.stubGlobal("requestAnimationFrame", (callback: () => void) => {
      frames.push(callback);
      return frames.length;
    });
    vi.stubGlobal("cancelAnimationFrame", () => {});
    const scrollIntoView = vi
      .spyOn(HTMLElement.prototype, "scrollIntoView")
      .mockImplementation(() => {});
    try {
      const boxedHandle: { current: ScrollToMessageHandle | null } = { current: null };
      const { container } = render(
        <ScrollToMessageHarness
          rows={[TARGET_MESSAGE_ID]}
          onHandle={(next) => {
            boxedHandle.current = next;
          }}
        />,
      );
      const handle = boxedHandle.current;
      if (!handle) throw new Error(HANDLE_RENDER_ERROR);
      const root = container.querySelector<HTMLElement>(
        `[data-testid="${SCROLL_TO_MESSAGE_ROOT}"]`,
      );
      const target = container.querySelector(`#msg-${TARGET_MESSAGE_ID}`);
      if (!root || !target) throw new Error(HARNESS_RENDER_ERROR);
      Object.defineProperty(root, "scrollTop", {
        configurable: true,
        writable: true,
        value: 0,
      });
      Object.defineProperty(root, "getBoundingClientRect", {
        configurable: true,
        value: () => createRect(0, 400),
      });
      // The target sits 120px below the scrollport top and moves with scroll.
      Object.defineProperty(target, "getBoundingClientRect", {
        configurable: true,
        value: () => createRect(120 - root.scrollTop, 20),
      });

      expect(handle(TARGET_MESSAGE_ID, { align: "start" })).toBe(true);

      // The smooth scroll never moves the container on its own; the verifier
      // must land the alignment within its bounded window.
      for (let i = 0; i < 10; i += 1) {
        act(() => {
          const pending = frames.splice(0);
          for (const frame of pending) frame();
        });
      }
      expect(root.scrollTop).toBe(120);
    } finally {
      vi.unstubAllGlobals();
      scrollIntoView.mockRestore();
    }
  });

  it("does not correct a centered row that is offset by scroll margin", () => {
    const frames: Array<() => void> = [];
    vi.stubGlobal("requestAnimationFrame", (callback: () => void) => {
      frames.push(callback);
      return frames.length;
    });
    vi.stubGlobal("cancelAnimationFrame", () => {});
    const scrollIntoView = vi
      .spyOn(HTMLElement.prototype, "scrollIntoView")
      .mockImplementation(() => {});
    vi.spyOn(window, "getComputedStyle").mockReturnValue({
      scrollMarginTop: "20px",
    } as CSSStyleDeclaration);
    try {
      const boxedHandle: { current: ScrollToMessageHandle | null } = { current: null };
      const { container } = render(
        <ScrollToMessageHarness
          rows={[TARGET_MESSAGE_ID]}
          onHandle={(next) => {
            boxedHandle.current = next;
          }}
        />,
      );
      const handle = boxedHandle.current;
      if (!handle) throw new Error(HANDLE_RENDER_ERROR);
      const root = container.querySelector<HTMLElement>(
        `[data-testid="${SCROLL_TO_MESSAGE_ROOT}"]`,
      );
      const target = container.querySelector(`#msg-${TARGET_MESSAGE_ID}`);
      if (!root || !target) throw new Error(HARNESS_RENDER_ERROR);
      let scrollTop = 0;
      let writes = 0;
      Object.defineProperties(root, {
        scrollTop: {
          configurable: true,
          get: () => scrollTop,
          set: (value: number) => {
            writes += 1;
            scrollTop = value;
          },
        },
        scrollHeight: { configurable: true, value: 1000 },
        clientHeight: { configurable: true, value: 400 },
        getBoundingClientRect: {
          configurable: true,
          value: () => createRect(0, 400),
        },
      });
      Object.defineProperty(target, "getBoundingClientRect", {
        configurable: true,
        // Center alignment with a 20px scroll margin places the row center
        // 10px below the viewport center.
        value: () => createRect(200, 20),
      });

      expect(handle(TARGET_MESSAGE_ID)).toBe(true);
      for (let i = 0; i < 2; i += 1) {
        act(() => {
          const pending = frames.splice(0);
          for (const frame of pending) frame();
        });
      }

      expect(writes).toBe(0);
      expect(scrollTop).toBe(0);
    } finally {
      vi.unstubAllGlobals();
      scrollIntoView.mockRestore();
    }
  });

  it("accepts the nearest reachable position when start alignment is beyond the scroll range", () => {
    const frames: Array<() => void> = [];
    vi.stubGlobal("requestAnimationFrame", (callback: () => void) => {
      frames.push(callback);
      return frames.length;
    });
    vi.stubGlobal("cancelAnimationFrame", () => {});
    const scrollIntoView = vi
      .spyOn(HTMLElement.prototype, "scrollIntoView")
      .mockImplementation(() => {});
    try {
      const boxedHandle: { current: ScrollToMessageHandle | null } = { current: null };
      const { container } = render(
        <ScrollToMessageHarness
          rows={[TARGET_MESSAGE_ID]}
          onHandle={(next) => {
            boxedHandle.current = next;
          }}
        />,
      );
      const handle = boxedHandle.current;
      if (!handle) throw new Error(HANDLE_RENDER_ERROR);
      const root = container.querySelector<HTMLElement>(
        `[data-testid="${SCROLL_TO_MESSAGE_ROOT}"]`,
      );
      const target = container.querySelector(`#msg-${TARGET_MESSAGE_ID}`);
      if (!root || !target) throw new Error(HARNESS_RENDER_ERROR);
      let scrollTop = 0;
      Object.defineProperties(root, {
        scrollTop: {
          configurable: true,
          get: () => scrollTop,
          set: (value: number) => {
            scrollTop = Math.min(value, 100);
          },
        },
        scrollHeight: { configurable: true, value: 500 },
        clientHeight: { configurable: true, value: 400 },
        getBoundingClientRect: {
          configurable: true,
          value: () => createRect(0, 400),
        },
      });
      Object.defineProperty(target, "getBoundingClientRect", {
        configurable: true,
        value: () => createRect(180 - scrollTop, 20),
      });

      expect(handle(TARGET_MESSAGE_ID, { align: "start" })).toBe(true);
      act(() => frames.splice(0).forEach((frame) => frame()));
      act(() => frames.splice(0).forEach((frame) => frame()));
      act(() => frames.splice(0).forEach((frame) => frame()));
      act(() => frames.splice(0).forEach((frame) => frame()));

      expect(scrollTop).toBe(100);
      expect(frames).toHaveLength(0);
    } finally {
      vi.unstubAllGlobals();
      scrollIntoView.mockRestore();
    }
  });
});

describe("useScrollToMessage — superseded verifiers", () => {
  it("lets a newer scroll supersede an in-flight verifier without stale force-landing", () => {
    // Round-14 regression: a verifier scheduled by prompt A must not force-
    // land the transcript after a newer prompt B consumed. The generation
    // guard makes A's callback bail; B's lands the target.
    const frames: Array<() => void> = [];
    vi.stubGlobal("requestAnimationFrame", (callback: () => void) => {
      frames.push(callback);
      return frames.length;
    });
    vi.stubGlobal("cancelAnimationFrame", () => {});
    const scrollIntoView = vi
      .spyOn(HTMLElement.prototype, "scrollIntoView")
      .mockImplementation(() => {});
    try {
      const boxedHandle: { current: ScrollToMessageHandle | null } = { current: null };
      const { container } = render(
        <ScrollToMessageHarness
          rows={["target-a", "target-b"]}
          onHandle={(next) => {
            boxedHandle.current = next;
          }}
        />,
      );
      const handle = boxedHandle.current;
      if (!handle) throw new Error(HANDLE_RENDER_ERROR);
      const root = container.querySelector<HTMLElement>(
        `[data-testid="${SCROLL_TO_MESSAGE_ROOT}"]`,
      );
      const targetA = container.querySelector("#msg-target-a");
      const targetB = container.querySelector("#msg-target-b");
      if (!root || !targetA || !targetB) throw new Error(HARNESS_RENDER_ERROR);
      Object.defineProperty(root, "scrollTop", {
        configurable: true,
        writable: true,
        value: 0,
      });
      Object.defineProperty(root, "getBoundingClientRect", {
        configurable: true,
        value: () => createRect(0, 400),
      });
      // A sits at 120px, B at 240px below the scrollport top.
      Object.defineProperty(targetA, "getBoundingClientRect", {
        configurable: true,
        value: () => createRect(120 - root.scrollTop, 20),
      });
      Object.defineProperty(targetB, "getBoundingClientRect", {
        configurable: true,
        value: () => createRect(240 - root.scrollTop, 20),
      });

      expect(handle("target-a", { align: "start" })).toBe(true);
      // B supersedes A before A's verifier ever lands.
      expect(handle("target-b", { align: "start" })).toBe(true);

      for (let i = 0; i < 15; i += 1) {
        act(() => {
          const pending = frames.splice(0);
          for (const frame of pending) frame();
        });
      }
      // The transcript lands on B (240), never on the stale A (120).
      expect(root.scrollTop).toBe(240);
    } finally {
      vi.unstubAllGlobals();
      scrollIntoView.mockRestore();
    }
  });
});

describe("useScrollToMessage — absent superseder", () => {
  it("invalidates an in-flight verifier even when the superseding row is absent", () => {
    // Round-15 regression: a newer scrollToMessage whose row is not rendered
    // yet returns false but must still advance the generation, so A's
    // in-flight verifier can never force-land on the stale prompt.
    const frames: Array<() => void> = [];
    vi.stubGlobal("requestAnimationFrame", (callback: () => void) => {
      frames.push(callback);
      return frames.length;
    });
    vi.stubGlobal("cancelAnimationFrame", () => {});
    const scrollIntoView = vi
      .spyOn(HTMLElement.prototype, "scrollIntoView")
      .mockImplementation(() => {});
    try {
      const boxedHandle: { current: ScrollToMessageHandle | null } = { current: null };
      const { container } = render(
        <ScrollToMessageHarness
          rows={["target-a"]}
          onHandle={(next) => {
            boxedHandle.current = next;
          }}
        />,
      );
      const handle = boxedHandle.current;
      if (!handle) throw new Error(HANDLE_RENDER_ERROR);
      const root = container.querySelector<HTMLElement>(
        `[data-testid="${SCROLL_TO_MESSAGE_ROOT}"]`,
      );
      const targetA = container.querySelector("#msg-target-a");
      if (!root || !targetA) throw new Error(HARNESS_RENDER_ERROR);
      Object.defineProperty(root, "scrollTop", {
        configurable: true,
        writable: true,
        value: 0,
      });
      Object.defineProperty(root, "getBoundingClientRect", {
        configurable: true,
        value: () => createRect(0, 400),
      });
      Object.defineProperty(targetA, "getBoundingClientRect", {
        configurable: true,
        value: () => createRect(120 - root.scrollTop, 20),
      });

      expect(handle("target-a", { align: "start" })).toBe(true);
      // The superseding target row is absent: the call returns false but
      // must invalidate A's pending verifier.
      expect(handle("missing-b", { align: "start" })).toBe(false);

      for (let i = 0; i < 10; i += 1) {
        act(() => {
          const pending = frames.splice(0);
          for (const frame of pending) frame();
        });
      }
      expect(root.scrollTop).toBe(0);
    } finally {
      vi.unstubAllGlobals();
      scrollIntoView.mockRestore();
    }
  });
});
