import type { WebSocketClient } from "@/lib/ws/client";

export type WebSocketClientListener = (client: WebSocketClient | null) => void;

let activeClient: WebSocketClient | null = null;
const listeners = new Set<WebSocketClientListener>();

export function setWebSocketClient(client: WebSocketClient | null) {
  if (activeClient === client) return;
  activeClient = client;
  for (const listener of Array.from(listeners)) {
    listener(activeClient);
  }
}

export function getWebSocketClient() {
  return activeClient;
}

export function subscribeWebSocketClient(listener: WebSocketClientListener) {
  listeners.add(listener);
  listener(activeClient);
  return () => {
    listeners.delete(listener);
  };
}
