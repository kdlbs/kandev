'use client';

import { useEffect, useMemo, useState } from 'react';

import { useAppStoreApi } from '@/components/state-provider';
import { getLocalStorage } from '@/lib/local-storage';
import { DEFAULT_BACKEND_URL, STORAGE_KEYS } from '@/lib/settings/constants';
import { httpToWebSocketUrl, isValidBackendUrl } from '@/lib/ws/utils';
import { useWebSocket } from '@/lib/ws/use-websocket';

export function WebSocketConnector() {
  const store = useAppStoreApi();
  const [backendUrl, setBackendUrl] = useState<string>(DEFAULT_BACKEND_URL);

  useEffect(() => {
    const saved = getLocalStorage(STORAGE_KEYS.BACKEND_URL, DEFAULT_BACKEND_URL);
    setBackendUrl(isValidBackendUrl(saved) ? saved : DEFAULT_BACKEND_URL);
  }, []);

  const wsUrl = useMemo(() => httpToWebSocketUrl(backendUrl), [backendUrl]);
  useWebSocket(store, wsUrl);

  return null;
}
