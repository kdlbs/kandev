import { useEffect } from 'react';
import { fetchBoardSnapshot } from '@/lib/api';
import { snapshotToState } from '@/lib/ssr/mapper';
import { useAppStore, useAppStoreApi } from '@/components/state-provider';

export function useBoardSnapshot(boardId: string | null) {
  const store = useAppStoreApi();
  const connectionStatus = useAppStore((state) => state.connection.status);

  useEffect(() => {
    if (!boardId) return;
    fetchBoardSnapshot(boardId, { cache: 'no-store' })
      .then((snapshot) => {
        store.getState().hydrate(snapshotToState(snapshot));
      })
      .catch(() => {
        // Ignore snapshot errors — will retry on WS reconnect.
      });
  }, [boardId, store, connectionStatus]);
}
