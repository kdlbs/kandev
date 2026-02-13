import { useEffect, useMemo } from 'react';
import { useAppStore } from '@/components/state-provider';
import { getWebSocketClient } from '@/lib/ws/connection';
import type { TaskSession } from '@/lib/types/http';

type UseSessionResult = {
  session: TaskSession | null;
  isActive: boolean;
  isFailed: boolean;
  errorMessage: string | undefined;
};

export function useSession(sessionId: string | null): UseSessionResult {
  const session = useAppStore((state) =>
    sessionId ? state.taskSessions.items[sessionId] ?? null : null
  );
  const connectionStatus = useAppStore((state) => state.connection.status);
  const agentctlReady = useAppStore((state) =>
    sessionId ? state.sessionAgentctl.itemsBySessionId[sessionId]?.status === 'ready' : false
  );

  const isActive = useMemo(() => {
    if (!session?.state) return false;
    if (session.state === 'RUNNING' || session.state === 'WAITING_FOR_INPUT') return true;
    // Workspace infrastructure (agentctl) is ready even though the agent CLI hasn't started
    if (session.state === 'CREATED' && agentctlReady) return true;
    return false;
  }, [session?.state, agentctlReady]);

  const isFailed = useMemo(() => {
    return session?.state === 'FAILED';
  }, [session?.state]);

  useEffect(() => {
    if (connectionStatus !== 'connected') return;
    if (!session?.id) return;
    const client = getWebSocketClient();
    if (!client) return;
    const unsubscribe = client.subscribeSession(session.id);
    return () => {
      unsubscribe();
    };
  }, [session?.id, connectionStatus]);

  return { session, isActive, isFailed, errorMessage: session?.error_message };
}
