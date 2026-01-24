import { useEffect } from 'react';
import { useAppStore } from '@/components/state-provider';
import { getWebSocketClient } from '@/lib/ws/connection';

/**
 * Hook to get the current git status for a session.
 * Git status is populated via WebSocket from git snapshot updates.
 * For historical snapshots, use useSessionGitSnapshots hook.
 */
export function useSessionGitStatus(sessionId: string | null) {
  const gitStatus = useAppStore((state) =>
    sessionId ? state.gitStatus.bySessionId[sessionId] : undefined
  );

  // Subscribe to session updates to receive git status via WebSocket
  useEffect(() => {
    if (!sessionId) return;
    const client = getWebSocketClient();
    if (client) {
      const unsubscribe = client.subscribeSession(sessionId);
      return () => {
        unsubscribe();
        // Don't clear git status on cleanup - keep it cached for when user switches back
      };
    }
  }, [sessionId]);

  return gitStatus;
}
