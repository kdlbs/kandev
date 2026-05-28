import type { WebSocketClient } from "@/lib/ws/client";

let activeClient: WebSocketClient | null = null;
const listeners = new Set<(client: WebSocketClient | null) => void>();

export function setWebSocketClient(client: WebSocketClient | null) {
  activeClient = client;
  for (const listener of listeners) listener(client);
}

export function getWebSocketClient() {
  return activeClient;
}

/**
 * Subscribes to WS client (re)assignment. Used by consumers that mount
 * before `<WebSocketConnector />` and need to act once the client is
 * available (notably `<QueryBridge />`). The listener is invoked
 * synchronously with the current client (if any) and on every subsequent
 * setWebSocketClient call. Returns an unsubscribe function.
 */
export function subscribeWebSocketClient(
  listener: (client: WebSocketClient | null) => void,
): () => void {
  listeners.add(listener);
  listener(activeClient);
  return () => {
    listeners.delete(listener);
  };
}
