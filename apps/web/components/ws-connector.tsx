'use client';

import { useMemo } from 'react';

import { useAppStoreApi } from '@/components/state-provider';
import { getBackendConfig } from '@/lib/config';
import { httpToWebSocketUrl, isValidBackendUrl } from '@/lib/ws/utils';
import { useWebSocket } from '@/lib/ws/use-websocket';

export function WebSocketConnector() {
  const store = useAppStoreApi();
  const backendUrl = useMemo(() => {
    const configured = getBackendConfig().apiBaseUrl;
    return isValidBackendUrl(configured) ? configured : 'http://localhost:8080';
  }, []);

  const wsUrl = useMemo(() => httpToWebSocketUrl(backendUrl), [backendUrl]);
  useWebSocket(store, wsUrl);

  return null;
}
