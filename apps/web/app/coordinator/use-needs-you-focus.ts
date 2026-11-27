"use client";

import { useEffect, useRef, type RefObject } from "react";
import type { NeedsYouItem } from "@/lib/coordinator/attention";
import { needsYouItemHeadingId } from "./components/needs-you-item-card";

export const NEEDS_YOU_EMPTY_HEADING_ID = "needs-you-empty-heading";

/**
 * Tracks which item's card last received focus via a document-level
 * `focusin` listener rather than reading `document.activeElement` after an
 * item unmounts: removing a focused node clears `activeElement` before any
 * effect can observe it, so the last real focus target has to be captured
 * as it happens.
 */
function useLastFocusedItemId(): RefObject<string | null> {
  const ref = useRef<string | null>(null);
  useEffect(() => {
    function onFocusIn(event: FocusEvent) {
      const target = event.target;
      if (!(target instanceof Element)) return;
      const itemEl = target.closest<HTMLElement>('[data-testid^="needs-you-item-"]');
      ref.current = itemEl?.dataset.testid?.replace("needs-you-item-", "") ?? ref.current;
    }
    document.addEventListener("focusin", onFocusIn);
    return () => document.removeEventListener("focusin", onFocusIn);
  }, []);
  return ref;
}

function focusTargetId(prevIds: string[], removedIndex: number): string {
  const nextId = prevIds[removedIndex + 1];
  if (nextId) return needsYouItemHeadingId(nextId);
  const previousId = prevIds[removedIndex - 1];
  return previousId ? needsYouItemHeadingId(previousId) : NEEDS_YOU_EMPTY_HEADING_ID;
}

/**
 * Moves focus to the next item's heading when a decided item leaves the
 * Needs-you list, else the previous item's, else the empty state's heading
 * (proposal-cards.md#cards "Focus after a decision"). Only moves focus when
 * it was inside the removed item, so a decision made elsewhere (the chat
 * card, another browser) never steals focus.
 */
export function useNeedsYouFocusAfterDecision(items: NeedsYouItem[]): void {
  const lastFocusedItemIdRef = useLastFocusedItemId();
  const prevIdsRef = useRef<string[] | null>(null);

  useEffect(() => {
    const ids = items.map((item) => item.id);
    const prevIds = prevIdsRef.current;
    prevIdsRef.current = ids;
    if (!prevIds) return;

    const removedIndex = prevIds.findIndex((id) => !ids.includes(id));
    if (removedIndex === -1) return;
    const removedId = prevIds[removedIndex];
    if (lastFocusedItemIdRef.current !== removedId) return;

    document.getElementById(focusTargetId(prevIds, removedIndex))?.focus();
  }, [items, lastFocusedItemIdRef]);
}
