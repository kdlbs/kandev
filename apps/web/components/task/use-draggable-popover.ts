"use client";

import {
  useCallback,
  useEffect,
  useRef,
  useState,
  type MouseEvent as ReactMouseEvent,
  type RefObject,
} from "react";

type AnchorPosition = { x: number; y: number };
type PopoverPosition = { left: number; top: number };
type DragState = { startX: number; startY: number; origLeft: number; origTop: number };

export function computePopoverInitialPos(
  position: AnchorPosition,
  width: number,
  height: number,
): PopoverPosition {
  if (typeof window === "undefined") return { left: position.x, top: position.y };

  let left = position.x;
  let top = position.y;
  if (left + width > window.innerWidth - 16) left = Math.max(16, window.innerWidth - width - 16);
  if (top + height > window.innerHeight - 16) top = Math.max(16, position.y - height);
  return { left, top };
}

export function useDraggablePopover(position: AnchorPosition, width: number, height: number) {
  const [pos, setPos] = useState(() => computePopoverInitialPos(position, width, height));
  const dragRef = useRef<DragState | null>(null);

  const onDragStart = useCallback(
    (event: ReactMouseEvent) => {
      event.preventDefault();
      dragRef.current = {
        startX: event.clientX,
        startY: event.clientY,
        origLeft: pos.left,
        origTop: pos.top,
      };
      const onMove = (moveEvent: MouseEvent) => {
        if (!dragRef.current) return;
        const dx = moveEvent.clientX - dragRef.current.startX;
        const dy = moveEvent.clientY - dragRef.current.startY;
        setPos({ left: dragRef.current.origLeft + dx, top: dragRef.current.origTop + dy });
      };
      const onUp = () => {
        dragRef.current = null;
        document.removeEventListener("mousemove", onMove);
        document.removeEventListener("mouseup", onUp);
      };
      document.addEventListener("mousemove", onMove);
      document.addEventListener("mouseup", onUp);
    },
    [pos.left, pos.top],
  );

  return { pos, onDragStart };
}

export function usePopoverDismiss(
  onClose: () => void,
  popoverRef: RefObject<HTMLDivElement | null>,
) {
  useEffect(() => {
    const handleClickOutside = (event: MouseEvent) => {
      if (popoverRef.current && !popoverRef.current.contains(event.target as Node)) onClose();
    };
    const timer = setTimeout(() => {
      document.addEventListener("mousedown", handleClickOutside);
    }, 100);
    return () => {
      clearTimeout(timer);
      document.removeEventListener("mousedown", handleClickOutside);
    };
  }, [onClose, popoverRef]);

  useEffect(() => {
    const handleKeyDown = (event: KeyboardEvent) => {
      if (event.key === "Escape") {
        event.stopPropagation();
        onClose();
      }
    };
    document.addEventListener("keydown", handleKeyDown, true);
    return () => document.removeEventListener("keydown", handleKeyDown, true);
  }, [onClose]);
}
