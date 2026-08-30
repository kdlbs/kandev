let launcherFocus: HTMLElement | null = null;
let launcherFocusShouldBeSilent = false;
let quickChatCloseHandler: (() => void) | null = null;
const SILENT_FOCUS_ATTRIBUTE = "data-quick-chat-silent-focus";

function markFocusAsSilent(element: HTMLElement): void {
  element.setAttribute(SILENT_FOCUS_ATTRIBUTE, "true");
  element.addEventListener("blur", () => element.removeAttribute(SILENT_FOCUS_ATTRIBUTE), {
    once: true,
  });
}

/** Records the control that opened the shared Quick Chat surface. */
export function captureQuickChatLauncherFocus(options: { silent?: boolean } = {}): void {
  launcherFocusShouldBeSilent = false;
  if (typeof document === "undefined") return;
  launcherFocus = document.activeElement instanceof HTMLElement ? document.activeElement : null;
  launcherFocusShouldBeSilent = options.silent ?? true;
}

/** Restores launcher focus after the shared dialog has finished closing. */
export function restoreQuickChatLauncherFocus(): void {
  const element = launcherFocus;
  const shouldSilenceFocus = launcherFocusShouldBeSilent;
  launcherFocus = null;
  launcherFocusShouldBeSilent = false;
  if (!element) return;
  requestAnimationFrame(() => {
    if (!element.isConnected) return;
    if (shouldSilenceFocus) markFocusAsSilent(element);
    element.focus();
  });
}

/** Registers the mounted modal's close lifecycle for global launchers. */
export function registerQuickChatCloseHandler(handler: () => void): () => void {
  quickChatCloseHandler = handler;
  return () => {
    if (quickChatCloseHandler === handler) quickChatCloseHandler = null;
  };
}

/** Closes Quick Chat through its mounted modal lifecycle when available. */
export function requestQuickChatClose(): boolean {
  const handler = quickChatCloseHandler;
  if (!handler) return false;
  handler();
  return true;
}
