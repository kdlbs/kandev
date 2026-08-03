import { useRef } from "react";
import { render } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

vi.mock("@/lib/state/dockview-store", () => ({
  useDockviewStore: Object.assign(
    (selector: (state: { pendingChatScrollTop: number | null }) => unknown) =>
      selector({ pendingChatScrollTop: null }),
    { getState: () => ({ pendingChatScrollTop: null }) },
  ),
}));

import { useScrollToDividerOrBottom } from "./message-list-native";

const DIVIDER_KEY = "m2";

function Harness({
  itemCount,
  anchoredBarOffsetPx,
  dividerKey = DIVIDER_KEY,
}: {
  itemCount: number;
  anchoredBarOffsetPx: number;
  dividerKey?: string | null;
}) {
  const scrollRef = useRef<HTMLDivElement>(null);
  useScrollToDividerOrBottom(scrollRef, itemCount, dividerKey, anchoredBarOffsetPx);
  return (
    <div ref={scrollRef}>
      <div id="msg-m1" />
      <div id={`msg-${DIVIDER_KEY}`} />
    </div>
  );
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
