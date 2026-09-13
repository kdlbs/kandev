import { useCallback, useLayoutEffect, useMemo, useRef, useState, type RefObject } from "react";
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

function columnIdForElement(elements: ReadonlyMap<string, HTMLElement>, target: Element) {
  for (const [id, element] of elements) {
    if (element === target) return id;
  }
  return null;
}

function measureVisibility(
  board: HTMLElement,
  elements: ReadonlyMap<string, HTMLElement>,
  visibility: Map<string, VisibilityEntry>,
): boolean {
  const root = board.getBoundingClientRect();
  if (root.width <= 0 || root.height <= 0) return false;
  for (const [id, element] of elements) {
    const rect = element.getBoundingClientRect();
    visibility.set(id, {
      element,
      isIntersecting:
        rect.right > root.left &&
        rect.left < root.right &&
        rect.bottom > root.top &&
        rect.top < root.bottom,
    });
  }
  return true;
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
  layoutKey = "columns",
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

  useLayoutEffect(() => {
    const board = boardRef.current;
    observerRef.current?.disconnect();
    observerRef.current = null;
    visibilityRef.current.clear();
    if (!board || typeof IntersectionObserver === "undefined") {
      setVisibleIds(new Set());
      setObserverReady(false);
      return;
    }
    setObserverReady(measureVisibility(board, elementsRef.current, visibilityRef.current));
    updateVisibleIds();

    const observer = new IntersectionObserver(
      (entries) => {
        if (observerRef.current !== observer) return;
        for (const entry of entries) {
          const taskId = columnIdForElement(elementsRef.current, entry.target);
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
    // Reflow or board replacement rebuilds observation from measured geometry.
  }, [hasColumns, layoutKey, isMobile, updateVisibleIds]);

  const fallbackTaskId =
    focusedTaskId && orderedIds.includes(focusedTaskId) ? focusedTaskId : (orderedIds[0] ?? null);

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
