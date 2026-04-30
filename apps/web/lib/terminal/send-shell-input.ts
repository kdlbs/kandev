import { getWebSocketClient } from "@/lib/ws/connection";
import { applyShellModifiers } from "./apply-shell-modifiers";
import { useShellModifiersStore, isActive } from "./shell-modifiers";

/**
 * Single entry point for shell input. Applies active ctrl/shift modifiers,
 * sends over the WS, then consumes latched (non-sticky) modifiers.
 *
 * Used by both the virtual key-bar and xterm's own `onData` callback so that
 * a Ctrl latch set from the key-bar modifies the next character the user
 * types on the OS keyboard — which is the whole point of the modifier.
 */
export function sendShellInput(sessionId: string, data: string): void {
  if (!data) return;
  const client = getWebSocketClient();
  if (!client) return; // keep modifiers armed; the user can retry once reconnected
  const store = useShellModifiersStore.getState();
  const ctrlActive = isActive(store.ctrl);
  const shiftActive = isActive(store.shift);
  const transformed = applyShellModifiers(data, { ctrl: ctrlActive, shift: shiftActive });
  client.send({
    type: "request",
    action: "shell.input",
    payload: { session_id: sessionId, data: transformed },
  });
  if (ctrlActive) store.consumeCtrl();
  if (shiftActive) store.consumeShift();
}
