"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { getChatInputHeight, setChatInputHeight } from "@/lib/local-storage";

const MIN_HEIGHT = 100;
const DEFAULT_HEIGHT = 134;
const MAX_ABSOLUTE = 450;

function clampHeight(value: number): number {
  const maxHeight = Math.min(window.innerHeight * 0.6, MAX_ABSOLUTE);
  return Math.min(Math.max(value, MIN_HEIGHT), maxHeight);
}

/** Measure the natural content height of an editor inside its container. */
function measureContentHeight(container: HTMLElement, content: HTMLElement): number {
  const chrome = container.offsetHeight - content.offsetHeight;
  const prev = content.style.height;
  content.style.height = "0px";
  const naturalHeight = content.scrollHeight;
  content.style.height = prev;
  return clampHeight(naturalHeight + chrome);
}

export function useResizableInput(
  sessionId: string | undefined,
  getContentElement?: () => HTMLElement | null,
) {
  const [height, setHeight] = useState(() => {
    if (typeof window === "undefined" || !sessionId) return DEFAULT_HEIGHT;
    const saved = getChatInputHeight(sessionId);
    return saved ? clampHeight(saved) : DEFAULT_HEIGHT;
  });
  const containerRef = useRef<HTMLDivElement>(null);
  const isDragging = useRef(false);
  const startY = useRef(0);
  const startHeight = useRef(0);
  // Restore height when session changes
  const prevSessionRef = useRef(sessionId);
  useEffect(() => {
    if (sessionId === prevSessionRef.current) return;
    prevSessionRef.current = sessionId;
    /* eslint-disable react-hooks/set-state-in-effect -- syncing from localStorage on session switch */
    const saved = sessionId ? getChatInputHeight(sessionId) : null;
    setHeight(saved ? clampHeight(saved) : DEFAULT_HEIGHT);
    /* eslint-enable react-hooks/set-state-in-effect */
  }, [sessionId]);

  // Persist height after drag ends
  const persistHeight = useCallback(
    (h: number) => {
      if (sessionId) {
        setChatInputHeight(sessionId, h);
      }
    },
    [sessionId],
  );

  const handleMouseDown = useCallback(
    (e: React.MouseEvent) => {
      e.preventDefault();
      isDragging.current = true;
      startY.current = e.clientY;
      startHeight.current = height;
      document.body.style.cursor = "ns-resize";
      document.body.style.userSelect = "none";
    },
    [height],
  );

  const resetHeight = useCallback(() => {
    setHeight(DEFAULT_HEIGHT);
    persistHeight(DEFAULT_HEIGHT);
  }, [persistHeight]);

  useEffect(() => {
    const handleMouseMove = (e: MouseEvent) => {
      if (!isDragging.current) return;
      const delta = startY.current - e.clientY;
      setHeight(clampHeight(startHeight.current + delta));
    };

    const handleMouseUp = () => {
      if (!isDragging.current) return;
      isDragging.current = false;
      document.body.style.cursor = "";
      document.body.style.userSelect = "";
      // Persist the final height after drag
      setHeight((h) => {
        persistHeight(h);
        return h;
      });
    };

    document.addEventListener("mousemove", handleMouseMove);
    document.addEventListener("mouseup", handleMouseUp);
    return () => {
      document.removeEventListener("mousemove", handleMouseMove);
      document.removeEventListener("mouseup", handleMouseUp);
    };
  }, [persistHeight]);

  const autoExpand = useCallback(() => {
    const container = containerRef.current;
    const content = getContentElement?.();
    if (!container || !content || isDragging.current) return;
    const needed = measureContentHeight(container, content);
    if (needed !== height) {
      setHeight(needed);
      persistHeight(needed);
    }
  }, [height, persistHeight, getContentElement]);

  return {
    height,
    resetHeight,
    autoExpand,
    containerRef,
    resizeHandleProps: {
      onMouseDown: handleMouseDown,
      onDoubleClick: resetHeight,
    },
  };
}
