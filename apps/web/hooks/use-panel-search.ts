"use client";

import { useEffect } from "react";
import { SHORTCUTS } from "@/lib/keyboard/constants";
import { matchesShortcut } from "@/lib/keyboard/utils";

type UsePanelSearchParams = {
  containerRef: React.RefObject<HTMLElement | null>;
  isOpen: boolean;
  onOpen: () => void;
  onClose: () => void;
  /** When true, suppress focus-within check — useful when panel uses a
   * focus-stealing child (e.g. xterm) and registers its own key capture. */
  alwaysActive?: boolean;
};

/**
 * Module-level tracker of the most recently interacted panel container.
 * When focus is on `<body>` (no input focused — the typical state when a user
 * is just scrolling/reading messages), the panel they last clicked into still
 * "owns" the Ctrl+F shortcut. Without this, the chat panel could never claim
 * Ctrl+F because the message list isn't focusable.
 */
let lastInteractedPanel: HTMLElement | null = null;

function isFocusWithin(container: HTMLElement | null): boolean {
  if (!container) return false;
  const active = document.activeElement;
  if (!active) return false;
  if (container === active) return true;
  return container.contains(active);
}

/** True when nothing meaningful is focused (typically body or null). */
function isAmbientFocus(): boolean {
  const active = document.activeElement;
  if (!active) return true;
  if (active === document.body) return true;
  if (active.tagName === "HTML") return true;
  return false;
}

function shouldClaim(container: HTMLElement | null, alwaysActive: boolean): boolean {
  if (alwaysActive) return true;
  if (isFocusWithin(container)) return true;
  // Fallback: focus is ambient — let the most recently clicked panel claim it.
  if (container && isAmbientFocus() && lastInteractedPanel === container) return true;
  return false;
}

export function usePanelSearch({
  containerRef,
  isOpen,
  onOpen,
  onClose,
  alwaysActive = false,
}: UsePanelSearchParams): void {
  useEffect(() => {
    if (alwaysActive) return;
    let claimedContainer: HTMLElement | null = null;
    // Attach at the window in capture phase so we see the event before any
    // descendant can stopPropagation. Target-containment check avoids relying
    // on the capture descent reaching the panel container.
    const onPointerDown = (event: PointerEvent) => {
      const container = containerRef.current;
      if (!container) return;
      const target = event.target;
      if (target instanceof Node && container.contains(target)) {
        lastInteractedPanel = container;
        claimedContainer = container;
      }
    };
    window.addEventListener("pointerdown", onPointerDown, true);
    return () => {
      window.removeEventListener("pointerdown", onPointerDown, true);
      if (claimedContainer && lastInteractedPanel === claimedContainer) {
        lastInteractedPanel = null;
      }
    };
  }, [containerRef, alwaysActive]);

  useEffect(() => {
    const handler = (event: KeyboardEvent) => {
      if (matchesShortcut(event, SHORTCUTS.FIND_IN_PANEL)) {
        if (!shouldClaim(containerRef.current, alwaysActive)) return;
        event.preventDefault();
        event.stopPropagation();
        onOpen();
        return;
      }
      if (isOpen && event.key === "Escape") {
        if (!shouldClaim(containerRef.current, alwaysActive)) return;
        event.preventDefault();
        event.stopPropagation();
        onClose();
      }
    };
    window.addEventListener("keydown", handler, true);
    return () => window.removeEventListener("keydown", handler, true);
  }, [containerRef, isOpen, onOpen, onClose, alwaysActive]);
}
