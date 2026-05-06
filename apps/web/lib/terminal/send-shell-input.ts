import { getWebSocketClient } from "@/lib/ws/connection";
import { applyShellModifiers } from "./apply-shell-modifiers";
import { getActiveTerminalSender } from "./mobile-active-terminal";
import { useShellModifiersStore, isActive } from "./shell-modifiers";

/**
 * Single entry point for shell input. Applies active ctrl/shift modifiers,
 * sends over the WS, then consumes latched (non-sticky) modifiers.
 *
 * Used by both the virtual key-bar and xterm's own `onData` callback so that
 * a Ctrl latch set from the key-bar modifies the next character the user
 * types on the OS keyboard — which is the whole point of the modifier.
 *
 * Mobile multi-terminal: when a PassthroughTerminal is registered as the
 * active key-bar target, route input through it (xterm.paste → onData → its
 * dedicated WS) instead of the per-session default shell. Falls back to the
 * default `shell.input` action when no active terminal is registered.
 */
export function sendShellInput(sessionId: string, data: string): void {
  if (!data) return;
  const store = useShellModifiersStore.getState();
  const ctrlActive = isActive(store.ctrl);
  const shiftActive = isActive(store.shift);
  const transformed = applyShellModifiers(data, { ctrl: ctrlActive, shift: shiftActive });

  const activeSender = getActiveTerminalSender();
  if (activeSender) {
    try {
      activeSender(transformed);
    } catch (err) {
      // A stale registry entry (e.g. xterm disposed mid-frame) shouldn't
      // silently drop the keystroke nor consume modifiers. Fall through to
      // the WS path so the input still has a chance of landing.
      console.error("Active terminal sender threw, falling back to shell.input:", err);
      const client = getWebSocketClient();
      if (!client) return;
      client.send({
        type: "request",
        action: "shell.input",
        payload: { session_id: sessionId, data: transformed },
      });
    }
  } else {
    const client = getWebSocketClient();
    if (!client) return; // keep modifiers armed; user can retry once reconnected
    client.send({
      type: "request",
      action: "shell.input",
      payload: { session_id: sessionId, data: transformed },
    });
  }

  if (ctrlActive) store.consumeCtrl();
  if (shiftActive) store.consumeShift();
}
