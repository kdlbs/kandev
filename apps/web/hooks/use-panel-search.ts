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

function isFocusWithin(container: HTMLElement | null): boolean {
  if (!container) return false;
  const active = document.activeElement;
  if (!active) return false;
  if (container === active) return true;
  return container.contains(active);
}

export function usePanelSearch({
  containerRef,
  isOpen,
  onOpen,
  onClose,
  alwaysActive = false,
}: UsePanelSearchParams): void {
  useEffect(() => {
    const handler = (event: KeyboardEvent) => {
      if (matchesShortcut(event, SHORTCUTS.FIND_IN_PANEL)) {
        if (!alwaysActive && !isFocusWithin(containerRef.current)) return;
        event.preventDefault();
        event.stopPropagation();
        onOpen();
        return;
      }
      if (isOpen && event.key === "Escape") {
        if (!alwaysActive && !isFocusWithin(containerRef.current)) return;
        event.preventDefault();
        event.stopPropagation();
        onClose();
      }
    };
    window.addEventListener("keydown", handler, true);
    return () => window.removeEventListener("keydown", handler, true);
  }, [containerRef, isOpen, onOpen, onClose, alwaysActive]);
}
