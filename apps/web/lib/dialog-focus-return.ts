import type { RefObject } from "react";

/**
 * Builds a Radix `onCloseAutoFocus` handler that returns keyboard focus to
 * `focusReturnRef` on close. Radix's own default restores focus to whatever
 * element had it when the dialog opened, which has nothing valid to return to
 * when the dialog was opened from a menu item that has already unmounted
 * (e.g. the task actions menu closes on entry activation before its dialog
 * opens).
 */
export function createFocusReturnHandler(focusReturnRef?: RefObject<HTMLElement | null>) {
  return (event: Event) => {
    const target = focusReturnRef?.current;
    if (!target || !document.contains(target)) return;
    event.preventDefault();
    target.focus();
  };
}
