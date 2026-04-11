"use client";

import { useState, useCallback, useRef, useMemo } from "react";

type UseMultiSelectOptions = {
  items: string[];
  onSelectionChange?: (selected: Set<string>) => void;
};

type UseMultiSelectReturn = {
  selectedPaths: Set<string>;
  isSelected: (path: string) => boolean;
  /** Returns true if the click was consumed by selection (modifier key held). */
  handleClick: (path: string, event: React.MouseEvent) => boolean;
  selectAll: () => void;
  clearSelection: () => void;
  setSelectedPaths: (paths: Set<string>) => void;
};

export type SelectionParams = {
  prev: Set<string>;
  path: string;
  items: string[];
  isShift: boolean;
  isCtrlOrMeta: boolean;
  lastClickedRef: React.RefObject<string | null>;
};

/** Exported for testing. */
export function computeNextSelection(params: SelectionParams): Set<string> {
  const { prev, path, items, isShift, isCtrlOrMeta, lastClickedRef } = params;
  if (isShift) {
    if (!lastClickedRef.current) {
      lastClickedRef.current = path;
      return new Set([path]);
    }
    const anchorIndex = items.indexOf(lastClickedRef.current);
    const currentIndex = items.indexOf(path);
    if (anchorIndex === -1 || currentIndex === -1) {
      lastClickedRef.current = path;
      return new Set([path]);
    }
    const start = Math.min(anchorIndex, currentIndex);
    const end = Math.max(anchorIndex, currentIndex);
    const next = isCtrlOrMeta ? new Set(prev) : new Set<string>();
    for (let i = start; i <= end; i++) next.add(items[i]);
    return next;
  }

  // Ctrl/Cmd+click: toggle individual item
  const next = new Set(prev);
  if (next.has(path)) {
    next.delete(path);
  } else {
    next.add(path);
  }
  lastClickedRef.current = path;
  return next;
}

export function useMultiSelect({
  items,
  onSelectionChange,
}: UseMultiSelectOptions): UseMultiSelectReturn {
  const [rawSelection, setRawSelection] = useState<Set<string>>(new Set());
  const lastClickedRef = useRef<string | null>(null);

  const itemSet = useMemo(() => new Set(items), [items]);
  const selectedPaths = useMemo(() => {
    if (rawSelection.size === 0) return rawSelection;
    let allValid = true;
    for (const p of rawSelection) {
      if (!itemSet.has(p)) {
        allValid = false;
        break;
      }
    }
    if (allValid) return rawSelection;
    const pruned = new Set<string>();
    for (const p of rawSelection) {
      if (itemSet.has(p)) pruned.add(p);
    }
    return pruned;
  }, [rawSelection, itemSet]);

  const setSelectedPaths = useCallback(
    (paths: Set<string>) => {
      setRawSelection(paths);
      onSelectionChange?.(paths);
    },
    [onSelectionChange],
  );

  const handleClick = useCallback(
    (path: string, event: React.MouseEvent): boolean => {
      const isCtrlOrMeta = event.ctrlKey || event.metaKey;
      const isShift = event.shiftKey;

      // Plain click: set anchor, clear selection, let caller handle action
      if (!isCtrlOrMeta && !isShift) {
        lastClickedRef.current = path;
        setRawSelection((prev) => {
          if (prev.size === 0) return prev;
          return new Set();
        });
        onSelectionChange?.(new Set());
        return false;
      }

      // Modifier click: compute new selection, then notify
      const next = computeNextSelection({
        prev: rawSelection,
        path,
        items,
        isShift,
        isCtrlOrMeta,
        lastClickedRef,
      });
      setRawSelection(next);
      onSelectionChange?.(next);
      return true;
    },
    [items, rawSelection, onSelectionChange],
  );

  const selectAll = useCallback(() => {
    const all = new Set(items);
    setRawSelection(all);
    onSelectionChange?.(all);
  }, [items, onSelectionChange]);

  const clearSelection = useCallback(() => {
    setRawSelection(new Set());
    onSelectionChange?.(new Set());
    lastClickedRef.current = null;
  }, [onSelectionChange]);

  const isSelected = useCallback((path: string) => selectedPaths.has(path), [selectedPaths]);

  return {
    selectedPaths,
    isSelected,
    handleClick,
    selectAll,
    clearSelection,
    setSelectedPaths,
  };
}
