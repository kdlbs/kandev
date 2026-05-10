/**
 * Tracks the current input target for the mobile terminal key-bar.
 *
 * Mobile renders multiple terminals via PassthroughTerminal, each with its own
 * WebSocket. The shared key-bar (Ctrl, ^C, arrow keys, etc.) needs to dispatch
 * keystrokes to whichever terminal is on screen, not the per-session default
 * shell. The active terminal registers a `paste` sender on mount; the key-bar
 * reads from this registry and falls back to the default shell otherwise.
 */

type Sender = (data: string) => void;

let activeSender: Sender | null = null;

export function setActiveTerminalSender(sender: Sender | null): void {
  activeSender = sender;
}

export function getActiveTerminalSender(): Sender | null {
  return activeSender;
}
