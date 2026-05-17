"use client";

import { useCallback, useState, type KeyboardEvent as ReactKeyboardEvent } from "react";
import type { VisibleRow } from "./use-tree";

/**
 * Keyboard navigation on a useTree's flat visibleRows list:
 * - ArrowDown / ArrowUp move focus by one visible row.
 * - ArrowRight expands a collapsed dir; for an already-expanded dir or a file,
 *   moves to the next row.
 * - ArrowLeft collapses an expanded dir; for a collapsed dir or a file,
 *   moves to the parent row (first row above whose depth is smaller).
 * - Enter (and Space) activates: dir toggles, file fires onActivate.
 *
 * Focus state is owned by this hook. Consumers wire `handleKeyDown` to the
 * scrollable container's onKeyDown and read `focusedPath` to highlight a row.
 */

export interface UseTreeKeyboardNavOptions<N> {
  visibleRows: VisibleRow<N>[];
  toggle: (path: string) => void;
  expand: (path: string) => void;
  collapse: (path: string) => void;
  isExpanded: (path: string) => boolean;
  /** Called when Enter / Space is pressed on a file row. */
  onActivate?: (row: VisibleRow<N>) => void;
}

export interface UseTreeKeyboardNavResult<N> {
  focusedPath: string | null;
  setFocusedPath: (path: string | null) => void;
  handleKeyDown: (e: ReactKeyboardEvent) => void;
  /** Equivalent of running `handleKeyDown` programmatically — useful in tests. */
  dispatchKey: (key: string) => void;
  visibleRows: VisibleRow<N>[];
}

function moveIndex(idx: number, delta: number, max: number): number {
  if (idx < 0) return delta > 0 ? 0 : max - 1;
  return Math.max(0, Math.min(idx + delta, max - 1));
}

function findParentIndex<N>(rows: VisibleRow<N>[], idx: number): number {
  const row = rows[idx];
  for (let i = idx - 1; i >= 0; i--) {
    if (rows[i].depth < row.depth) return i;
  }
  return -1;
}

interface HorizontalCtx<N> {
  rows: VisibleRow<N>[];
  idx: number;
  isExpanded: (p: string) => boolean;
  expand: (p: string) => void;
  collapse: (p: string) => void;
  setFocusedPath: (p: string | null) => void;
}

function handleHorizontal<N>(key: "ArrowLeft" | "ArrowRight", ctx: HorizontalCtx<N>): boolean {
  const { rows, idx, isExpanded, expand, collapse, setFocusedPath } = ctx;
  const row = rows[idx];
  if (!row) return false;
  if (key === "ArrowRight") {
    if (row.isDir && !isExpanded(row.path)) expand(row.path);
    else if (idx + 1 < rows.length) setFocusedPath(rows[idx + 1].path);
    return true;
  }
  if (row.isDir && isExpanded(row.path)) {
    collapse(row.path);
    return true;
  }
  const parent = findParentIndex(rows, idx);
  if (parent >= 0) setFocusedPath(rows[parent].path);
  return true;
}

function handleActivate<N>(
  rows: VisibleRow<N>[],
  idx: number,
  toggle: (p: string) => void,
  onActivate: ((row: VisibleRow<N>) => void) | undefined,
): boolean {
  const row = rows[idx];
  if (!row) return false;
  if (row.isDir) toggle(row.path);
  else onActivate?.(row);
  return true;
}

export function useTreeKeyboardNav<N>(
  opts: UseTreeKeyboardNavOptions<N>,
): UseTreeKeyboardNavResult<N> {
  const { visibleRows, toggle, expand, collapse, isExpanded, onActivate } = opts;
  const [focusedPath, setFocusedPath] = useState<string | null>(null);

  const apply = useCallback(
    (key: string): boolean => {
      const max = visibleRows.length;
      if (max === 0) return false;
      const idx = visibleRows.findIndex((r) => r.path === focusedPath);

      if (key === "ArrowDown") {
        setFocusedPath(visibleRows[moveIndex(idx, 1, max)].path);
        return true;
      }
      if (key === "ArrowUp") {
        setFocusedPath(visibleRows[moveIndex(idx, -1, max)].path);
        return true;
      }
      if (key === "ArrowLeft" || key === "ArrowRight") {
        return handleHorizontal(key, {
          rows: visibleRows,
          idx,
          isExpanded,
          expand,
          collapse,
          setFocusedPath,
        });
      }
      if (key === "Enter" || key === " ") {
        return handleActivate(visibleRows, idx, toggle, onActivate);
      }
      return false;
    },
    [visibleRows, focusedPath, toggle, expand, collapse, isExpanded, onActivate],
  );

  const handleKeyDown = useCallback(
    (e: ReactKeyboardEvent) => {
      if (apply(e.key)) e.preventDefault();
    },
    [apply],
  );

  const dispatchKey = useCallback(
    (key: string) => {
      apply(key);
    },
    [apply],
  );

  return { focusedPath, setFocusedPath, handleKeyDown, dispatchKey, visibleRows };
}
