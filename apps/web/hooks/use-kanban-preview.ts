"use client";

import { useState, useEffect, useCallback, useRef } from "react";
import { getKanbanPreviewState, setKanbanPreviewState } from "@/lib/local-storage";
import { PREVIEW_PANEL } from "@/lib/settings/constants";

interface UseKanbanPreviewOptions {
  onClose?: () => void;
  initialTaskId?: string;
  /** Another panel holds the right-hand slot; the saved preview stays intact. */
  displaced?: boolean;
}

export function useKanbanPreview(options: UseKanbanPreviewOptions = {}) {
  // Always start with default values to avoid hydration mismatch
  const [selectedTaskId, setSelectedTaskId] = useState<string | null>(null);
  const [isOpenRaw, setIsOpen] = useState(false);
  const [displaced, setDisplaced] = useState(options.displaced ?? false);
  const isOpen = isOpenRaw && !displaced;
  const [previewWidthPx, setPreviewWidthPx] = useState<number>(PREVIEW_PANEL.DEFAULT_WIDTH_PX);
  const containerRef = useRef<HTMLDivElement | null>(null);
  const hasInitialized = useRef(false);

  // Load persisted state from localStorage AFTER hydration
  // Prioritize initialTaskId from SSR over localStorage
  useEffect(() => {
    if (hasInitialized.current) return;
    hasInitialized.current = true;

    const savedState = getKanbanPreviewState({
      isOpen: false,
      previewWidthPx: PREVIEW_PANEL.DEFAULT_WIDTH_PX,
      selectedTaskId: null,
    });

    // Prioritize initial task ID from SSR
    const taskIdToUse = options.initialTaskId ?? savedState.selectedTaskId;

    if (taskIdToUse || savedState.isOpen) {
      // eslint-disable-next-line react-hooks/set-state-in-effect
      setIsOpen(true);
    }
    if (savedState.previewWidthPx) {
      setPreviewWidthPx(Math.max(PREVIEW_PANEL.MIN_WIDTH_PX, savedState.previewWidthPx));
    }
    if (taskIdToUse) {
      setSelectedTaskId(taskIdToUse);
    }
  }, [options.initialTaskId]);

  // Persist state to localStorage
  useEffect(() => {
    setKanbanPreviewState({ isOpen: isOpenRaw });
  }, [isOpenRaw]);

  useEffect(() => {
    if (isOpenRaw && previewWidthPx > 0) {
      setKanbanPreviewState({ previewWidthPx });
    }
  }, [isOpenRaw, previewWidthPx]);

  useEffect(() => {
    setKanbanPreviewState({ selectedTaskId });
  }, [selectedTaskId]);

  const open = useCallback((taskId: string) => {
    setSelectedTaskId(taskId);
    setIsOpen(true);
    setDisplaced(false);
  }, []);

  const close = useCallback(() => {
    setIsOpen(false);
    setSelectedTaskId(null);
    options.onClose?.();
  }, [options]);

  const toggle = useCallback(() => {
    setIsOpen((prev) => !prev);
  }, []);

  const updatePreviewWidth = useCallback((width: number) => {
    setPreviewWidthPx(Math.max(PREVIEW_PANEL.MIN_WIDTH_PX, width));
  }, []);

  return {
    selectedTaskId,
    isOpen,
    previewWidthPx,
    open,
    close,
    toggle,
    displace: setDisplaced,
    setSelectedTaskId,
    updatePreviewWidth,
    containerRef,
  };
}
