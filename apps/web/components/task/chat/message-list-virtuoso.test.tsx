import { render } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { RenderItem } from "@/hooks/use-processed-messages";
import type { Message } from "@/lib/types/http";
import type { VirtuosoHandle } from "react-virtuoso";

import { useScrollToDividerOnceResolved } from "./message-list-virtuoso";

const items: RenderItem[] = [
  { type: "message", message: { id: "m1" } as Message },
  { type: "message", message: { id: "m2" } as Message },
];
const itemsWithLiveMessage: RenderItem[] = [
  ...items,
  { type: "message", message: { id: "m3" } as Message },
];
const DIVIDER_KEY = "m2";

afterEach(() => {
  vi.restoreAllMocks();
});

function makeHarness(
  virtuosoRef: { current: VirtuosoHandle },
  scrollParent: HTMLDivElement,
  dividerKey: string | null = DIVIDER_KEY,
  runLocked: (performScroll: () => void) => void = (performScroll) => performScroll(),
) {
  return function DirectHarness({
    offsetPx,
    renderItems = items,
  }: {
    offsetPx: number;
    renderItems?: RenderItem[];
  }) {
    useScrollToDividerOnceResolved(virtuosoRef, renderItems, 0, dividerKey, {
      offsetPx,
      scrollParent,
      runLocked,
    });
    return null;
  };
}

it("runs the reassert scroll through runLocked so followOutput can't fight a live-update reassertion", () => {
  let insideLock = false;
  let scrolledInsideLock: boolean | null = null;
  const scrollToIndex = vi.fn(() => {
    scrolledInsideLock = insideLock;
  });
  const virtuosoRef = { current: { scrollToIndex } as unknown as VirtuosoHandle };
  const runLocked = vi.fn((performScroll: () => void) => {
    insideLock = true;
    performScroll();
    insideLock = false;
  });
  const DirectHarness = makeHarness(
    virtuosoRef,
    document.createElement("div"),
    DIVIDER_KEY,
    runLocked,
  );

  render(<DirectHarness offsetPx={76} />);

  expect(runLocked).toHaveBeenCalledTimes(1);
  expect(scrollToIndex).toHaveBeenCalledTimes(1);
  expect(scrolledInsideLock).toBe(true);
});

describe("useScrollToDividerOnceResolved — anchored-bar offset", () => {
  it("negatively offsets the divider scroll by the anchored bar's height so the item lands below the pinned bar", () => {
    const scrollToIndex = vi.fn();
    const virtuosoRef = { current: { scrollToIndex } as unknown as VirtuosoHandle };
    const DirectHarness = makeHarness(virtuosoRef, document.createElement("div"));

    render(<DirectHarness offsetPx={76} />);

    expect(scrollToIndex).toHaveBeenCalledWith({ index: 1, align: "start", offset: -76 });
  });

  it("re-scrolls once the anchored bar's measured height arrives after the initial placement", () => {
    const scrollToIndex = vi.fn();
    const virtuosoRef = { current: { scrollToIndex } as unknown as VirtuosoHandle };
    const DirectHarness = makeHarness(virtuosoRef, document.createElement("div"));

    const { rerender } = render(<DirectHarness offsetPx={0} />);
    expect(scrollToIndex).toHaveBeenCalledTimes(1);
    expect(scrollToIndex).toHaveBeenLastCalledWith({ index: 1, align: "start", offset: 0 });

    rerender(<DirectHarness offsetPx={76} />);

    expect(scrollToIndex).toHaveBeenCalledTimes(2);
    expect(scrollToIndex).toHaveBeenLastCalledWith({ index: 1, align: "start", offset: -76 });
  });

  it("never re-scrolls once the reader has started scrolling, even if the anchored bar's height changes afterward", () => {
    const scrollToIndex = vi.fn();
    const virtuosoRef = { current: { scrollToIndex } as unknown as VirtuosoHandle };
    const scrollParent = document.createElement("div");
    document.body.appendChild(scrollParent);
    const DirectHarness = makeHarness(virtuosoRef, scrollParent);

    const { rerender } = render(<DirectHarness offsetPx={0} />);
    expect(scrollToIndex).toHaveBeenCalledTimes(1);

    scrollParent.dispatchEvent(new Event("wheel", { bubbles: true }));

    rerender(<DirectHarness offsetPx={76} />);

    expect(scrollToIndex).toHaveBeenCalledTimes(1);
    scrollParent.remove();
  });

  it("never scrolls when there is no unread boundary, regardless of anchored-bar height changes", () => {
    const scrollToIndex = vi.fn();
    const virtuosoRef = { current: { scrollToIndex } as unknown as VirtuosoHandle };
    const DirectHarness = makeHarness(virtuosoRef, document.createElement("div"), null);

    const { rerender } = render(<DirectHarness offsetPx={0} />);
    expect(scrollToIndex).not.toHaveBeenCalled();

    rerender(<DirectHarness offsetPx={76} />);

    expect(scrollToIndex).not.toHaveBeenCalled();
  });

  it("stops re-scrolling once the settling window has elapsed, even without any user interaction", () => {
    vi.useFakeTimers();
    const scrollToIndex = vi.fn();
    const virtuosoRef = { current: { scrollToIndex } as unknown as VirtuosoHandle };
    const DirectHarness = makeHarness(virtuosoRef, document.createElement("div"));

    const { rerender } = render(<DirectHarness offsetPx={0} />);
    expect(scrollToIndex).toHaveBeenCalledTimes(1);

    vi.advanceTimersByTime(4001);
    rerender(<DirectHarness offsetPx={76} />);

    expect(scrollToIndex).toHaveBeenCalledTimes(1);
    vi.useRealTimers();
  });

  it("keeps the same offset across a live-update reassertion triggered by a new message, not just an offset change", () => {
    const scrollToIndex = vi.fn();
    const virtuosoRef = { current: { scrollToIndex } as unknown as VirtuosoHandle };
    const DirectHarness = makeHarness(virtuosoRef, document.createElement("div"));

    const { rerender } = render(<DirectHarness offsetPx={76} />);
    expect(scrollToIndex).toHaveBeenCalledTimes(1);
    expect(scrollToIndex).toHaveBeenLastCalledWith({ index: 1, align: "start", offset: -76 });

    // A live message arriving mid-settling-window changes `items` (a new
    // array reference) while the anchored bar's height is unchanged — the
    // pre-existing multi-wave reassertion must reapply the *same* offset,
    // not drop or double it.
    rerender(<DirectHarness offsetPx={76} renderItems={itemsWithLiveMessage} />);

    expect(scrollToIndex).toHaveBeenCalledTimes(2);
    expect(scrollToIndex).toHaveBeenLastCalledWith({ index: 1, align: "start", offset: -76 });
  });
});
