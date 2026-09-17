"use client";

import { useCallback, useEffect, useLayoutEffect, useRef, useState, type RefObject } from "react";

const EDGE_TOLERANCE_PX = 1;
const SCROLL_IDLE_DELAY_MS = 800;

export type KanbanOverflowAxis = "horizontal" | "vertical" | "both";

export type KanbanOverflowState = {
  canScrollBottom: boolean;
  canScrollLeft: boolean;
  canScrollRight: boolean;
  canScrollTop: boolean;
  isScrolling: boolean;
};

type KanbanOverflowOptions = {
  axis?: KanbanOverflowAxis;
  contentRef?: RefObject<HTMLElement | null>;
  revision?: unknown;
};

const EMPTY_OVERFLOW: KanbanOverflowState = {
  canScrollBottom: false,
  canScrollLeft: false,
  canScrollRight: false,
  canScrollTop: false,
  isScrolling: false,
};

function isStateEqual(previous: KanbanOverflowState, next: KanbanOverflowState): boolean {
  return (
    previous.canScrollBottom === next.canScrollBottom &&
    previous.canScrollLeft === next.canScrollLeft &&
    previous.canScrollRight === next.canScrollRight &&
    previous.canScrollTop === next.canScrollTop &&
    previous.isScrolling === next.isScrolling
  );
}

function readOverflowState(
  element: HTMLElement | null,
  axis: KanbanOverflowAxis,
  isScrolling: boolean,
  contentElement: HTMLElement | null,
): KanbanOverflowState {
  if (!element) return { ...EMPTY_OVERFLOW, isScrolling };

  const readsVertical = axis === "vertical" || axis === "both";
  const readsHorizontal = axis === "horizontal" || axis === "both";
  const viewportHeight = element.clientHeight;
  const viewportWidth = element.clientWidth;
  const contentHeight = element.scrollHeight;
  const contentWidth =
    readsHorizontal && contentElement
      ? Math.max(contentElement.scrollWidth, contentElement.clientWidth)
      : element.scrollWidth;
  const maxScrollTop = Math.max(0, contentHeight - viewportHeight);
  const maxScrollLeft = Math.max(0, contentWidth - viewportWidth);

  return {
    canScrollBottom: readsVertical && maxScrollTop - element.scrollTop > EDGE_TOLERANCE_PX,
    canScrollLeft: readsHorizontal && element.scrollLeft > EDGE_TOLERANCE_PX,
    canScrollRight: readsHorizontal && maxScrollLeft - element.scrollLeft > EDGE_TOLERANCE_PX,
    canScrollTop: readsVertical && element.scrollTop > EDGE_TOLERANCE_PX,
    isScrolling,
  };
}

export function useKanbanOverflow(
  scrollRef: RefObject<HTMLElement | null>,
  { axis = "both", contentRef, revision }: KanbanOverflowOptions = {},
): KanbanOverflowState {
  const [state, setState] = useState<KanbanOverflowState>(EMPTY_OVERFLOW);
  const frameRef = useRef<number | null>(null);
  const idleTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const disposedRef = useRef(false);

  const updateGeometry = useCallback(() => {
    if (disposedRef.current) return;
    setState((previous) => {
      const next = readOverflowState(
        scrollRef.current,
        axis,
        previous.isScrolling,
        contentRef?.current ?? null,
      );
      return isStateEqual(previous, next) ? previous : next;
    });
  }, [axis, contentRef, scrollRef]);

  const scheduleGeometryUpdate = useCallback(() => {
    if (disposedRef.current || frameRef.current !== null) return;
    frameRef.current = window.requestAnimationFrame(() => {
      frameRef.current = null;
      updateGeometry();
    });
  }, [updateGeometry]);

  const handleScroll = useCallback(() => {
    if (disposedRef.current) return;
    setState((previous) => (previous.isScrolling ? previous : { ...previous, isScrolling: true }));
    if (idleTimerRef.current !== null) clearTimeout(idleTimerRef.current);
    idleTimerRef.current = setTimeout(() => {
      idleTimerRef.current = null;
      setState((previous) =>
        previous.isScrolling ? { ...previous, isScrolling: false } : previous,
      );
    }, SCROLL_IDLE_DELAY_MS);
    scheduleGeometryUpdate();
  }, [scheduleGeometryUpdate]);

  useLayoutEffect(() => {
    disposedRef.current = false;
    const scrollElement = scrollRef.current;
    if (!scrollElement) return undefined;
    const contentElement =
      contentRef?.current ??
      (scrollElement.firstElementChild instanceof HTMLElement
        ? scrollElement.firstElementChild
        : null);

    scrollElement.addEventListener("scroll", handleScroll, { passive: true });
    const observer =
      typeof ResizeObserver === "undefined"
        ? null
        : new ResizeObserver(() => {
            scheduleGeometryUpdate();
          });
    observer?.observe(scrollElement);
    if (contentElement && contentElement !== scrollElement) observer?.observe(contentElement);
    updateGeometry();
    scheduleGeometryUpdate();

    return () => {
      disposedRef.current = true;
      scrollElement.removeEventListener("scroll", handleScroll);
      observer?.disconnect();
      if (frameRef.current !== null) {
        window.cancelAnimationFrame(frameRef.current);
        frameRef.current = null;
      }
      if (idleTimerRef.current !== null) {
        clearTimeout(idleTimerRef.current);
        idleTimerRef.current = null;
      }
    };
  }, [contentRef, handleScroll, scheduleGeometryUpdate, scrollRef, updateGeometry]);

  useEffect(() => {
    scheduleGeometryUpdate();
  }, [revision, scheduleGeometryUpdate]);

  return state;
}
