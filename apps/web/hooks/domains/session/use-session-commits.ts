import { useEffect, useCallback } from 'react';
import { useAppStore } from '@/components/state-provider';
import { getWebSocketClient } from '@/lib/ws/connection';
import type { SessionCommit } from '@/lib/state/slices/session-runtime/types';

/**
 * Hook to fetch and manage commits for a session.
 * Session commits track all commits made during a task session.
 */
export function useSessionCommits(sessionId: string | null) {
  const commits = useAppStore((state) =>
    sessionId ? state.sessionCommits.bySessionId[sessionId] : undefined
  );
  const loading = useAppStore((state) =>
    sessionId ? state.sessionCommits.loading[sessionId] : false
  );
  const setSessionCommits = useAppStore((state) => state.setSessionCommits);
  const setSessionCommitsLoading = useAppStore((state) => state.setSessionCommitsLoading);
  const connectionStatus = useAppStore((state) => state.connection.status);

  const fetchCommits = useCallback(async () => {
    if (!sessionId) return;

    const client = getWebSocketClient();
    if (!client) return;

    setSessionCommitsLoading(sessionId, true);
    try {
      const response = await client.request<{ commits?: SessionCommit[] }>(
        'session.git.commits',
        { session_id: sessionId }
      );

      if (response?.commits) {
        setSessionCommits(sessionId, response.commits);
      }
    } catch (error) {
      console.error('Failed to fetch session commits:', error);
    } finally {
      setSessionCommitsLoading(sessionId, false);
    }
  }, [sessionId, setSessionCommits, setSessionCommitsLoading]);

  // Fetch commits on mount
  useEffect(() => {
    if (connectionStatus !== 'connected') return;
    if (sessionId && !commits) {
      fetchCommits();
    }
  }, [sessionId, commits, fetchCommits, connectionStatus]);

  return {
    commits: commits ?? [],
    loading,
    refetch: fetchCommits,
  };
}

