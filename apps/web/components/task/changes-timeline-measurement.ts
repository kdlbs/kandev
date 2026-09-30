export type ChangesTimelineAnchor = {
  key: string;
  index: number;
  viewportOffset: number;
};

type TimelineRow = { key: string };
type MeasuredItem = { index: number; start: number; end: number };

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
): { key: string; index: number } | null {
  if (rows.length === 0) return null;
  const exactIndex = rows.findIndex((row) => row.key === anchor.key);
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
