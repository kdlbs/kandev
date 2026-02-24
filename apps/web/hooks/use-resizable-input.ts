"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { getChatInputHeight, setChatInputHeight } from "@/lib/local-storage";

const MIN_HEIGHT = 100;
const DEFAULT_HEIGHT = 134;
const MAX_ABSOLUTE = 450;

export function useResizableInput(
  sessionId: string | undefined,
  getContentElement?: () => HTMLElement | null,
) {
  const [height, setHeight] = useState(() => {
    const maxHeight =
      typeof window !== "undefined"
        ? Math.min(window.innerHeight * 0.6, MAX_ABSOLUTE)
        : MAX_ABSOLUTE;
    if (sessionId) {
      const saved = getChatInputHeight(sessionId);
      return saved ? Math.min(saved, maxHeight) : DEFAULT_HEIGHT;
    }
    return DEFAULT_HEIGHT;
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
    if (sessionId) {
      const maxHeight = Math.min(window.innerHeight * 0.6, MAX_ABSOLUTE);
      const saved = getChatInputHeight(sessionId);
      setHeight(saved ? Math.min(saved, maxHeight) : DEFAULT_HEIGHT);
    } else {
      setHeight(DEFAULT_HEIGHT);
    }
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

  const handleDoubleClick = useCallback(() => {
    resetHeight();
  }, [resetHeight]);

  useEffect(() => {
    const handleMouseMove = (e: MouseEvent) => {
      if (!isDragging.current) return;
      // Dragging UP (clientY decreases) increases height
      const delta = startY.current - e.clientY;
      const maxHeight = Math.min(window.innerHeight * 0.6, MAX_ABSOLUTE);
      const newHeight = Math.min(Math.max(startHeight.current + delta, MIN_HEIGHT), maxHeight);
      setHeight(newHeight);
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

  /** Expand or shrink the input to fit content, clamped to [DEFAULT_HEIGHT, maxHeight]. */
  const autoExpand = useCallback(() => {
    const container = containerRef.current;
    const content = getContentElement?.();
    if (!container || !content || isDragging.current) return;
    const maxHeight = Math.min(window.innerHeight * 0.6, MAX_ABSOLUTE);
    // How much extra space the container uses beyond the editor (toolbar, context area, etc.)
    const chrome = container.offsetHeight - content.offsetHeight;
    // Temporarily collapse the editor to measure its natural content height,
    // since it normally fills the container via flex and scrollHeight === offsetHeight.
    const prev = content.style.height;
    content.style.height = "0px";
    const naturalHeight = content.scrollHeight;
    content.style.height = prev;
    const needed = Math.min(Math.max(naturalHeight + chrome, DEFAULT_HEIGHT), maxHeight);
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
      onDoubleClick: handleDoubleClick,
    },
  };
}
