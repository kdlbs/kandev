import { useLayoutEffect, useMemo, useRef } from "react";
import type { ActiveThread } from "./active-threads";

/**
 * Holds the deck's columns in the slots they already occupy.
 *
 * `selectActiveThreads` ranks by attention and recency, which is the right
 * order to *arrive* at but the wrong thing to re-apply live: replying to a
 * thread moves it from "needs a human" to "working" and refreshes its
 * activity, so the column the reader was typing into would slide across the
 * deck, then slide back when the turn ended. A deck whose columns move while
 * you use them is unusable, so ranking decides where a column first appears
 * and nothing after that.
 *
 * New threads append rather than sorting in, so an arriving column never
 * displaces one already on screen. A thread that leaves gives up its slot
 * outright; reserving it would mean holding a gap for work that may never
 * come back.
 */
export function applyStableThreadOrder(
  previousOrder: readonly string[],
  threads: readonly ActiveThread[],
  maxItems: number | null = null,
): ActiveThread[] {
  const byTaskId = new Map(threads.map((thread) => [thread.taskId, thread]));
  const held = previousOrder
    .map((taskId) => byTaskId.get(taskId))
    .filter((thread): thread is ActiveThread => thread !== undefined);

  const heldIds = new Set(held.map((thread) => thread.taskId));
  const arrived = threads.filter((thread) => !heldIds.has(thread.taskId));
  const ordered = [...held, ...arrived];
  return maxItems === null ? ordered : ordered.slice(0, Math.max(0, maxItems));
}

/**
 * Applies {@link applyStableThreadOrder} across renders.
 *
 * The carried-over order is recorded after commit, not during render: a render
 * React discards must not leave its order behind, or a later render would hold
 * slots for an arrangement the reader never saw. Deriving during render stays
 * correct because the ordering is a pure function of the committed order and
 * the current threads.
 */
export function useStableThreadOrder(
  threads: ActiveThread[],
  resetKey?: string | number,
  options: {
    /** Sorted/capped columns to use for an intentional reset. */
    resetThreads?: ActiveThread[];
    /** Hard admission bound for normal live reconciliation. */
    maxItems?: number | null;
  } = {},
): ActiveThread[] {
  const orderRef = useRef<string[]>([]);
  const resetKeyRef = useRef<string | number | undefined>(resetKey);
  const isReset = resetKeyRef.current !== resetKey;
  const resetThreads = options.resetThreads ?? threads;
  const maxItems = options.maxItems ?? null;
  const source = isReset ? resetThreads : threads;
  const previousOrder = isReset ? [] : orderRef.current;
  const ordered = useMemo(
    () => applyStableThreadOrder(previousOrder, source, maxItems),
    // eslint-disable-next-line react-hooks/exhaustive-deps -- the ref is the
    // carried-over order, deliberately read without re-running on its change.
    [maxItems, resetKey, source],
  );
  useLayoutEffect(() => {
    orderRef.current = ordered.map((thread) => thread.taskId);
    resetKeyRef.current = resetKey;
  }, [ordered, resetKey]);
  return ordered;
}
