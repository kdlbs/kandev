import {
  useCallback,
  useEffect,
  useLayoutEffect,
  useRef,
  type RefObject,
  type SyntheticEvent,
} from "react";
import { resolveRemainingThreadId } from "@/lib/threads/thread-selection-fallback";
import { nearestTaskId } from "./thread-viewport-geometry";

function columns(board: HTMLElement | null) {
  return new Map(
    Array.from(board?.querySelectorAll<HTMLElement>("[data-thread-column-id]") ?? []).map(
      (element) => [element.dataset.threadColumnId!, element],
    ),
  );
}

function restoreRemovedFocus(
  focused: Element | null,
  element: HTMLElement | undefined | null,
  container: HTMLElement | null,
) {
  if (!focused || focused.isConnected || document.activeElement !== document.body) return;
  const fallback =
    element?.querySelector<HTMLElement>("[data-thread-task-menu] button") ??
    container?.querySelector<HTMLElement>(
      '[data-testid="threads-empty-state"], [data-testid="threads-loading-state"]',
    );
  fallback?.focus({ preventScroll: true });
}

/** Keep the reader's task and offset across membership and layout changes, without reranking. */
export function useThreadSelectionRecovery(
  orderedIds: readonly string[],
  boardRef: RefObject<HTMLDivElement | null>,
  isMobile: boolean,
  layoutKey = "columns",
) {
  const previous = useRef<{
    ids: readonly string[];
    layoutKey: string;
    taskId: string | null;
    offset: number;
    width: number | null;
    focused: Element | null;
    container: HTMLElement | null;
  }>({ ids: [], layoutKey, taskId: null, offset: 0, width: null, focused: null, container: null });
  const record = useCallback(
    (requestedId?: string) => {
      const board = boardRef.current;
      if (!board) return;
      const elements = columns(board);
      const bounds = board.getBoundingClientRect();
      // CSS resize can emit scroll before the responsive layout commits.
      // Keep the reader identity until that reflow restores its anchor.
      if (
        !requestedId &&
        previous.current.width !== null &&
        previous.current.width !== bounds.width
      )
        return;
      const held = previous.current.taskId;
      const heldRect = held ? elements.get(held)?.getBoundingClientRect() : null;
      const heldVisible = heldRect && heldRect.right > bounds.left && heldRect.left < bounds.right;
      const taskId =
        requestedId ??
        (!isMobile && heldVisible ? held : nearestTaskId([...elements.keys()], board, elements));
      const element = taskId ? elements.get(taskId) : null;
      previous.current = {
        ...previous.current,
        taskId,
        width: bounds.width,
        offset: element ? element.getBoundingClientRect().left - bounds.left : 0,
        focused: board.contains(document.activeElement)
          ? document.activeElement
          : previous.current.focused,
        container: board.parentElement,
      };
    },
    [boardRef, isMobile],
  );
  const idsKey = orderedIds.join("\u0000");
  useLayoutEffect(() => {
    const old = previous.current;
    const next = resolveRemainingThreadId(old.ids, orderedIds, old.taskId);
    const board = boardRef.current;
    const element = next ? columns(board).get(next) : null;
    const changed = old.ids.join("\u0000") !== idsKey || old.layoutKey !== layoutKey;
    if (changed && old.ids.length > 0 && board && element) {
      if (next === old.taskId) {
        const adjustment =
          element.getBoundingClientRect().left - board.getBoundingClientRect().left - old.offset;
        // Even assigning the current offset cancels an in-flight browser smooth scroll.
        if (adjustment !== 0) board.scrollLeft += adjustment;
      } else {
        element.scrollIntoView({ inline: "start", block: "nearest", behavior: "instant" });
      }
    }
    if (changed) restoreRemovedFocus(old.focused, element, old.container);
    previous.current = { ...old, ids: orderedIds, layoutKey, taskId: next };
    if (changed) record(next ?? undefined);
  }, [idsKey, layoutKey, orderedIds, boardRef, record]);
  const isEmpty = orderedIds.length === 0;
  useEffect(() => {
    const board = boardRef.current;
    if (!board) return;
    const onScroll = () => record();
    board.addEventListener("scroll", onScroll, { passive: true });
    return () => board.removeEventListener("scroll", onScroll);
  }, [boardRef, record, isEmpty]);
  return (event: SyntheticEvent) => {
    const column = (event.target as Element).closest<HTMLElement>("[data-thread-column-id]");
    if (column) record(column.dataset.threadColumnId);
  };
}
