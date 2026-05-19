"use client";

import { useCallback, useEffect, useRef, useState } from "react";

const MIN_HEIGHT = 120;
const MAX_VIEWPORT_RATIO = 0.5;

function viewportHeight(): number {
  return typeof window === "undefined" ? 800 : window.innerHeight;
}

function clampHeight(value: number): number {
  const max = Math.round(viewportHeight() * MAX_VIEWPORT_RATIO);
  return Math.min(Math.max(value, MIN_HEIGHT), max);
}

/**
 * Drives the user-resizable clarification overlay.
 *
 * Default behaviour: the container sizes to its content (`height === null` →
 * caller omits the inline height style). Once the user drags the resize
 * handle, the height switches to an explicit pixel value clamped between
 * MIN_HEIGHT and 50% of the viewport. Double-clicking the handle returns to
 * the auto-sized default.
 *
 * Callers MUST invoke `resetHeight()` when the overlay closes so a fresh
 * clarification starts with auto-sized height instead of inheriting a
 * dragged value.
 */
export function useResizableClarificationOverlay() {
  // null = auto-size to content; number = user-driven pixel height.
  const [height, setHeight] = useState<number | null>(null);
  const containerRef = useRef<HTMLDivElement>(null);
  const isDragging = useRef(false);
  const startY = useRef(0);
  const startHeight = useRef(0);

  const handleMouseDown = useCallback((e: React.MouseEvent) => {
    e.preventDefault();
    // Capture the *currently rendered* height so the drag continues from
    // wherever the overlay sits — whether it's auto-sized or already pinned.
    const measured = containerRef.current?.getBoundingClientRect().height ?? MIN_HEIGHT;
    isDragging.current = true;
    startY.current = e.clientY;
    startHeight.current = measured;
    document.body.style.cursor = "ns-resize";
    document.body.style.userSelect = "none";
  }, []);

  const resetHeight = useCallback(() => setHeight(null), []);

  useEffect(() => {
    const handleMouseMove = (e: MouseEvent) => {
      if (!isDragging.current) return;
      // Dragging the handle UP grows the overlay (it expands toward the top
      // of the screen), so a smaller clientY → larger height.
      const delta = startY.current - e.clientY;
      setHeight(clampHeight(startHeight.current + delta));
    };
    const handleMouseUp = () => {
      if (!isDragging.current) return;
      isDragging.current = false;
      document.body.style.cursor = "";
      document.body.style.userSelect = "";
    };
    document.addEventListener("mousemove", handleMouseMove);
    document.addEventListener("mouseup", handleMouseUp);
    return () => {
      document.removeEventListener("mousemove", handleMouseMove);
      document.removeEventListener("mouseup", handleMouseUp);
      if (isDragging.current) {
        isDragging.current = false;
        document.body.style.cursor = "";
        document.body.style.userSelect = "";
      }
    };
  }, []);

  return {
    height,
    containerRef,
    resetHeight,
    resizeHandleProps: { onMouseDown: handleMouseDown, onDoubleClick: resetHeight },
  };
}
