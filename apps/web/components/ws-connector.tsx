'use client';

import { useMemo } from 'react';

import { useAppStoreApi } from '@/components/state-provider';
import { getBackendConfig } from '@/lib/config';
import { httpToWebSocketUrl } from '@/lib/ws/utils';
import { useWebSocket } from '@/lib/ws/use-websocket';

export function WebSocketConnector() {
  const store = useAppStoreApi();

  // Get WebSocket URL from current hostname (supports remote access)
  const wsUrl = useMemo(() => {
    const { apiBaseUrl } = getBackendConfig();
    return httpToWebSocketUrl(apiBaseUrl);
  }, []);

  useWebSocket(store, wsUrl);

  return null;
}
