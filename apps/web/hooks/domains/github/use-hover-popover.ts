"use client";

import { useCallback, useEffect, useRef, useState } from "react";

/**
 * Shared hover lifecycle for the CI popovers (chat-bar chip + top-bar PR
 * button). Both previously kept their own near-identical copy of this timer
 * logic and the same hover-bridge bug had to be fixed in each — see
 * docs history. The single tricky requirement is the "bridge": moving the
 * cursor from the trigger, across the Radix `sideOffset` gap, onto the
 * portalled popover content must NOT close the popover.
 *
 * Why a naive "cancel the close on enter" is not enough: the popover content
 * is rendered in a portal, so the browser/React can dispatch the content's
 * mouseenter *before* the trigger's mouseleave. A single shared handler that
 * cancels the pending close on enter then re-arms it on leave will therefore
 * re-arm the close *after* it was cancelled and the popover vanishes mid-hover.
 *
 * Fix: track the trigger and the content as two independent hover regions and
 * only actually close when the pointer is over *neither* — re-checked at the
 * moment the close timer fires, so event ordering between the two regions no
 * longer matters. The content also treats mouse-move as "enter" so a flaky or
 * missed portal mouseenter can't strand a pending close.
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
  // Whether the pointer is currently over each hover region. The close only
  // commits when both are false, so trigger-leave / content-enter ordering
  // across the portal can't close a popover the pointer is still inside.
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
