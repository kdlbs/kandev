"use client";

import {
  useCallback,
  useRef,
  useState,
  type MouseEvent as ReactMouseEvent,
  type PointerEvent as ReactPointerEvent,
} from "react";
import {
  resolveAppStatusDropTarget,
  type AppStatusDropTarget,
  type AppStatusItemGeometry,
} from "./app-status-bar-order";

const DRAG_THRESHOLD_PX = 4;

type DragCandidate = {
  itemId: string;
  pointerId: number;
  startX: number;
  dragging: boolean;
  target: AppStatusDropTarget | null;
};

export function useAppStatusBarDrag({
  moveItem,
}: {
  moveItem: (itemId: string, side: "left" | "right", activeIndex: number) => void;
}) {
  const barRef = useRef<HTMLElement>(null);
  const candidate = useRef<DragCandidate | null>(null);
  const suppressedClick = useRef(false);
  const [draggingId, setDraggingId] = useState<string | null>(null);

  const onItemPointerDown = useCallback((itemId: string, event: ReactPointerEvent<HTMLElement>) => {
    if (event.pointerType !== "mouse" || event.button !== 0 || (!event.metaKey && !event.ctrlKey)) {
      return;
    }
    event.preventDefault();
    barRef.current?.setPointerCapture?.(event.pointerId);
    candidate.current = {
      itemId,
      pointerId: event.pointerId,
      startX: event.clientX,
      dragging: false,
      target: null,
    };
  }, []);

  const onPointerMove = useCallback((event: ReactPointerEvent<HTMLElement>) => {
    const current = candidate.current;
    if (!current || current.pointerId !== event.pointerId) return;
    if (!current.dragging && Math.abs(event.clientX - current.startX) <= DRAG_THRESHOLD_PX) return;
    current.dragging = true;
    document.getSelection()?.removeAllRanges();
    setDraggingId(current.itemId);
    current.target = readDropTarget(barRef.current, current.itemId, event.clientX);
    event.preventDefault();
  }, []);

  const finishDrag = useCallback(
    (event: ReactPointerEvent<HTMLElement>, cancelled: boolean) => {
      const current = candidate.current;
      if (!current || current.pointerId !== event.pointerId) return;
      candidate.current = null;
      setDraggingId(null);
      if (!current.dragging) return;
      event.preventDefault();
      suppressedClick.current = true;
      window.setTimeout(() => {
        suppressedClick.current = false;
      }, 0);
      if (!cancelled && current.target) {
        moveItem(current.itemId, current.target.side, current.target.activeIndex);
      }
    },
    [moveItem],
  );

  const onClickCapture = useCallback((event: ReactMouseEvent<HTMLElement>) => {
    if (!suppressedClick.current) return;
    suppressedClick.current = false;
    event.preventDefault();
    event.stopPropagation();
  }, []);

  return {
    barRef,
    draggingId,
    onItemPointerDown,
    barHandlers: {
      onPointerMove,
      onPointerUp: (event: ReactPointerEvent<HTMLElement>) => finishDrag(event, false),
      onPointerCancel: (event: ReactPointerEvent<HTMLElement>) => finishDrag(event, true),
      onClickCapture,
    },
  };
}

function readDropTarget(
  bar: HTMLElement | null,
  draggedItemId: string,
  clientX: number,
): AppStatusDropTarget | null {
  if (!bar) return null;
  const spacer = bar.querySelector<HTMLElement>("[data-testid='app-status-bar-spacer']");
  if (!spacer) return null;
  const items: AppStatusItemGeometry[] = [];
  for (const element of bar.querySelectorAll<HTMLElement>("[data-status-item-id]")) {
    const id = element.dataset.statusItemId;
    const side = element.dataset.statusSide;
    if (!id || id === draggedItemId || (side !== "left" && side !== "right")) continue;
    const rect = element.getBoundingClientRect();
    items.push({ id, side, left: rect.left, right: rect.right });
  }
  const spacerRect = spacer.getBoundingClientRect();
  return resolveAppStatusDropTarget(clientX, items, spacerRect);
}
