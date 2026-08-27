/* eslint-disable max-lines -- native scroll behavior and regression cases share one harness. */
import { useLayoutEffect, useRef } from "react";
import { act, cleanup, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { Message } from "@/lib/types/http";
import type { RenderItem } from "@/hooks/use-processed-messages";

const sharedSentinelCalls = vi.hoisted(() => [] as unknown[][]);
const sharedSentinelUserGesture = vi.hoisted(() => vi.fn());

vi.mock("@/hooks/use-lazy-load-sentinel", () => ({
  useLazyLoadSentinel: (...args: unknown[]) => {
    sharedSentinelCalls.push(args);
    return { sentinelRef: () => {}, onUserGesture: sharedSentinelUserGesture };
  },
}));

vi.mock("@/lib/state/dockview-store", () => ({
  useDockviewStore: Object.assign(
    (selector: (state: { pendingChatScrollTop: number | null }) => unknown) =>
      selector({ pendingChatScrollTop: null }),
    { getState: () => ({ pendingChatScrollTop: null }) },
  ),
}));

vi.mock("@/components/state-provider", () => ({
  useAppStoreApi: () => ({
    getState: () => ({ setTranscriptScrollTop: vi.fn() }),
  }),
}));

import { useScrollToDividerOrBottom } from "./message-list-native";
import {
  resolvePaginationStopReason,
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
const TEST_MESSAGES = [{} as Message];
/** Always returns false: the harness never locks programmatic scrolling. */
const NEVER_LOCKED = () => false;

/** Renders a scroll container wired to useScrollToDividerOrBottom with mocked divider geometry. */
function Harness({
  itemCount,
  anchoredBarOffsetPx,
  dividerKey = DIVIDER_KEY,
  onDividerScroll,
  scrollLayoutKey = "initial",
  dividerDocumentTop = 250,
}: {
  itemCount: number;
  anchoredBarOffsetPx: number;
  dividerKey?: string | null;
  onDividerScroll?: () => void;
  scrollLayoutKey?: string;
  dividerDocumentTop?: number;
}) {
  const scrollRef = useRef<HTMLDivElement>(null);
  useLayoutEffect(() => {
    const scrollContainer = scrollRef.current;
    if (!scrollContainer) return;
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
  useScrollToDividerOrBottom(scrollRef, itemCount, dividerKey, anchoredBarOffsetPx, {
    onDividerScroll,
    scrollLayoutKey,
  });
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
}: {
  isWorking: boolean;
  hasUnreadDivider: boolean;
  messages?: Message[];
  markRef?: { current?: () => void };
  enabled?: boolean;
}) {
  const scrollRef = useRef<HTMLDivElement>(null);
  const { markNotNearBottom } = useAutoScroll({
    scrollRef,
    messages,
    isWorking,
    sessionId: null,
    enabled,
    hasUnreadDivider,
    isProgrammaticScrollLocked: NEVER_LOCKED,
  });
  if (markRef) markRef.current = markNotNearBottom;
  return <div ref={scrollRef} data-testid="auto-scroll-container" />;
}

/** Stubs scrollHeight (1000) and clientHeight (400) onto an element. */
function setScrollMetrics(element: HTMLElement) {
  Object.defineProperty(element, "scrollHeight", { configurable: true, value: 1000 });
  Object.defineProperty(element, "clientHeight", { configurable: true, value: 400 });
}

afterEach(() => {
  cleanup();
  sharedSentinelCalls.length = 0;
  sharedSentinelUserGesture.mockReset();
  vi.restoreAllMocks();
});

function transcriptMessage(id: string): RenderItem {
  return { type: "message", message: { id } as Message };
}

function transcriptActivity(id: string, turnId = id): RenderItem {
  return { type: "turn_group", id, turnId, messages: [] };
}

function NativeScrollManagementHarness({
  items,
  metrics,
  loadMore = async () => 0,
}: {
  items: RenderItem[];
  metrics?: { scrollHeight: number; scrollTop: number; clientHeight: number };
  loadMore?: () => Promise<number>;
}) {
  const scrollRef = useRef<HTMLDivElement>(null);
  useNativeScrollManagement({
    scrollRef,
    items,
    messages: [],
    isWorking: false,
    sessionId: null,
    enabled: false,
    hasUnreadDivider: false,
    messagesLoading: false,
    hasMore: true,
    isLoadingMore: false,
    loadMore,
  });
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
  }, [metrics]);
  return <div data-testid="native-scroll-management-container" ref={scrollRef} />;
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

describe("useNativeScrollManagement transcript pagination", () => {
  it("retries a disarmed short page on the next upward scroll", () => {
    const metrics = { scrollHeight: 1000, scrollTop: 100, clientHeight: 400 };
    render(<NativeScrollManagementHarness items={[]} metrics={metrics} />);
    const scroller = screen.getByTestId("native-scroll-management-container");

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

  it("re-arms the native sentinel with a visible-boundary continuation decision", () => {
    render(<NativeScrollManagementHarness items={[]} />);

    const options = sharedSentinelCalls[0]?.[5];
    expect(options).toEqual({
      rearmWhileIntersecting: true,
      shouldContinueWhileIntersecting: expect.any(Function),
      onLoadSettled: expect.any(Function),
    });
  });

  it("continues only while an older page keeps the same visible boundary", async () => {
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
    expect(options.shouldContinueWhileIntersecting()).toBe(false);
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
      screen.getByTestId("native-scroll-management-container").dispatchEvent(new Event("scroll"));
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
      screen.getByTestId("native-scroll-management-container").dispatchEvent(new Event("scroll"));
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
});

// eslint-disable-next-line max-lines-per-function -- this suite keeps the related scroll invariants together.
describe("useScrollToDividerOrBottom — anchored-bar offset", () => {
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
      '[data-testid="auto-scroll-container"]',
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
      '[data-testid="auto-scroll-container"]',
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

  it("restores the disabled offset after a transient layout clamp", () => {
    const { rerender } = render(
      <AutoScrollHarness isWorking={false} hasUnreadDivider={false} enabled />,
    );
    const scrollContainer = document.querySelector<HTMLElement>(
      '[data-testid="auto-scroll-container"]',
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
