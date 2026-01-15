"use client";

import { useState, useEffect, useCallback, useRef } from "react";
import { getKanbanPreviewState, setKanbanPreviewState } from "@/lib/local-storage";
import { PREVIEW_PANEL } from "@/lib/settings/constants";

interface UseKanbanPreviewOptions {
  onClose?: () => void;
}

export function useKanbanPreview(options: UseKanbanPreviewOptions = {}) {
  // Always start with default values to avoid hydration mismatch
  const [selectedTaskId, setSelectedTaskId] = useState<string | null>(null);
  const [isOpen, setIsOpen] = useState(false);
  const [previewWidthPx, setPreviewWidthPx] = useState<number>(PREVIEW_PANEL.DEFAULT_WIDTH_PX);
  const containerRef = useRef<HTMLDivElement | null>(null);

  // Load persisted state from localStorage AFTER hydration
  // This is necessary to avoid hydration mismatch between server and client
  useEffect(() => {
    const savedState = getKanbanPreviewState({
      isOpen: false,
      previewWidthPx: PREVIEW_PANEL.DEFAULT_WIDTH_PX,
      selectedTaskId: null,
    });

    if (savedState.isOpen) {
      // eslint-disable-next-line react-hooks/set-state-in-effect
      setIsOpen(true);
    }
    if (savedState.previewWidthPx) {
      setPreviewWidthPx(Math.max(PREVIEW_PANEL.MIN_WIDTH_PX, savedState.previewWidthPx));
    }
    if (savedState.selectedTaskId) {
      setSelectedTaskId(savedState.selectedTaskId);
    }
  }, []);

  // Persist state to localStorage
  useEffect(() => {
    setKanbanPreviewState({ isOpen });
  }, [isOpen]);

  useEffect(() => {
    if (isOpen && previewWidthPx > 0) {
      setKanbanPreviewState({ previewWidthPx });
    }
  }, [isOpen, previewWidthPx]);

  useEffect(() => {
    setKanbanPreviewState({ selectedTaskId });
  }, [selectedTaskId]);

  const open = useCallback((taskId: string) => {
    setSelectedTaskId(taskId);
    setIsOpen(true);
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
    setSelectedTaskId,
    updatePreviewWidth,
    containerRef,
  };
}
