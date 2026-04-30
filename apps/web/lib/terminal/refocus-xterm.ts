/**
 * Synchronously re-focus xterm.js's hidden textarea after a virtual-key tap.
 *
 * Why: on iOS Safari, tapping a button in a virtual key-bar can dismiss the
 * soft keyboard even with preventDefault on pointerdown/mousedown, because
 * WebKit transfers focus on touchend. Calling .focus() on the xterm textarea
 * inside the same user gesture (the onClick handler) re-shows the keyboard.
 */
export function refocusXtermTextarea(): void {
  if (typeof document === "undefined") return;
  const textarea = document.querySelector<HTMLTextAreaElement>(".xterm-helper-textarea");
  textarea?.focus({ preventScroll: true });
}
