import type { Virtualizer } from "@tanstack/react-virtual";

export type ChangesTimelineAnchor = {
  key: string;
  index: number;
  viewportOffset: number;
};

type TimelineRow = { key: string };
type MeasuredItem = { index: number; start: number; end: number };

export function refreshChangesTimelineMeasurements(
  viewport: HTMLDivElement | null,
  virtualizer: Virtualizer<HTMLDivElement, HTMLDivElement>,
): void {
  const measurements = Array.from(
    viewport?.querySelectorAll<HTMLDivElement>("[data-changes-timeline-row]") ?? [],
  ).flatMap((element) => {
    const index = Number(element.dataset.index);
    if (
      !element.isConnected ||
      !Number.isInteger(index) ||
      index < 0 ||
      index >= virtualizer.options.count
    ) {
      return [];
    }
    const key = virtualizer.options.getItemKey(index);
    if (element.dataset.changesRowKey !== key) return [];
    const size = element.offsetHeight || virtualizer.itemSizeCache.get(key);
    return size !== undefined && size > 0 ? [{ element, index, key, size }] : [];
  });

  // Unchanged mounted rows will not receive another observer entry after cache invalidation.
  virtualizer.measure();
  // Rebuild estimated offsets synchronously before restoring the mounted row sizes.
  virtualizer.getVirtualItems();
  for (const { element, index, key, size } of measurements) {
    if (
      element.isConnected &&
      index < virtualizer.options.count &&
      virtualizer.options.getItemKey(index) === key
    ) {
      virtualizer.resizeItem(index, size);
    }
  }
}

export function observeChangesTimelinePresentationChanges(
  element: HTMLElement,
  onChange: () => void,
): () => void {
  let width = element.getBoundingClientRect().width;
  const resizeObserver = new ResizeObserver((entries) => {
    const entry = entries.find((candidate) => candidate.target === element);
    if (!entry || entry.contentRect.width === width) return;
    width = entry.contentRect.width;
    onChange();
  });
  resizeObserver.observe(element);

  let language = document.documentElement.lang;
  const languageObserver = new MutationObserver(() => {
    const nextLanguage = document.documentElement.lang;
    if (nextLanguage === language) return;
    language = nextLanguage;
    onChange();
  });
  languageObserver.observe(document.documentElement, {
    attributes: true,
    attributeFilter: ["lang"],
  });

  const pointerQuery = window.matchMedia?.("(pointer: coarse)");
  let coarsePointer = pointerQuery?.matches;
  const onPointerChange = () => {
    if (!pointerQuery || pointerQuery.matches === coarsePointer) return;
    coarsePointer = pointerQuery.matches;
    onChange();
  };
  pointerQuery?.addEventListener("change", onPointerChange);

  const fonts = document.fonts;
  const onFontsChanged = () => onChange();
  fonts?.addEventListener("loadingdone", onFontsChanged);
  fonts?.addEventListener("loadingerror", onFontsChanged);

  return () => {
    resizeObserver.disconnect();
    languageObserver.disconnect();
    pointerQuery?.removeEventListener("change", onPointerChange);
    fonts?.removeEventListener("loadingdone", onFontsChanged);
    fonts?.removeEventListener("loadingerror", onFontsChanged);
  };
}

export function captureChangesTimelineAnchor<Row extends TimelineRow>(
  rows: Row[],
  visibleItems: MeasuredItem[],
  scrollTop: number,
): ChangesTimelineAnchor | null {
  const item = visibleItems.find((candidate) => candidate.end > scrollTop);
  const row = item ? rows[item.index] : undefined;
  if (!item || !row) return null;
  return { key: row.key, index: item.index, viewportOffset: item.start - scrollTop };
}

export function resolveChangesTimelineAnchor<Row extends TimelineRow>(
  rows: Row[],
  anchor: ChangesTimelineAnchor,
  indexByKey?: ReadonlyMap<string, number>,
): { key: string; index: number } | null {
  if (rows.length === 0) return null;
  const exactIndex = indexByKey
    ? (indexByKey.get(anchor.key) ?? -1)
    : rows.findIndex((row) => row.key === anchor.key);
  const index = exactIndex >= 0 ? exactIndex : Math.min(anchor.index, rows.length - 1);
  const row = rows[index];
  return row ? { key: row.key, index } : null;
}

export function scrollTopForChangesTimelineAnchor(
  itemStart: number,
  viewportOffset: number,
): number {
  return Math.max(0, itemStart - viewportOffset);
}
