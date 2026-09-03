"use client";

import { useState, type FocusEvent, type PointerEvent } from "react";

function isEditableTarget(target: EventTarget | null): boolean {
  return (
    target instanceof HTMLElement &&
    (target.matches("input, textarea, select") || target.isContentEditable)
  );
}

/** Shared fine-pointer and keyboard disclosure behavior for compact task indicators. */
export function useTaskIconTooltipState(onOpen?: () => void) {
  const [hovered, setHovered] = useState(false);
  const [focused, setFocused] = useState(false);
  const [dismissed, setDismissed] = useState(false);

  return {
    open: !dismissed && (hovered || focused),
    onPointerEnter(event: PointerEvent<HTMLSpanElement>) {
      if (event.pointerType !== "mouse") return;
      setHovered(true);
      setDismissed(false);
      onOpen?.();
    },
    onPointerLeave(event: PointerEvent<HTMLSpanElement>) {
      if (event.pointerType !== "mouse") return;
      setHovered(false);
      if (!focused) setDismissed(false);
    },
    onFocus(event: FocusEvent<HTMLSpanElement>) {
      if (!event.currentTarget.matches(":focus-visible")) return;
      setFocused(true);
      setDismissed(false);
      onOpen?.();
    },
    onBlur() {
      setFocused(false);
      if (!hovered) setDismissed(false);
    },
    onEscapeKeyDown(event: Event) {
      if (isEditableTarget(event.target)) return;
      setDismissed(true);
    },
  };
}
