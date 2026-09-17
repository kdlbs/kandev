import { describe, expect, it } from "vitest";
import { Virtualizer } from "@tanstack/react-virtual";
import { measureFileTreeElement } from "./file-tree-measurement";

function createVirtualizer(estimateSize: number): Virtualizer<HTMLDivElement, HTMLDivElement> {
  return new Virtualizer<HTMLDivElement, HTMLDivElement>({
    count: 1,
    getScrollElement: () => null,
    estimateSize: () => estimateSize,
    scrollToFn: () => {},
    observeElementRect: () => {},
    observeElementOffset: () => {},
    getItemKey: () => "row-a",
  });
}

function resizeEntry(element: HTMLDivElement, blockSize: number): ResizeObserverEntry {
  return {
    target: element,
    borderBoxSize: [{ blockSize, inlineSize: 320 }],
  } as unknown as ResizeObserverEntry;
}

describe("file-tree row measurements", () => {
  it("retains a positive cached row size when a hidden row reports zero height", () => {
    const element = document.createElement("div");
    element.dataset.index = "0";
    const virtualizer = createVirtualizer(28);
    virtualizer.itemSizeCache.set("row-a", 32);

    expect(measureFileTreeElement(element, resizeEntry(element, 0), virtualizer)).toBe(32);
  });

  it.each([28, 44])(
    "uses the current estimate when no positive size exists (%dpx)",
    (estimateSize) => {
      const element = document.createElement("div");
      element.dataset.index = "0";
      const virtualizer = createVirtualizer(estimateSize);

      expect(measureFileTreeElement(element, resizeEntry(element, 0), virtualizer)).toBe(
        estimateSize,
      );
    },
  );

  it("accepts a later positive measurement after a cached fallback", () => {
    const element = document.createElement("div");
    element.dataset.index = "0";
    const virtualizer = createVirtualizer(28);
    virtualizer.itemSizeCache.set("row-a", 28);

    expect(measureFileTreeElement(element, resizeEntry(element, 48), virtualizer)).toBe(48);
  });
});
