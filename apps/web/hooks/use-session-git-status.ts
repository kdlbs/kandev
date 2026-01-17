import { useEffect } from 'react';
import { useAppStore } from '@/components/state-provider';
import { getWebSocketClient } from '@/lib/ws/connection';

export function useSessionGitStatus(sessionId: string | null) {
  const gitStatus = useAppStore((state) =>
    sessionId ? state.gitStatus.bySessionId[sessionId] : undefined
  );
  const clearGitStatus = useAppStore((state) => state.clearGitStatus);

  useEffect(() => {
    if (!sessionId) return;
    const client = getWebSocketClient();
    if (client) {
      const unsubscribe = client.subscribeSession(sessionId);
      return () => {
        unsubscribe();
        clearGitStatus(sessionId);
      };
    }
    return () => {
      clearGitStatus(sessionId);
    };
  }, [clearGitStatus, sessionId]);

  return gitStatus;
}
