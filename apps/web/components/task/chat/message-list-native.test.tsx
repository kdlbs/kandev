import { useLayoutEffect, useRef } from "react";
import { cleanup, render } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { Message } from "@/lib/types/http";

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
import { useAutoScroll } from "./message-list-native-scroll";

const DIVIDER_KEY = "m2";
const DIVIDER_SCROLL_CONTAINER_TEST_ID = "divider-scroll-container";
const MISSING_SCROLL_CONTAINER_ERROR = "scroll container did not render";
const TEST_MESSAGES = [{} as Message];
const NEVER_LOCKED = () => false;

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

function AutoScrollHarness({
  isWorking,
  hasUnreadDivider,
  messages = TEST_MESSAGES,
  markRef,
}: {
  isWorking: boolean;
  hasUnreadDivider: boolean;
  messages?: Message[];
  markRef?: { current?: () => void };
}) {
  const scrollRef = useRef<HTMLDivElement>(null);
  const { markNotNearBottom } = useAutoScroll({
    scrollRef,
    messages,
    isWorking,
    sessionId: null,
    enabled: true,
    hasUnreadDivider,
    isProgrammaticScrollLocked: NEVER_LOCKED,
  });
  if (markRef) markRef.current = markNotNearBottom;
  return <div ref={scrollRef} data-testid="auto-scroll-container" />;
}

function setScrollMetrics(element: HTMLElement) {
  Object.defineProperty(element, "scrollHeight", { configurable: true, value: 1000 });
  Object.defineProperty(element, "clientHeight", { configurable: true, value: 400 });
}

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
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
