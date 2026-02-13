'use client';

import { useCallback, useEffect, useRef, useState } from 'react';

const MIN_HEIGHT = 100;
const DEFAULT_HEIGHT = 134;
const MAX_ABSOLUTE = 800;

export function useResizableInput() {
  const [height, setHeight] = useState(DEFAULT_HEIGHT);
  const containerRef = useRef<HTMLDivElement>(null);
  const isDragging = useRef(false);
  const startY = useRef(0);
  const startHeight = useRef(0);

  const getMaxHeight = useCallback(() => {
    // Walk up to find the nearest panel-level ancestor (the full-height flex container)
    let el = containerRef.current?.parentElement;
    while (el) {
      if (el.clientHeight > MAX_ABSOLUTE * 0.5) {
        return Math.min(el.clientHeight * 0.8, MAX_ABSOLUTE);
      }
      el = el.parentElement;
    }
    // Fallback: use viewport height
    return Math.min(window.innerHeight * 0.7, MAX_ABSOLUTE);
  }, []);

  const handleMouseDown = useCallback(
    (e: React.MouseEvent) => {
      e.preventDefault();
      isDragging.current = true;
      startY.current = e.clientY;
      startHeight.current = height;
      document.body.style.cursor = 'ns-resize';
      document.body.style.userSelect = 'none';
    },
    [height]
  );

  const handleDoubleClick = useCallback(() => {
    setHeight(DEFAULT_HEIGHT);
  }, []);

  useEffect(() => {
    const handleMouseMove = (e: MouseEvent) => {
      if (!isDragging.current) return;
      // Dragging UP (clientY decreases) increases height
      const delta = startY.current - e.clientY;
      const maxHeight = getMaxHeight();
      const newHeight = Math.min(Math.max(startHeight.current + delta, MIN_HEIGHT), maxHeight);
      setHeight(newHeight);
    };

    const handleMouseUp = () => {
      if (!isDragging.current) return;
      isDragging.current = false;
      document.body.style.cursor = '';
      document.body.style.userSelect = '';
    };

    document.addEventListener('mousemove', handleMouseMove);
    document.addEventListener('mouseup', handleMouseUp);
    return () => {
      document.removeEventListener('mousemove', handleMouseMove);
      document.removeEventListener('mouseup', handleMouseUp);
    };
  }, [getMaxHeight]);

  return {
    height,
    containerRef,
    resizeHandleProps: {
      onMouseDown: handleMouseDown,
      onDoubleClick: handleDoubleClick,
    },
  };
}
