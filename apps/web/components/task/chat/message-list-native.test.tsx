import { useRef } from "react";
import { render } from "@testing-library/react";
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
const TEST_MESSAGES = [{} as Message];
const NEVER_LOCKED = () => false;

function Harness({
  itemCount,
  anchoredBarOffsetPx,
  dividerKey = DIVIDER_KEY,
  onDividerScroll,
}: {
  itemCount: number;
  anchoredBarOffsetPx: number;
  dividerKey?: string | null;
  onDividerScroll?: () => void;
}) {
  const scrollRef = useRef<HTMLDivElement>(null);
  useScrollToDividerOrBottom(
    scrollRef,
    itemCount,
    dividerKey,
    anchoredBarOffsetPx,
    onDividerScroll,
  );
  return (
    <div ref={scrollRef}>
      <div id="msg-m1" />
      <div id={`msg-${DIVIDER_KEY}`} />
    </div>
  );
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
  vi.restoreAllMocks();
});

describe("useScrollToDividerOrBottom — anchored-bar offset", () => {
  it("re-scrolls the divider into view once the anchored bar's measured height arrives, so it lands below the pinned bar instead of under it", () => {
    const scrollIntoView = vi.fn();
    HTMLElement.prototype.scrollIntoView = scrollIntoView;

    const { rerender } = render(<Harness itemCount={2} anchoredBarOffsetPx={0} />);
    expect(scrollIntoView).toHaveBeenCalledTimes(1);

    // The anchored bar's real height resolves a tick after mount (async
    // ResizeObserver report) — the divider must be re-placed now that the
    // scroll-margin backing it is no longer 0, or it stays hidden under the
    // bar for good (didScrollToDivider already latched true).
    rerender(<Harness itemCount={2} anchoredBarOffsetPx={76} />);

    expect(scrollIntoView).toHaveBeenCalledTimes(2);
    expect(scrollIntoView).toHaveBeenLastCalledWith({ block: "start" });
  });

  it("resynchronizes auto-scroll state after placing the divider", () => {
    HTMLElement.prototype.scrollIntoView = vi.fn();
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
    const scrollIntoView = vi.fn();
    HTMLElement.prototype.scrollIntoView = scrollIntoView;

    const { rerender, container } = render(<Harness itemCount={2} anchoredBarOffsetPx={0} />);
    expect(scrollIntoView).toHaveBeenCalledTimes(1);

    container.querySelector("div")?.dispatchEvent(new Event("wheel", { bubbles: true }));

    rerender(<Harness itemCount={2} anchoredBarOffsetPx={76} />);

    expect(scrollIntoView).toHaveBeenCalledTimes(1);
  });

  it("never scrolls the divider when there is no unread boundary, regardless of anchored-bar height changes", () => {
    const scrollIntoView = vi.fn();
    HTMLElement.prototype.scrollIntoView = scrollIntoView;

    const { rerender } = render(
      <Harness itemCount={2} anchoredBarOffsetPx={0} dividerKey={null} />,
    );
    expect(scrollIntoView).not.toHaveBeenCalled();

    rerender(<Harness itemCount={2} anchoredBarOffsetPx={76} dividerKey={null} />);

    expect(scrollIntoView).not.toHaveBeenCalled();
  });

  it("stops re-scrolling once the settling window has elapsed, even without any user interaction", () => {
    vi.useFakeTimers();
    const scrollIntoView = vi.fn();
    HTMLElement.prototype.scrollIntoView = scrollIntoView;

    const { rerender } = render(<Harness itemCount={2} anchoredBarOffsetPx={0} />);
    expect(scrollIntoView).toHaveBeenCalledTimes(1);

    // Past the 4s settling window (e.g. a scrollbar drag with no
    // wheel/touch/key event to catch — the correction must freeze anyway).
    vi.advanceTimersByTime(4001);
    rerender(<Harness itemCount={2} anchoredBarOffsetPx={76} />);

    expect(scrollIntoView).toHaveBeenCalledTimes(1);
    vi.useRealTimers();
  });
});
