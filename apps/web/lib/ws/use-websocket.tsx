'use client';

import { useEffect, useRef } from 'react';
import type { StoreApi } from 'zustand';
import { WebSocketClient } from '@/lib/ws/client';
import { registerWsHandlers } from '@/lib/ws/router';
import type { AppState } from '@/lib/state/store';

export function useWebSocket(store: StoreApi<AppState>, url: string) {
  const clientRef = useRef<WebSocketClient | null>(null);

  useEffect(() => {
    const client = new WebSocketClient(url, (status) => {
      const setConnectionStatus = store.getState().setConnectionStatus;
      switch (status) {
        case 'connecting':
          setConnectionStatus('connecting', null);
          break;
        case 'open':
          setConnectionStatus('connected', null);
          break;
        case 'error':
          setConnectionStatus('error', 'WebSocket error');
          break;
        case 'closed':
        case 'idle':
        default:
          setConnectionStatus('disconnected', null);
          break;
      }
    });
    clientRef.current = client;
    client.connect();

    const handlers = registerWsHandlers(store);
    const unsubscribers = Object.entries(handlers).map(([type, handler]) =>
      client.on(type as keyof typeof handlers, handler as never)
    );

    return () => {
      unsubscribers.forEach((unsubscribe) => unsubscribe());
      client.disconnect();
    };
  }, [store, url]);

  return clientRef;
}
