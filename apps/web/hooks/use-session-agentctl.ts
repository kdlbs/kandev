import { useEffect } from 'react';
import { useAppStore } from '@/components/state-provider';
import { getWebSocketClient } from '@/lib/ws/connection';

export function useSessionAgentctl(sessionId: string | null) {
  const session = useAppStore((state) =>
    sessionId ? state.taskSessions.items[sessionId] : undefined
  );
  const status = useAppStore((state) =>
    sessionId ? state.sessionAgentctl.itemsBySessionId[sessionId] : undefined
  );

  useEffect(() => {
    if (!session?.id) return;
    const client = getWebSocketClient();
    if (!client) return;
    return client.subscribeSession(session.id);
  }, [session?.id]);

  return {
    status: status?.status ?? 'starting',
    errorMessage: status?.errorMessage,
    agentExecutionId: status?.agentExecutionId,
    isReady: status?.status === 'ready',
    isStarting: status?.status === 'starting' || !status,
    isError: status?.status === 'error',
  };
}
