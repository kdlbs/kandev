import { useEffect, useMemo } from 'react';
import { useAppStore } from '@/components/state-provider';
import { getWebSocketClient } from '@/lib/ws/connection';
import type { TaskSession } from '@/lib/types/http';

type UseSessionResult = {
  session: TaskSession | null;
  isActive: boolean;
};

export function useSession(sessionId: string | null): UseSessionResult {
  const session = useAppStore((state) =>
    sessionId ? state.taskSessions.items[sessionId] ?? null : null
  );
  const connectionStatus = useAppStore((state) => state.connection.status);

  const isActive = useMemo(() => {
    if (!session?.state) return false;
    return session.state === 'RUNNING' || session.state === 'WAITING_FOR_INPUT';
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

  return { session, isActive };
}
