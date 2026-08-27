let launcherFocus: HTMLElement | null = null;
const SILENT_FOCUS_ATTRIBUTE = "data-quick-chat-silent-focus";

function markFocusAsSilent(element: HTMLElement): void {
  element.setAttribute(SILENT_FOCUS_ATTRIBUTE, "true");
  element.addEventListener("blur", () => element.removeAttribute(SILENT_FOCUS_ATTRIBUTE), {
    once: true,
  });
}

/** Records the control that opened the shared Quick Chat surface. */
export function captureQuickChatLauncherFocus(): void {
  if (typeof document === "undefined") return;
  launcherFocus = document.activeElement instanceof HTMLElement ? document.activeElement : null;
}

/** Restores launcher focus after the shared dialog has finished closing. */
export function restoreQuickChatLauncherFocus(): void {
  const element = launcherFocus;
  launcherFocus = null;
  if (!element) return;
  requestAnimationFrame(() => {
    if (!element.isConnected) return;
    markFocusAsSilent(element);
    element.focus();
  });
}
