import { measureElement as tanstackMeasureElement } from "@tanstack/react-virtual";
import type { Virtualizer } from "@tanstack/react-virtual";

export function measureFileTreeElement(
  element: HTMLDivElement,
  entry: ResizeObserverEntry | undefined,
  instance: Virtualizer<HTMLDivElement, HTMLDivElement>,
): number {
  const measuredSize = tanstackMeasureElement(element, entry, instance);
  if (measuredSize > 0) return measuredSize;

  const index = instance.indexFromElement(element);
  const key = instance.options.getItemKey(index);
  // Hidden rows must retain positive cached geometry until the browser can measure them again.
  const cachedSize = instance.itemSizeCache.get(key);
  return cachedSize !== undefined && cachedSize > 0
    ? cachedSize
    : instance.options.estimateSize(index);
}
