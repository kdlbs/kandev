import {
  measureElement as tanstackMeasureElement,
  observeElementRect,
} from "@tanstack/react-virtual";
import type { Rect, Virtualizer } from "@tanstack/react-virtual";

/** Let the browser's layout pass size the viewport before mounting its rows. */
export function observeFileTreeRect(
  instance: Virtualizer<HTMLDivElement, HTMLDivElement>,
  onRect: (rect: Rect) => void,
): (() => void) | undefined {
  const { scrollElement, targetWindow } = instance;
  if (!scrollElement || !targetWindow) return;
  if (!targetWindow.ResizeObserver) return observeElementRect(instance, onRect);
  let active = true;
  const observer = new targetWindow.ResizeObserver((entries) => {
    const entry = entries[0];
    if (!active || !entry) return;
    const box = entry.borderBoxSize?.[0];
    onRect({
      width: Math.round(box?.inlineSize ?? entry.contentRect.width),
      height: Math.round(box?.blockSize ?? entry.contentRect.height),
    });
  });
  observer.observe(scrollElement, { box: "border-box" });
  return () => {
    active = false;
    observer.disconnect();
  };
}

export function measureFileTreeElement(
  element: HTMLDivElement,
  entry: ResizeObserverEntry | undefined,
  instance: Virtualizer<HTMLDivElement, HTMLDivElement>,
): number {
  // ResizeObserver supplies actual geometry after layout. Mounting rows uses
  // cached/estimated sizes instead of forcing layout for each new DOM node.
  // Without an observer, synchronous measurement remains the geometry source.
  let measuredSize = 0;
  if (entry) measuredSize = tanstackMeasureElement(element, entry, instance);
  else if (!instance.targetWindow?.ResizeObserver) measuredSize = element.offsetHeight;
  if (measuredSize > 0) return measuredSize;

  const index = instance.indexFromElement(element);
  const key = instance.options.getItemKey(index);
  // Hidden rows must retain positive cached geometry until the browser can measure them again.
  const cachedSize = instance.itemSizeCache.get(key);
  return cachedSize !== undefined && cachedSize > 0
    ? cachedSize
    : instance.options.estimateSize(index);
}
