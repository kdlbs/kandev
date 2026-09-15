import { useSyncExternalStore } from "react";
import type { WebSocketClient } from "@/lib/ws/client";

let activeClient: WebSocketClient | null = null;
const listeners = new Set<() => void>();

export function setWebSocketClient(client: WebSocketClient | null) {
  activeClient = client;
  for (const listener of listeners) listener();
}

export function getWebSocketClient() {
  return activeClient;
}

export function subscribeWebSocketClient(listener: () => void) {
  listeners.add(listener);
  return () => listeners.delete(listener);
}

export function useWebSocketClient() {
  return useSyncExternalStore(subscribeWebSocketClient, getWebSocketClient, () => null);
}
