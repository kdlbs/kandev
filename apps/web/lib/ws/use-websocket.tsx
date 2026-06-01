"use client";

import { useEffect, useRef } from "react";
import type { StoreApi } from "zustand";
import { useQueryClient } from "@tanstack/react-query";
import { WebSocketClient } from "@/lib/ws/client";
import { registerWsHandlers } from "@/lib/ws/router";
import type { AppState } from "@/lib/state/store";
import { setWebSocketClient } from "@/lib/ws/connection";
import { createDebugLogger } from "@/lib/debug/log";

const debug = createDebugLogger("ws:connection");

export function useWebSocket(store: StoreApi<AppState>, url: string) {
  const clientRef = useRef<WebSocketClient | null>(null);
  const queryClient = useQueryClient();

  useEffect(() => {
    debug("WS hook mounting", { url });
    const client = new WebSocketClient(
      url,
      (status) => {
        const setConnectionStatus = store.getState().setConnectionStatus;
        debug("status transition", { status, timestamp: new Date().toISOString() });
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

    // Register the Zustand-side WS handlers BEFORE publishing the client via
    // setWebSocketClient. Publishing fires the QueryBridge subscriber, which
    // registers the session-state bridge handlers. The agent-session handler
    // must read the TaskSession by-id TQ cache (adoption / failure-toast
    // branch on the *prior* record) before the bridge overwrites it, and WS
    // handlers fire in registration order — so ours must be added first.
    const handlers = registerWsHandlers(store, queryClient);
    const unsubscribers = Object.entries(handlers).map(([type, handler]) =>
      client.on(type as keyof typeof handlers, handler as never),
    );

    setWebSocketClient(client);

    return () => {
      unsubscribers.forEach((unsubscribe) => unsubscribe());
      client.disconnect();
      setWebSocketClient(null);
    };
  }, [store, url, queryClient]);

  return clientRef;
}
