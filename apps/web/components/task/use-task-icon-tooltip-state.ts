"use client";

import { useRef, useState, type FocusEvent, type PointerEvent } from "react";
import { useHoverPopover } from "@/components/integrations/use-hover-popover";

type TaskIconTooltipStateOptions = {
  hoverable?: boolean;
};

const HOVER_CLOSE_DELAY_MS = 150;

function isEditableTarget(target: EventTarget | null): boolean {
  return (
    target instanceof HTMLElement &&
    (target.matches("input, textarea, select") || target.isContentEditable)
  );
}

function isFocusVisibleTarget(target: EventTarget | null): target is HTMLElement {
  return target instanceof HTMLElement && target.matches(":focus-visible");
}

/** Shared fine-pointer and keyboard disclosure behavior for compact task indicators. */
export function useTaskIconTooltipState(
  onOpen?: () => void,
  options: TaskIconTooltipStateOptions = {},
) {
  const hoverable = options.hoverable ?? false;
  const [hovered, setHovered] = useState(false);
  const [focused, setFocused] = useState(false);
  const [dismissed, setDismissed] = useState(false);
  const triggerFocused = useRef(false);
  const triggerFocusVisible = useRef(false);
  const contentFocusVisible = useRef(false);
  const suppressFocusOpen = useRef(false);
  const hoverPopover = useHoverPopover({
    openDelayMs: 0,
    closeDelayMs: HOVER_CLOSE_DELAY_MS,
    disabled: !hoverable,
  });

  const openHoverable = (event: { type?: string }) => {
    suppressFocusOpen.current = false;
    setDismissed(false);
    hoverPopover.onTriggerEnter(event);
    hoverPopover.onOpenChange(true);
    onOpen?.();
  };

  return {
    open: !dismissed && (hoverable ? hoverPopover.open : hovered || focused),
    onPointerEnter(event: PointerEvent<HTMLSpanElement>) {
      if (event.pointerType !== "mouse") return;
      if (hoverable) {
        openHoverable(event);
        return;
      }
      setHovered(true);
      setDismissed(false);
      onOpen?.();
    },
    onPointerLeave(event: PointerEvent<HTMLSpanElement>) {
      if (event.pointerType !== "mouse") return;
      if (hoverable) return hoverPopover.onTriggerLeave(event);
      setHovered(false);
      if (!focused) setDismissed(false);
    },
    onFocus(event: FocusEvent<HTMLSpanElement>) {
      triggerFocused.current = true;
      if (!event.currentTarget.matches(":focus-visible")) return;
      triggerFocusVisible.current = true;
      if (hoverable) {
        if (suppressFocusOpen.current) {
          suppressFocusOpen.current = false;
          return;
        }
        openHoverable(event);
        return;
      }
      setFocused(true);
      setDismissed(false);
      onOpen?.();
    },
    onBlur(event: FocusEvent<HTMLSpanElement>) {
      triggerFocused.current = false;
      if (hoverable) {
        if (!triggerFocusVisible.current) return;
        triggerFocusVisible.current = false;
        hoverPopover.onTriggerLeave(event);
        return;
      }
      setFocused(false);
      if (!hovered) setDismissed(false);
    },
    onContentPointerEnter(event: PointerEvent<HTMLDivElement>) {
      if (!hoverable || event.pointerType !== "mouse") return;
      setDismissed(false);
      hoverPopover.onContentEnter(event);
    },
    onContentPointerLeave(event: PointerEvent<HTMLDivElement>) {
      if (!hoverable || event.pointerType !== "mouse") return;
      hoverPopover.onContentLeave(event);
    },
    onContentFocus(event: FocusEvent<HTMLDivElement>) {
      if (!hoverable || !isFocusVisibleTarget(event.target)) return;
      contentFocusVisible.current = true;
      setDismissed(false);
      hoverPopover.onContentEnter(event);
    },
    onContentBlur(event: FocusEvent<HTMLDivElement>) {
      if (!hoverable || !contentFocusVisible.current) return;
      contentFocusVisible.current = false;
      hoverPopover.onContentLeave(event);
    },
    onEscapeKeyDown(event: Event) {
      if (isEditableTarget(event.target)) return false;
      suppressFocusOpen.current = hoverable && !triggerFocused.current;
      setDismissed(true);
      if (hoverable) hoverPopover.onOpenChange(false);
      return true;
    },
  };
}
