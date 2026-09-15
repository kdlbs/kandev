"use client";

import { useEffect, useRef, useSyncExternalStore } from "react";
import type { StoreApi } from "zustand";
import { WebSocketClient } from "@/lib/ws/client";
import { registerWsHandlers } from "@/lib/ws/router";
import type { AppState } from "@/lib/state/store";
import { setWebSocketClient } from "@/lib/ws/connection";
import { createDebugLogger } from "@/lib/debug/log";
import { ConnectionIssueMonitor } from "@/lib/ws/connection-issue-monitor";

const debug = createDebugLogger("ws:connection");

export function useWebSocket(store: StoreApi<AppState>, url: string) {
  const userID = useSyncExternalStore(
    store.subscribe,
    () => store.getState().auth.user?.id ?? null,
    () => null,
  );
  const clientRef = useRef<WebSocketClient | null>(null);

  useEffect(() => {
    let active = true;
    const connectionIssueMonitor = new ConnectionIssueMonitor((severity) => {
      store.getState().setConnectionIssueSeverity(severity);
    });
    const client = new WebSocketClient(
      url,
      (status) => {
        if (!active) return;
        connectionIssueMonitor.onStatusChange(status);
        const setConnectionStatus = store.getState().setConnectionStatus;
        debug("status transition", { status, timestamp: new Date().toISOString() });
        // WS client and ConnectionState share one ConnectionStatus vocabulary,
        // so this is a 1:1 forward with an `error` message attached.
        // i18n-exempt: transport diagnostic interpolated into the translated
        // `sidebar:connectionErrorDetail` frame; see connection-status-item.tsx.
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
    client.subscribeUser();
    clientRef.current = client;
    client.connect();
    setWebSocketClient(client);

    const registration = registerWsHandlers(store);
    const unsubscribers = Object.entries(registration.handlers).map(([type, handler]) =>
      client.on(
        type as keyof typeof registration.handlers,
        ((payload: never) => {
          if (active) (handler as (value: never) => void)(payload);
        }) as never,
      ),
    );

    return () => {
      active = false;
      unsubscribers.forEach((unsubscribe) => unsubscribe());
      registration.dispose();
      connectionIssueMonitor.dispose();
      client.disconnect();
      setWebSocketClient(null);
    };
  }, [store, url, userID]);

  return clientRef;
}
