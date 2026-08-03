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
}: {
  itemCount: number;
  anchoredBarOffsetPx: number;
}) {
  const scrollRef = useRef<HTMLDivElement>(null);
  useScrollToDividerOrBottom(scrollRef, itemCount, DIVIDER_KEY, anchoredBarOffsetPx);
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
});
