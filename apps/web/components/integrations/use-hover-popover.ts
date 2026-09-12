"use client";

import { useCallback, useEffect, useRef, useState } from "react";

type HoverEvent = { type?: string };

function isFocusEvent(event?: HoverEvent): boolean {
  return event?.type === "focus" || event?.type === "blur";
}

function setRegionPresence(
  event: HoverEvent | undefined,
  pointerRegion: { current: boolean },
  focusRegion: { current: boolean },
  present: boolean,
) {
  if (isFocusEvent(event)) focusRegion.current = present;
  else pointerRegion.current = present;
}

function hasRegionPresence(...regions: Array<{ current: boolean }>): boolean {
  return regions.some((region) => region.current);
}

function clearRegionPresence(...regions: Array<{ current: boolean }>) {
  for (const region of regions) region.current = false;
}

function enterRegion({
  event,
  disabled,
  pointerRegion,
  focusRegion,
  clearClose,
  scheduleOpen,
}: {
  event?: HoverEvent;
  disabled: boolean;
  pointerRegion: { current: boolean };
  focusRegion: { current: boolean };
  clearClose: () => void;
  scheduleOpen?: () => void;
}) {
  if (disabled) return;
  setRegionPresence(event, pointerRegion, focusRegion, true);
  clearClose();
  scheduleOpen?.();
}

function leaveRegion({
  event,
  disabled,
  pointerRegion,
  focusRegion,
  scheduleClose,
}: {
  event?: HoverEvent;
  disabled: boolean;
  pointerRegion: { current: boolean };
  focusRegion: { current: boolean };
  scheduleClose: () => void;
}) {
  if (disabled) return;
  setRegionPresence(event, pointerRegion, focusRegion, false);
  scheduleClose();
}

function useHoverRegionHandlers({
  disabled,
  overTrigger,
  triggerFocused,
  overContent,
  contentFocused,
  clearClose,
  scheduleOpen,
  scheduleClose,
}: {
  disabled: boolean;
  overTrigger: { current: boolean };
  triggerFocused: { current: boolean };
  overContent: { current: boolean };
  contentFocused: { current: boolean };
  clearClose: () => void;
  scheduleOpen: () => void;
  scheduleClose: () => void;
}) {
  const onTriggerEnter = useCallback(
    (event?: HoverEvent) =>
      enterRegion({
        event,
        disabled,
        pointerRegion: overTrigger,
        focusRegion: triggerFocused,
        clearClose,
        scheduleOpen,
      }),
    [disabled, clearClose, scheduleOpen],
  );
  const onTriggerLeave = useCallback(
    (event?: HoverEvent) =>
      leaveRegion({
        event,
        disabled,
        pointerRegion: overTrigger,
        focusRegion: triggerFocused,
        scheduleClose,
      }),
    [disabled, scheduleClose],
  );
  const onContentEnter = useCallback(
    (event?: HoverEvent) =>
      enterRegion({
        event,
        disabled,
        pointerRegion: overContent,
        focusRegion: contentFocused,
        clearClose,
      }),
    [disabled, clearClose],
  );
  const onContentLeave = useCallback(
    (event?: HoverEvent) =>
      leaveRegion({
        event,
        disabled,
        pointerRegion: overContent,
        focusRegion: contentFocused,
        scheduleClose,
      }),
    [disabled, scheduleClose],
  );

  return { onTriggerEnter, onTriggerLeave, onContentEnter, onContentLeave };
}

/**
 * Provider-neutral hover lifecycle for portalled popovers.
 *
 * Trigger and content are tracked as independent pointer and focus regions.
 * The close timer rechecks all regions when it fires, so portal event ordering
 * or keyboard focus changes cannot close the popover while it remains active.
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
  const triggerFocused = useRef(false);
  const overContent = useRef(false);
  const contentFocused = useRef(false);

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
      if (!hasRegionPresence(overTrigger, triggerFocused, overContent, contentFocused)) {
        setOpen(false);
      }
    }, closeDelayMs);
  }, [disabled, clearOpen, clearClose, closeDelayMs]);

  const scheduleOpen = useCallback(() => {
    if (disabled || open || openTimer.current) return;
    openTimer.current = setTimeout(() => {
      openTimer.current = null;
      setOpen(true);
    }, openDelayMs);
  }, [disabled, open, openDelayMs]);

  const { onTriggerEnter, onTriggerLeave, onContentEnter, onContentLeave } = useHoverRegionHandlers(
    {
      disabled,
      overTrigger,
      triggerFocused,
      overContent,
      contentFocused,
      clearClose,
      scheduleOpen,
      scheduleClose,
    },
  );

  const onOpenChange = useCallback(
    (next: boolean) => {
      if (next) {
        setOpen(true);
        return;
      }
      clearRegionPresence(overTrigger, triggerFocused, overContent, contentFocused);
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
