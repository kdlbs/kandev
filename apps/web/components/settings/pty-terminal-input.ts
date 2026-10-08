import { applyShellModifiers } from "@/lib/terminal/apply-shell-modifiers";
import { isActive, useShellModifiersStore } from "@/lib/terminal/shell-modifiers";

/** Host PTYs own their socket; they never use the task shell-input fallback. */
export function sendPtyInput(
  socket: Pick<WebSocket, "readyState" | "send"> | null,
  data: string,
  applyModifiers: boolean,
): void {
  if (!data || !socket || socket.readyState !== WebSocket.OPEN) return;
  const store = useShellModifiersStore.getState();
  const ctrl = applyModifiers && isActive(store.ctrl);
  const shift = applyModifiers && isActive(store.shift);
  socket.send(new TextEncoder().encode(applyShellModifiers(data, { ctrl, shift })));
  if (ctrl) store.consumeCtrl();
  if (shift) store.consumeShift();
}
