import { useDockviewStore } from "./dockview-store";

let nextRestoreToken = 0;

/**
 * Capture the current chat scroll position and restore it after the next
 * layout rebuild completes (isRestoringLayout transitions to false).
 * Safe to call from non-React code (WS handlers, store actions).
 */
export function preserveChatScrollDuringLayout(): void {
  const scrollEl = document.querySelector<HTMLElement>(".chat-message-list");
  const savedScrollTop = scrollEl?.scrollTop ?? 0;
  const sessionId = scrollEl?.dataset.sessionId ?? null;
  const token = ++nextRestoreToken;

  useDockviewStore.getState().setPendingChatScrollTop({
    scrollTop: savedScrollTop,
    sessionId,
    token,
  });

  // Wait for the full isRestoringLayout transition (false → true → false).
  // Callers may emit non-layout updates (e.g. pinnedWidths) before flipping
  // isRestoringLayout to true; without this guard the subscriber would
  // unsubscribe on the first false-state emit and skip the real restore.
  let sawRestoring = useDockviewStore.getState().isRestoringLayout;
  const unsub = useDockviewStore.subscribe((state) => {
    if (state.isRestoringLayout) {
      sawRestoring = true;
      return;
    }
    if (!sawRestoring) return;
    unsub();
    requestAnimationFrame(() => {
      const state = useDockviewStore.getState();
      if (state.pendingChatScrollTop?.token !== token) return;
      const el = document.querySelector<HTMLElement>(".chat-message-list");
      const applied = (el?.dataset.sessionId ?? null) === sessionId;
      if (el && applied) el.scrollTop = savedScrollTop;
      state.completePendingChatScrollTop(token, applied);
    });
  });
}
