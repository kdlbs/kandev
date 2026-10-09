"use client";

import {
  useCallback,
  useEffect,
  useRef,
  useState,
  type FocusEvent,
  type PointerEvent,
  type RefObject,
} from "react";
import { useHoverPopover } from "@/components/integrations/use-hover-popover";

type TaskIconTooltipStateOptions = {
  hoverable?: boolean;
  openDelayMs?: number;
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

function useOnOpenTransition(open: boolean, onOpen?: () => void) {
  const previousOpen = useRef(false);
  const onOpenRef = useRef(onOpen);

  useEffect(() => {
    onOpenRef.current = onOpen;
  }, [onOpen]);

  useEffect(() => {
    if (!open) {
      previousOpen.current = false;
      return;
    }
    if (previousOpen.current) return;
    previousOpen.current = true;
    onOpenRef.current?.();
  }, [open]);
}

function useTooltipEscape({
  hoverable,
  pendingHover,
  triggerFocused,
  suppressFocusOpen,
  setDismissed,
  onOpenChange,
}: {
  hoverable: boolean;
  pendingHover: boolean;
  triggerFocused: RefObject<boolean>;
  suppressFocusOpen: RefObject<boolean>;
  setDismissed: (dismissed: boolean) => void;
  onOpenChange: (open: boolean) => void;
}) {
  const onEscapeKeyDown = useCallback(
    (event: Event) => {
      if (isEditableTarget(event.target)) return false;
      suppressFocusOpen.current = hoverable && !pendingHover && !triggerFocused.current;
      setDismissed(true);
      if (hoverable) onOpenChange(false);
      return true;
    },
    [hoverable, pendingHover, onOpenChange, setDismissed, suppressFocusOpen, triggerFocused],
  );
  useEffect(() => {
    if (!pendingHover) return;
    const onPendingKeyDown = (event: KeyboardEvent) => {
      if (event.key === "Escape") onEscapeKeyDown(event);
    };
    document.addEventListener("keydown", onPendingKeyDown, true);
    return () => document.removeEventListener("keydown", onPendingKeyDown, true);
  }, [pendingHover, onEscapeKeyDown]);
  return onEscapeKeyDown;
}

/** Shared fine-pointer and keyboard disclosure behavior for compact task indicators. */
export function useTaskIconTooltipState(
  onOpen?: () => void,
  options: TaskIconTooltipStateOptions = {},
) {
  const hoverable = options.hoverable ?? false;
  const openDelayMs = options.openDelayMs ?? 0;
  const [hovered, setHovered] = useState(false);
  const [focused, setFocused] = useState(false);
  const [dismissed, setDismissed] = useState(false);
  const triggerFocused = useRef(false);
  const triggerFocusVisible = useRef(false);
  const contentFocusVisible = useRef(false);
  const suppressFocusOpen = useRef(false);
  const hoverPopover = useHoverPopover({
    openDelayMs,
    closeDelayMs: HOVER_CLOSE_DELAY_MS,
    disabled: !hoverable,
  });
  const open = !dismissed && (hoverable ? hoverPopover.open : hovered || focused);
  useOnOpenTransition(open, onOpen);

  const onEscapeKeyDown = useTooltipEscape({
    hoverable,
    pendingHover: hoverable && hovered && !open && !dismissed && openDelayMs > 0,
    triggerFocused,
    suppressFocusOpen,
    setDismissed,
    onOpenChange: hoverPopover.onOpenChange,
  });

  return {
    open,
    onPointerEnter(event: PointerEvent<HTMLSpanElement>) {
      if (event.pointerType !== "mouse") return;
      setHovered(true);
      if (hoverable) {
        suppressFocusOpen.current = false;
        setDismissed(false);
        hoverPopover.onTriggerEnter(event);
        return;
      }
      setDismissed(false);
    },
    onPointerLeave(event: PointerEvent<HTMLSpanElement>) {
      if (event.pointerType !== "mouse") return;
      setHovered(false);
      if (hoverable) return hoverPopover.onTriggerLeave(event);
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
        suppressFocusOpen.current = false;
        setDismissed(false);
        hoverPopover.onTriggerEnter(event);
        hoverPopover.onOpenChange(true);
        return;
      }
      setFocused(true);
      setDismissed(false);
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
    onEscapeKeyDown,
  };
}
