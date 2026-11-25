import { useEffect } from "react";

function isEditableTarget(target: EventTarget | null): boolean {
  if (!(target instanceof HTMLElement)) return false;
  const tag = target.tagName;
  if (tag === "INPUT" || tag === "TEXTAREA" || tag === "SELECT") return true;
  return target.isContentEditable;
}

const OPEN_ESCAPE_BOUNDARY = [
  '[data-queue-escape-boundary][data-state="open"]',
  '[data-queue-escape-boundary][data-state="instant-open"]',
  '[data-queue-escape-boundary][data-state="delayed-open"]',
].join(",");

export function useQueuePanelEscape(open: boolean, onClose: () => void): void {
  useEffect(() => {
    if (!open) return;
    const onKey = (event: KeyboardEvent) => {
      if (event.key !== "Escape" || isEditableTarget(event.target)) return;
      // An open nested drawer or tooltip owns Escape even before it receives
      // focus; Radix tooltips report instant-open or delayed-open when shown.
      if (document.querySelector(OPEN_ESCAPE_BOUNDARY)) return;
      event.preventDefault();
      event.stopPropagation();
      onClose();
    };
    window.addEventListener("keydown", onKey, true);
    return () => window.removeEventListener("keydown", onKey, true);
  }, [open, onClose]);
}
