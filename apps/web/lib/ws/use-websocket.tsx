"use client";

import { useEffect, useRef } from "react";
import type { StoreApi } from "zustand";
import { WebSocketClient } from "@/lib/ws/client";
import { registerWsHandlers } from "@/lib/ws/router";
import type { AppState } from "@/lib/state/store";
import { setWebSocketClient } from "@/lib/ws/connection";

export function useWebSocket(store: StoreApi<AppState>, url: string) {
  const clientRef = useRef<WebSocketClient | null>(null);

  useEffect(() => {
    const client = new WebSocketClient(
      url,
      (status) => {
        const setConnectionStatus = store.getState().setConnectionStatus;
        // WS client and ConnectionState share one ConnectionStatus vocabulary,
        // so this is a 1:1 forward with an `error` message attached and a
        // `subscribeUser()` side-effect on first connect.
        if (status === "connected") {
          client.subscribeUser();
        }
        setConnectionStatus(status, status === "error" ? "WebSocket connection failed" : null);
      },
      {
        enabled: true,
        maxAttempts: 10,
        initialDelay: 1000,
        maxDelay: 30000,
        backoffMultiplier: 1.5,
      },
    );
    clientRef.current = client;
    client.connect();
    setWebSocketClient(client);

    const handlers = registerWsHandlers(store);
    const unsubscribers = Object.entries(handlers).map(([type, handler]) =>
      client.on(type as keyof typeof handlers, handler as never),
    );

    return () => {
      unsubscribers.forEach((unsubscribe) => unsubscribe());
      client.disconnect();
      setWebSocketClient(null);
    };
  }, [store, url]);

  return clientRef;
}
