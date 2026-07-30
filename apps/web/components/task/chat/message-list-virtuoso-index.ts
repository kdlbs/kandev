import { useMemo, useState } from "react";
import type { RenderItem } from "@/hooks/use-processed-messages";
import { createDebugLogger, isDebug } from "@/lib/debug/log";
import { getItemKey } from "./message-list-shared";

const FIRST_INDEX_BASE = 100_000;

const debugFirstIndex = createDebugLogger("chat:virtuoso:firstIndex");

function computeFirstItemIndex(prevKeys: string[], prevIndex: number, keys: string[]): number {
  if (prevKeys.length > 0 && keys.length > prevKeys.length) {
    const oldFirstKey = prevKeys[0];
    const newPos = keys.indexOf(oldFirstKey);
    if (newPos > 0) return prevIndex - newPos;
    if (newPos === -1) {
      for (let i = 0; i < prevKeys.length; i++) {
        const idx = keys.indexOf(prevKeys[i]);
        if (idx >= 0) return prevIndex - (idx - i);
      }
    }
    return prevIndex;
  }
  if (prevKeys.length === 0 && keys.length > 0) {
    return FIRST_INDEX_BASE - keys.length + 1;
  }
  return prevIndex;
}

type IndexState = { keys: string[]; firstItemIndex: number };

/** Virtuoso's `firstItemIndex` must only ever grow smaller (never reset) or
 * its scroll-anchored windowing jumps. Tracks a stable index across item-key
 * churn: preserves position when items prepend or reorder rather than
 * resetting to `FIRST_INDEX_BASE` on every render. */
export function useStableFirstItemIndex(items: RenderItem[]) {
  const keys = useMemo(() => items.map(getItemKey), [items]);

  const [state, setState] = useState<IndexState>(() => {
    const firstItemIndex = FIRST_INDEX_BASE - keys.length + 1;
    if (isDebug()) {
      debugFirstIndex("init", {
        keyCount: keys.length,
        firstItemIndex,
        firstKey: keys[0] ?? "-",
        lastKey: keys[keys.length - 1] ?? "-",
      });
    }
    return { keys, firstItemIndex };
  });

  if (keys !== state.keys) {
    const nextIndex = computeFirstItemIndex(state.keys, state.firstItemIndex, keys);
    if (isDebug()) {
      debugFirstIndex("transition", {
        prevKeyCount: state.keys.length,
        nextKeyCount: keys.length,
        prevIndex: state.firstItemIndex,
        nextIndex,
        delta: nextIndex - state.firstItemIndex,
        prevFirstKey: state.keys[0] ?? "-",
        nextFirstKey: keys[0] ?? "-",
        prevLastKey: state.keys[state.keys.length - 1] ?? "-",
        nextLastKey: keys[keys.length - 1] ?? "-",
      });
    }
    setState({ keys, firstItemIndex: nextIndex });
    return nextIndex;
  }

  return state.firstItemIndex;
}
