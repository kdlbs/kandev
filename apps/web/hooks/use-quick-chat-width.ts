"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { getLocalStorage, setLocalStorage } from "@/lib/local-storage";

const STORAGE_KEY = "kandev.quickChat.dialogWidth";
const DEFAULT_VIEWPORT_RATIO = 0.8;
const MIN_WIDTH = 480;
const VIEWPORT_MARGIN = 32;

export const QUICK_CHAT_WIDTH_LIMITS = {
  storageKey: STORAGE_KEY,
  defaultViewportRatio: DEFAULT_VIEWPORT_RATIO,
  minWidth: MIN_WIDTH,
  viewportMargin: VIEWPORT_MARGIN,
} as const;

function viewportWidth(): number {
  return typeof window === "undefined" ? 1200 : window.innerWidth;
}

export function clampQuickChatWidth(width: number, availableWidth = viewportWidth()): number {
  const maximum = Math.max(0, availableWidth - VIEWPORT_MARGIN);
  const minimum = Math.min(MIN_WIDTH, maximum);
  return Math.round(Math.min(Math.max(width, minimum), maximum));
}

function readInitialWidth(): number {
  const availableWidth = viewportWidth();
  const stored = getLocalStorage<number | null>(STORAGE_KEY, null);
  const preferred =
    typeof stored === "number" && Number.isFinite(stored)
      ? stored
      : availableWidth * DEFAULT_VIEWPORT_RATIO;
  return clampQuickChatWidth(preferred, availableWidth);
}

function restoreBodyStyles() {
  document.body.style.cursor = "";
  document.body.style.userSelect = "";
}

type ResizeEdge = "left" | "right";

export function useQuickChatWidth() {
  const [width, setWidth] = useState(readInitialWidth);
  const widthRef = useRef(width);
  const dragRef = useRef<{
    edge: ResizeEdge;
    startX: number;
    startWidth: number;
    dialog: HTMLElement;
  } | null>(null);

  const updateWidth = useCallback((nextWidth: number) => {
    widthRef.current = nextWidth;
    setWidth(nextWidth);
  }, []);

  const startResize = useCallback((edge: ResizeEdge, event: React.MouseEvent) => {
    event.preventDefault();
    const dialog = event.currentTarget.parentElement;
    if (!dialog) return;
    dragRef.current = { edge, startX: event.clientX, startWidth: widthRef.current, dialog };
    document.body.style.cursor = "ew-resize";
    document.body.style.userSelect = "none";
  }, []);

  const handleLeftMouseDown = useCallback(
    (event: React.MouseEvent) => startResize("left", event),
    [startResize],
  );
  const handleRightMouseDown = useCallback(
    (event: React.MouseEvent) => startResize("right", event),
    [startResize],
  );

  useEffect(() => {
    const handleMouseMove = (event: MouseEvent) => {
      const drag = dragRef.current;
      if (!drag) return;
      const pointerDelta = event.clientX - drag.startX;
      const widthDelta = (drag.edge === "right" ? pointerDelta : -pointerDelta) * 2;
      const nextWidth = clampQuickChatWidth(drag.startWidth + widthDelta);
      widthRef.current = nextWidth;
      drag.dialog.style.setProperty("--quick-chat-width", `${nextWidth}px`);
    };
    const handleMouseUp = () => {
      if (!dragRef.current) return;
      dragRef.current = null;
      restoreBodyStyles();
      setWidth(widthRef.current);
      setLocalStorage(STORAGE_KEY, widthRef.current);
    };
    const handleWindowResize = () => {
      const nextWidth = clampQuickChatWidth(widthRef.current);
      const drag = dragRef.current;
      if (drag) {
        widthRef.current = nextWidth;
        drag.dialog.style.setProperty("--quick-chat-width", `${nextWidth}px`);
        return;
      }
      updateWidth(nextWidth);
    };

    document.addEventListener("mousemove", handleMouseMove);
    document.addEventListener("mouseup", handleMouseUp);
    window.addEventListener("resize", handleWindowResize);
    return () => {
      document.removeEventListener("mousemove", handleMouseMove);
      document.removeEventListener("mouseup", handleMouseUp);
      window.removeEventListener("resize", handleWindowResize);
      if (dragRef.current) {
        dragRef.current = null;
        restoreBodyStyles();
      }
    };
  }, [updateWidth]);

  return {
    width,
    leftResizeHandleProps: { onMouseDown: handleLeftMouseDown },
    rightResizeHandleProps: { onMouseDown: handleRightMouseDown },
  };
}
