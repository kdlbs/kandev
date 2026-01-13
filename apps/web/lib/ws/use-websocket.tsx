'use client';

import { useEffect, useRef } from 'react';
import type { StoreApi } from 'zustand';
import { WebSocketClient } from '@/lib/ws/client';
import { registerWsHandlers } from '@/lib/ws/router';
import type { AppState } from '@/lib/state/store';
import { setWebSocketClient } from '@/lib/ws/connection';

export function useWebSocket(store: StoreApi<AppState>, url: string) {
  const clientRef = useRef<WebSocketClient | null>(null);

  useEffect(() => {
    const client = new WebSocketClient(
      url,
      (status) => {
        const setConnectionStatus = store.getState().setConnectionStatus;
        switch (status) {
          case 'connecting':
            setConnectionStatus('connecting', null);
            break;
          case 'open':
            setConnectionStatus('connected', null);
            client.subscribeUser();
            break;
          case 'reconnecting':
            setConnectionStatus('reconnecting', null);
            break;
          case 'error':
            setConnectionStatus('error', 'WebSocket connection failed');
            break;
          case 'closed':
          case 'idle':
          default:
            setConnectionStatus('disconnected', null);
            break;
        }
      },
      {
        enabled: true,
        maxAttempts: 10,
        initialDelay: 1000,
        maxDelay: 30000,
        backoffMultiplier: 1.5,
      }
    );
    clientRef.current = client;
    client.connect();
    setWebSocketClient(client);

    const handlers = registerWsHandlers(store);
    const unsubscribers = Object.entries(handlers).map(([type, handler]) =>
      client.on(type as keyof typeof handlers, handler as never)
    );

    return () => {
      unsubscribers.forEach((unsubscribe) => unsubscribe());
      client.disconnect();
      setWebSocketClient(null);
    };
  }, [store, url]);

  return clientRef;
}
