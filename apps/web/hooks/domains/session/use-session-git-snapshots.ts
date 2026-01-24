import { useEffect, useCallback } from 'react';
import { useAppStore } from '@/components/state-provider';
import { getWebSocketClient } from '@/lib/ws/connection';
import type { GitSnapshot } from '@/lib/state/slices/session-runtime/types';

/**
 * Hook to fetch and manage git snapshots for a session.
 * Git snapshots provide historical tracking of git status at key moments.
 */
export function useSessionGitSnapshots(sessionId: string | null, options?: { limit?: number }) {
  const snapshots = useAppStore((state) =>
    sessionId ? state.gitSnapshots.bySessionId[sessionId] : undefined
  );
  const latestSnapshot = useAppStore((state) =>
    sessionId ? state.gitSnapshots.latestBySessionId[sessionId] : undefined
  );
  const loading = useAppStore((state) =>
    sessionId ? state.gitSnapshots.loading[sessionId] : false
  );
  const setGitSnapshots = useAppStore((state) => state.setGitSnapshots);
  const setGitSnapshotsLoading = useAppStore((state) => state.setGitSnapshotsLoading);

  const fetchSnapshots = useCallback(async () => {
    if (!sessionId) return;

    const client = getWebSocketClient();
    if (!client) return;

    setGitSnapshotsLoading(sessionId, true);
    try {
      const response = await client.request<{ snapshots?: GitSnapshot[] }>(
        'session.git.snapshots',
        {
          session_id: sessionId,
          limit: options?.limit ?? 0,
        }
      );

      if (response?.snapshots) {
        setGitSnapshots(sessionId, response.snapshots);
      }
    } catch (error) {
      console.error('Failed to fetch git snapshots:', error);
    } finally {
      setGitSnapshotsLoading(sessionId, false);
    }
  }, [sessionId, options?.limit, setGitSnapshots, setGitSnapshotsLoading]);

  // Fetch snapshots on mount
  useEffect(() => {
    if (sessionId && !snapshots) {
      fetchSnapshots();
    }
  }, [sessionId, snapshots, fetchSnapshots]);

  return {
    snapshots: snapshots ?? [],
    latestSnapshot: latestSnapshot ?? null,
    loading,
    refetch: fetchSnapshots,
  };
}

