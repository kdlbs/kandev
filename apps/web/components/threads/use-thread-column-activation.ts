import { useCallback, useEffect, useMemo, useRef, useState, type RefObject } from "react";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import { nearestTaskId } from "./thread-viewport-geometry";
import { useMobileThreadPosition } from "./use-mobile-thread-position";

type VisibilityEntry = {
  element: Element;
  isIntersecting: boolean;
};

export type ThreadColumnActivation = {
  boardRef: RefObject<HTMLDivElement | null>;
  registerColumn: (taskId: string, element: HTMLElement | null) => void;
  preloadTaskIds: ReadonlySet<string>;
  detailTaskIds: ReadonlySet<string>;
  mobileTaskId: string | null;
};

function sameIds(a: ReadonlySet<string>, b: ReadonlySet<string>): boolean {
  if (a.size !== b.size) return false;
  for (const id of a) {
    if (!b.has(id)) return false;
  }
  return true;
}

function addAdjacent(ids: Set<string>, orderedIds: readonly string[], id: string): void {
  const index = orderedIds.indexOf(id);
  if (index < 0) return;
  if (index > 0) ids.add(orderedIds[index - 1]);
  if (index < orderedIds.length - 1) ids.add(orderedIds[index + 1]);
}

function resolveVisibleIds(
  orderedIds: readonly string[],
  visibleIds: ReadonlySet<string>,
  observerReady: boolean,
  fallbackTaskId: string | null,
): string[] {
  if (observerReady) return orderedIds.filter((id) => visibleIds.has(id));
  return fallbackTaskId ? [fallbackTaskId] : [];
}

function buildActivationSets({
  orderedIds,
  visibleIds,
  observerReady,
  fallbackTaskId,
  isMobile,
  board,
  elements,
}: {
  orderedIds: readonly string[];
  visibleIds: ReadonlySet<string>;
  observerReady: boolean;
  fallbackTaskId: string | null;
  isMobile: boolean;
  board: HTMLElement | null;
  elements: ReadonlyMap<string, HTMLElement>;
}) {
  const visible = resolveVisibleIds(orderedIds, visibleIds, observerReady, fallbackTaskId);
  const detailTaskIds = new Set<string>();
  if (isMobile) {
    const nearest = nearestTaskId(visible, board, elements);
    if (nearest) detailTaskIds.add(nearest);
  } else {
    visible.forEach((id) => detailTaskIds.add(id));
  }

  const preloadTaskIds = new Set(visible);
  for (const id of visible) addAdjacent(preloadTaskIds, orderedIds, id);
  return {
    preloadTaskIds: new Set(orderedIds.filter((id) => preloadTaskIds.has(id))),
    detailTaskIds,
  };
}

/**
 * Owns the expensive part of a Threads column's lifecycle. Every task keeps a
 * shell, while only visible columns receive session membership data and only
 * the active detail window mounts a transcript.
 */
export function useThreadColumnActivation(
  orderedIds: readonly string[],
  focusedTaskId?: string | null,
): ThreadColumnActivation {
  const { isMobile } = useResponsiveBreakpoint();
  const boardRef = useRef<HTMLDivElement>(null);
  const elementsRef = useRef(new Map<string, HTMLElement>());
  const visibilityRef = useRef(new Map<string, VisibilityEntry>());
  const observerRef = useRef<IntersectionObserver | null>(null);
  const [visibleIds, setVisibleIds] = useState<Set<string>>(() => new Set());
  const [observerReady, setObserverReady] = useState(false);
  const idsKey = orderedIds.join("\u0000");
  const hasColumns = orderedIds.length > 0;

  const updateVisibleIds = useCallback(() => {
    const next = new Set<string>();
    for (const [id, entry] of visibilityRef.current) {
      if (entry.isIntersecting && elementsRef.current.get(id) === entry.element) next.add(id);
    }
    setVisibleIds((previous) => (sameIds(previous, next) ? previous : next));
  }, []);

  const registerColumn = useCallback(
    (taskId: string, element: HTMLElement | null) => {
      if (element) {
        elementsRef.current.set(taskId, element);
        observerRef.current?.observe(element);
        return;
      }
      const previous = elementsRef.current.get(taskId);
      if (previous) observerRef.current?.unobserve(previous);
      elementsRef.current.delete(taskId);
      visibilityRef.current.delete(taskId);
      updateVisibleIds();
    },
    [updateVisibleIds],
  );

  useEffect(() => {
    const board = boardRef.current;
    observerRef.current?.disconnect();
    observerRef.current = null;
    visibilityRef.current.clear();
    setVisibleIds(new Set());
    setObserverReady(false);
    if (!board || typeof IntersectionObserver === "undefined") return;

    const observer = new IntersectionObserver(
      (entries) => {
        for (const entry of entries) {
          const taskId = [...elementsRef.current.entries()].find(
            ([, element]) => element === entry.target,
          )?.[0];
          if (!taskId) continue;
          visibilityRef.current.set(taskId, {
            element: entry.target,
            isIntersecting: entry.isIntersecting,
          });
        }
        setObserverReady(true);
        updateVisibleIds();
      },
      { root: board, threshold: [0, 0.5, 1] },
    );
    observerRef.current = observer;
    for (const element of elementsRef.current.values()) observer.observe(element);

    return () => {
      observer.disconnect();
      if (observerRef.current === observer) observerRef.current = null;
    };
    // Callback refs reconcile membership without dropping surviving visibility.
    // Only an empty/nonempty transition replaces the board's DOM element.
  }, [hasColumns, updateVisibleIds]);

  const fallbackTaskId = useMemo(() => {
    if (focusedTaskId && orderedIds.includes(focusedTaskId)) return focusedTaskId;
    return orderedIds[0] ?? null;
  }, [focusedTaskId, idsKey]);

  const mobileTaskId = useMobileThreadPosition({
    enabled: isMobile,
    boardRef,
    elementsRef,
    orderedIds,
    fallbackTaskId,
  });

  const sets = useMemo(
    () =>
      buildActivationSets({
        orderedIds,
        visibleIds,
        observerReady,
        fallbackTaskId: isMobile ? mobileTaskId : fallbackTaskId,
        isMobile,
        board: boardRef.current,
        elements: elementsRef.current,
      }),
    [fallbackTaskId, idsKey, isMobile, mobileTaskId, observerReady, visibleIds],
  );

  return { boardRef, registerColumn, mobileTaskId, ...sets };
}
