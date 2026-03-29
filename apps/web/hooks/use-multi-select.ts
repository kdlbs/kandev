"use client";

import { useState, useCallback, useRef, useMemo } from "react";

type UseMultiSelectOptions = {
  items: string[];
  onSelectionChange?: (selected: Set<string>) => void;
};

type UseMultiSelectReturn = {
  selectedPaths: Set<string>;
  isSelected: (path: string) => boolean;
  handleClick: (path: string, event: React.MouseEvent) => void;
  selectAll: () => void;
  clearSelection: () => void;
  setSelectedPaths: (paths: Set<string>) => void;
};

export function useMultiSelect({
  items,
  onSelectionChange,
}: UseMultiSelectOptions): UseMultiSelectReturn {
  const [rawSelection, setRawSelection] = useState<Set<string>>(new Set());
  const lastClickedRef = useRef<string | null>(null);

  // Derive the effective selection by pruning paths not in items.
  // This is computed during render (not in an effect) so it's always consistent.
  const itemSet = useMemo(() => new Set(items), [items]);
  const selectedPaths = useMemo(() => {
    if (rawSelection.size === 0) return rawSelection;
    let allValid = true;
    for (const path of rawSelection) {
      if (!itemSet.has(path)) {
        allValid = false;
        break;
      }
    }
    if (allValid) return rawSelection;
    const pruned = new Set<string>();
    for (const path of rawSelection) {
      if (itemSet.has(path)) pruned.add(path);
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
    (path: string, event: React.MouseEvent) => {
      const isCtrlOrMeta = event.ctrlKey || event.metaKey;
      const isShift = event.shiftKey;

      setRawSelection((prev) => {
        let next: Set<string>;

        if (isShift && lastClickedRef.current) {
          const anchorIndex = items.indexOf(lastClickedRef.current);
          const currentIndex = items.indexOf(path);
          if (anchorIndex === -1 || currentIndex === -1) {
            next = new Set([path]);
          } else {
            const start = Math.min(anchorIndex, currentIndex);
            const end = Math.max(anchorIndex, currentIndex);
            next = isCtrlOrMeta ? new Set(prev) : new Set<string>();
            for (let i = start; i <= end; i++) {
              next.add(items[i]);
            }
          }
        } else if (isCtrlOrMeta) {
          next = new Set(prev);
          if (next.has(path)) {
            next.delete(path);
          } else {
            next.add(path);
          }
          lastClickedRef.current = path;
        } else {
          next = new Set([path]);
          lastClickedRef.current = path;
        }

        onSelectionChange?.(next);
        return next;
      });
    },
    [items, onSelectionChange],
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

  const isSelected = useCallback(
    (path: string) => selectedPaths.has(path),
    [selectedPaths],
  );

  return {
    selectedPaths,
    isSelected,
    handleClick,
    selectAll,
    clearSelection,
    setSelectedPaths,
  };
}
