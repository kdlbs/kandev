"use client";

import { useState, type FocusEvent, type PointerEvent } from "react";

/**
 * Controlled open/dismiss state for a keyboard-and-mouse task badge tooltip
 * (PR/MR status). Shared by github/pr-task-icon.tsx and
 * gitlab/mr-task-icon.tsx so both badges agree on hover, focus, and Escape
 * dismissal behavior.
 */
export function useTaskBadgeTooltip() {
  const [hovered, setHovered] = useState(false);
  const [focused, setFocused] = useState(false);
  const [dismissed, setDismissed] = useState(false);

  return {
    open: !dismissed && (hovered || focused),
    onPointerEnter(event: PointerEvent<HTMLSpanElement>) {
      if (event.pointerType !== "mouse") return;
      setHovered(true);
      setDismissed(false);
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
    },
    onBlur() {
      setFocused(false);
      if (!hovered) setDismissed(false);
    },
    onEscapeKeyDown() {
      setDismissed(true);
    },
  };
}
