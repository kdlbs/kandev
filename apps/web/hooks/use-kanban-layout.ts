'use client';

import { useEffect, useRef, useState } from 'react';
import { PREVIEW_PANEL } from '@/lib/settings/constants';

/**
 * Custom hook that extracts complex layout calculations for the Kanban board.
 * Handles container measurement, floating mode detection, and width calculations.
 *
 * @param isPreviewOpen - Whether the preview panel is currently open
 * @param previewWidthPx - The width of the preview panel in pixels
 * @returns Layout state including containerRef, shouldFloat flag, and calculated widths
 */
export function useKanbanLayout(isPreviewOpen: boolean, previewWidthPx: number) {
  const containerRef = useRef<HTMLDivElement>(null);
  const [containerWidth, setContainerWidth] = useState<number>(0);

  // Measure container width on mount and resize
  useEffect(() => {
    if (!containerRef.current) return;

    const updateWidth = () => {
      if (containerRef.current) {
        setContainerWidth(containerRef.current.offsetWidth);
      }
    };

    updateWidth();
    const resizeObserver = new ResizeObserver(updateWidth);
    resizeObserver.observe(containerRef.current);

    return () => {
      resizeObserver.disconnect();
    };
  }, []);

  // Calculate if we should be in floating mode
  // Float when preview would make kanban less than minimum percentage
  const minKanbanWidthPx = (PREVIEW_PANEL.MIN_KANBAN_WIDTH_PERCENT / 100) * containerWidth;
  const availableForKanban = containerWidth - previewWidthPx;
  const shouldFloat = isPreviewOpen && containerWidth > 0 && availableForKanban < minKanbanWidthPx;

  // Calculate kanban width - keep it at min width when floating to avoid repainting
  let kanbanWidth = containerWidth;
  if (shouldFloat) {
    kanbanWidth = minKanbanWidthPx;
  } else if (isPreviewOpen) {
    kanbanWidth = containerWidth - previewWidthPx;
  }

  return {
    containerRef,
    shouldFloat,
    kanbanWidth,
    containerWidth,
  };
}
