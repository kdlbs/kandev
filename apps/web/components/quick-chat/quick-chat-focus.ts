let launcherFocus: HTMLElement | null = null;

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
    if (element.isConnected) element.focus();
  });
}
