import type { MouseEvent, RefObject } from "react";

const interactiveSelector =
  "input, textarea, select, button, a, [contenteditable], [tabindex]:not([tabindex='-1'])";

export function routePanelMouseDown(
  event: MouseEvent<HTMLDivElement>,
  ref: RefObject<HTMLDivElement | null>,
): void {
  const target = event.target as HTMLElement | null;
  if (!target || target.closest(interactiveSelector)) return;
  ref.current?.focus({ preventScroll: true });
}
