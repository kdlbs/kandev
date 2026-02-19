import type { WebSocketClient } from "@/lib/ws/client";

let activeClient: WebSocketClient | null = null;

export function setWebSocketClient(client: WebSocketClient | null) {
  activeClient = client;
}

export function getWebSocketClient() {
  return activeClient;
}
