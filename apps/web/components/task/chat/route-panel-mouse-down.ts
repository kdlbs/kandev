import type { MouseEvent, RefObject } from "react";

const interactiveSelector =
  "input, textarea, select, button, a, [contenteditable], [tabindex]:not([tabindex='-1'])";

export function routePanelMouseDown(
  event: MouseEvent<HTMLDivElement>,
  ref: RefObject<HTMLDivElement | null>,
): void {
  focusRoutePanel(event, ref, false);
}

export function routePanelClick(
  event: MouseEvent<HTMLDivElement>,
  ref: RefObject<HTMLDivElement | null>,
): void {
  focusRoutePanel(event, ref, true);
}

function focusRoutePanel(
  event: MouseEvent<HTMLDivElement>,
  ref: RefObject<HTMLDivElement | null>,
  defer: boolean,
): void {
  const target = event.target as HTMLElement | null;
  if (!target || !ref.current?.contains(target) || target.closest(interactiveSelector)) return;
  const focus = () => ref.current?.focus({ preventScroll: true });
  if (defer) requestAnimationFrame(focus);
  else focus();
}
