import type { LayoutColumn, LayoutGroup } from './types';
import { LAYOUT_SIDEBAR_RATIO } from './constants';

/**
 * Get the effective pinned width for a column,
 * considering user overrides, maxWidth cap, and ratio-based default.
 */
export function getPinnedWidth(
  column: LayoutColumn,
  totalWidth: number,
  override?: number,
): number {
  if (override !== undefined) {
    const max = column.maxWidth ?? Infinity;
    const min = column.minWidth ?? 50;
    return Math.max(min, Math.min(override, max));
  }
  const ratioWidth = Math.round(totalWidth * LAYOUT_SIDEBAR_RATIO);
  const max = column.maxWidth ?? Infinity;
  const min = column.minWidth ?? 50;
  return Math.max(min, Math.min(ratioWidth, max));
}

type ColumnBucket = {
  pinnedTotal: number;
  pinnedWidths: number[];
  explicitIndices: number[];
  explicitTotal: number;
  flexIndices: number[];
};

/** First pass: classify columns and compute pinned widths. */
function classifyColumns(
  columns: LayoutColumn[],
  totalWidth: number,
  pinnedOverrides: Map<string, number>,
): ColumnBucket {
  const bucket: ColumnBucket = {
    pinnedTotal: 0,
    pinnedWidths: new Array(columns.length).fill(0),
    explicitIndices: [],
    explicitTotal: 0,
    flexIndices: [],
  };

  for (let i = 0; i < columns.length; i++) {
    const col = columns[i];
    if (col.pinned) {
      const w = getPinnedWidth(col, totalWidth, pinnedOverrides.get(col.id));
      bucket.pinnedWidths[i] = w;
      bucket.pinnedTotal += w;
    } else if (col.width !== undefined && col.width > 0) {
      bucket.explicitIndices.push(i);
      bucket.explicitTotal += col.width;
    } else {
      bucket.flexIndices.push(i);
    }
  }
  return bucket;
}

/** Second pass: compute non-pinned column widths. */
function computeFlexWidths(
  columns: LayoutColumn[],
  bucket: ColumnBucket,
  remainingSpace: number,
): number[] {
  const widths = [...bucket.pinnedWidths];

  if (bucket.explicitIndices.length > 0 && bucket.flexIndices.length === 0) {
    // All non-pinned have explicit widths → scale proportionally
    const scale = bucket.explicitTotal > 0 ? remainingSpace / bucket.explicitTotal : 1;
    for (const i of bucket.explicitIndices) {
      widths[i] = Math.round((columns[i].width ?? 0) * scale);
    }
    return widths;
  }

  if (bucket.explicitIndices.length > 0) {
    // Mix of explicit and flex — use explicit as-is (capped), flex splits remainder
    let explicitUsed = 0;
    for (const i of bucket.explicitIndices) {
      const w = Math.min(columns[i].width ?? 0, remainingSpace);
      widths[i] = w;
      explicitUsed += w;
    }
    const flexSpace = Math.max(0, remainingSpace - explicitUsed);
    const perFlex = bucket.flexIndices.length > 0 ? Math.floor(flexSpace / bucket.flexIndices.length) : 0;
    for (const i of bucket.flexIndices) {
      widths[i] = perFlex;
    }
    return widths;
  }

  // All non-pinned are flex → equal split
  const perFlex = bucket.flexIndices.length > 0 ? Math.floor(remainingSpace / bucket.flexIndices.length) : 0;
  for (const i of bucket.flexIndices) {
    widths[i] = perFlex;
  }
  return widths;
}

/**
 * Compute absolute pixel widths for each column.
 *
 * Strategy:
 * 1. Pinned columns: use getPinnedWidth (ratio-based default or user override)
 * 2. Non-pinned columns with explicit width (from captured layouts): scale
 *    proportionally to fill remaining space
 * 3. Non-pinned columns without width: split remaining space equally
 */
export function computeColumnWidths(
  columns: LayoutColumn[],
  totalWidth: number,
  pinnedWidths: Map<string, number>,
): number[] {
  const bucket = classifyColumns(columns, totalWidth, pinnedWidths);
  const remainingSpace = Math.max(0, totalWidth - bucket.pinnedTotal);
  const widths = computeFlexWidths(columns, bucket, remainingSpace);

  return widths;
}

/**
 * Compute absolute pixel heights for groups within a column.
 * Equal distribution among groups.
 */
export function computeGroupHeights(groups: LayoutGroup[], totalHeight: number): number[] {
  if (groups.length === 0) return [];
  const h = Math.floor(totalHeight / groups.length);
  return groups.map(() => h);
}
