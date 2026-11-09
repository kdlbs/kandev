"use client";

import { useCallback, useEffect, useMemo, useRef, type RefObject } from "react";
import { useDndMonitor } from "@dnd-kit/core";
import { getEventCoordinates } from "@dnd-kit/utilities";

type PointerPosition = { x: number; y: number };
type DragScrollOptions = {
  isDragging: boolean;
  scrollWindowRef: RefObject<HTMLDivElement | null>;
  laneGridRef: RefObject<HTMLDivElement | null>;
};

const EDGE_RATIO = 0.2;
const MAX_SPEED_PX_PER_MS = 2;
const MAX_FRAME_MS = 32;

function scrollTowardPointer(
  viewport: HTMLDivElement,
  grid: HTMLDivElement,
  pointer: PointerPosition,
  elapsed: number,
) {
  const rect = viewport.getBoundingClientRect();
  if (
    viewport.clientWidth <= 0 ||
    pointer.x < rect.left ||
    pointer.x > rect.right ||
    pointer.y < rect.top ||
    pointer.y > rect.bottom
  )
    return;

  const edgeWidth = viewport.clientWidth * EDGE_RATIO;
  const leftIntent = Math.max(0, (rect.left + edgeWidth - pointer.x) / edgeWidth);
  const rightIntent = Math.max(0, (pointer.x - rect.right + edgeWidth) / edgeWidth);
  const movement = (rightIntent - leftIntent) * MAX_SPEED_PX_PER_MS * elapsed;
  const current = viewport.scrollLeft;
  if (movement < 0) {
    viewport.scrollLeft = Math.max(0, current + movement);
  } else if (movement > 0) {
    // The trailing reserve belongs to source anchoring, not automatic travel.
    const contentEnd =
      current + grid.getBoundingClientRect().right - rect.left - viewport.clientLeft;
    const maximum = Math.max(0, contentEnd - viewport.clientWidth);
    if (current < maximum) viewport.scrollLeft = Math.min(maximum, current + movement);
  }
}

function startDragScroll(
  viewport: HTMLDivElement,
  grid: HTMLDivElement,
  initialPointer: PointerPosition | null,
) {
  const owner = viewport.ownerDocument;
  const view = owner.defaultView;
  if (!view) return () => {};

  let pointer = initialPointer;
  let pendingFrame: number | null = null;
  let previousTime: number | null = null;
  const tick = (time: number) => {
    pendingFrame = null;
    if (!pointer) return;
    const elapsed = Math.min(MAX_FRAME_MS, Math.max(0, time - (previousTime ?? time - 16)));
    previousTime = time;
    scrollTowardPointer(viewport, grid, pointer, elapsed);
    pendingFrame = view.requestAnimationFrame(tick);
  };
  const updatePointer = (x: number, y: number) => {
    pointer = { x, y };
    if (pendingFrame === null) pendingFrame = view.requestAnimationFrame(tick);
  };
  const onPointerMove = (event: PointerEvent) => {
    if (event.isPrimary !== false) updatePointer(event.clientX, event.clientY);
  };
  const onTouchMove = (event: TouchEvent) => {
    const touch = event.touches[0];
    if (touch) updatePointer(touch.clientX, touch.clientY);
  };
  const stop = () => {
    pointer = null;
    if (pendingFrame !== null) view.cancelAnimationFrame(pendingFrame);
    pendingFrame = null;
    owner.removeEventListener("pointermove", onPointerMove, true);
    owner.removeEventListener("touchmove", onTouchMove, true);
    owner.removeEventListener("pointerup", onPointerEnd, true);
    owner.removeEventListener("pointercancel", onPointerEnd, true);
    owner.removeEventListener("touchend", stop, true);
    owner.removeEventListener("touchcancel", stop, true);
  };
  const onPointerEnd = (event: PointerEvent) => {
    if (event.isPrimary !== false) stop();
  };
  owner.addEventListener("pointermove", onPointerMove, { capture: true, passive: true });
  owner.addEventListener("touchmove", onTouchMove, { capture: true, passive: true });
  owner.addEventListener("pointerup", onPointerEnd, true);
  owner.addEventListener("pointercancel", onPointerEnd, true);
  owner.addEventListener("touchend", stop, true);
  owner.addEventListener("touchcancel", stop, true);
  pendingFrame = view.requestAnimationFrame(tick);
  return stop;
}

export function useKanbanHorizontalDragScroll({
  isDragging,
  scrollWindowRef,
  laneGridRef,
}: DragScrollOptions) {
  const dispose = useRef<(() => void) | null>(null);
  const latestPointer = useRef<PointerPosition | null>(null);
  const capturePointer = useCallback(({ nativeEvent }: { nativeEvent: Event }) => {
    if ("isPrimary" in nativeEvent && nativeEvent.isPrimary === false) return;
    latestPointer.current = getEventCoordinates(nativeEvent);
  }, []);
  const stop = useCallback(() => {
    dispose.current?.();
    dispose.current = null;
    latestPointer.current = null;
  }, []);
  useDndMonitor(
    useMemo(
      () => ({
        onDragStart: ({ activatorEvent }) => {
          const initialPointer = latestPointer.current ?? getEventCoordinates(activatorEvent);
          stop();
          const viewport = scrollWindowRef.current;
          const grid = laneGridRef.current;
          if (viewport && grid) dispose.current = startDragScroll(viewport, grid, initialPointer);
        },
        onDragEnd: stop,
        onDragCancel: stop,
        onDragAbort: stop,
      }),
      [scrollWindowRef, laneGridRef, stop],
    ),
  );
  useEffect(() => {
    if (!isDragging) stop();
  }, [isDragging, stop]);
  useEffect(() => stop, [stop]);
  return capturePointer;
}
