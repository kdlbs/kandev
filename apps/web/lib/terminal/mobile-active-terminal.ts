/**
 * Tracks the current input target for the mobile terminal key-bar.
 *
 * Mobile renders multiple terminals via PassthroughTerminal, each with its own
 * WebSocket. The shared key-bar (Ctrl, ^C, arrow keys, etc.) needs to dispatch
 * keystrokes to whichever terminal is on screen, not the per-session default
 * shell. The active terminal registers its raw sender and cursor-mode reader;
 * the key-bar falls back to the default shell when no target is registered.
 */

type Sender = (data: string) => boolean | void;

type Target = { sender: Sender; sessionId?: string; applicationCursorMode?: () => boolean };
let activeTarget: Target | null = null;

export function setActiveTerminalSender(
  sender: Sender | null,
  sessionId?: string,
  applicationCursorMode?: () => boolean,
): () => void {
  const target = sender ? { sender, sessionId, applicationCursorMode } : null;
  activeTarget = target;
  return () => {
    if (activeTarget === target) activeTarget = null;
  };
}

export function getActiveTerminalTarget(sessionId?: string): Target | null {
  if (sessionId && activeTarget?.sessionId && sessionId !== activeTarget.sessionId) return null;
  return activeTarget;
}

export function getActiveTerminalSender(sessionId?: string): Sender | null {
  return getActiveTerminalTarget(sessionId)?.sender ?? null;
}
