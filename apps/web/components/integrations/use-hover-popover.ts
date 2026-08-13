"use client";

import { useCallback, useEffect, useRef, useState } from "react";

/**
 * Provider-neutral hover lifecycle for portalled popovers.
 *
 * Trigger and content are tracked as independent hover regions. The close
 * timer rechecks both regions when it fires, so either browser event ordering
 * across the portal gap keeps the popover open while the pointer is inside.
 */
export function useHoverPopover({
  openDelayMs,
  closeDelayMs,
  disabled = false,
}: {
  openDelayMs: number;
  closeDelayMs: number;
  disabled?: boolean;
}) {
  const [open, setOpen] = useState(false);
  const openTimer = useRef<ReturnType<typeof setTimeout> | null>(null);
  const closeTimer = useRef<ReturnType<typeof setTimeout> | null>(null);
  const overTrigger = useRef(false);
  const overContent = useRef(false);

  const clearOpen = useCallback(() => {
    if (openTimer.current) {
      clearTimeout(openTimer.current);
      openTimer.current = null;
    }
  }, []);
  const clearClose = useCallback(() => {
    if (closeTimer.current) {
      clearTimeout(closeTimer.current);
      closeTimer.current = null;
    }
  }, []);

  const scheduleClose = useCallback(() => {
    if (disabled) return;
    clearOpen();
    clearClose();
    closeTimer.current = setTimeout(() => {
      closeTimer.current = null;
      if (!overTrigger.current && !overContent.current) setOpen(false);
    }, closeDelayMs);
  }, [disabled, clearOpen, clearClose, closeDelayMs]);

  const scheduleOpen = useCallback(() => {
    if (disabled || open || openTimer.current) return;
    openTimer.current = setTimeout(() => {
      openTimer.current = null;
      setOpen(true);
    }, openDelayMs);
  }, [disabled, open, openDelayMs]);

  const onTriggerEnter = useCallback(() => {
    if (disabled) return;
    overTrigger.current = true;
    clearClose();
    scheduleOpen();
  }, [disabled, clearClose, scheduleOpen]);

  const onTriggerLeave = useCallback(() => {
    if (disabled) return;
    overTrigger.current = false;
    scheduleClose();
  }, [disabled, scheduleClose]);

  const onContentEnter = useCallback(() => {
    if (disabled) return;
    overContent.current = true;
    clearClose();
  }, [disabled, clearClose]);

  const onContentLeave = useCallback(() => {
    if (disabled) return;
    overContent.current = false;
    scheduleClose();
  }, [disabled, scheduleClose]);

  const onOpenChange = useCallback(
    (next: boolean) => {
      if (next) {
        setOpen(true);
        return;
      }
      overTrigger.current = false;
      overContent.current = false;
      clearOpen();
      clearClose();
      setOpen(false);
    },
    [clearOpen, clearClose],
  );

  useEffect(
    () => () => {
      clearOpen();
      clearClose();
    },
    [clearOpen, clearClose],
  );

  return {
    open,
    onOpenChange,
    onTriggerEnter,
    onTriggerLeave,
    onContentEnter,
    onContentLeave,
  };
}
