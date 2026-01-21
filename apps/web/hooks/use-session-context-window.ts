import { useEffect, useMemo } from 'react';
import { useAppStore } from '@/components/state-provider';
import { getWebSocketClient } from '@/lib/ws/connection';
import type { ContextWindowEntry } from '@/lib/state/store';

export function useSessionContextWindow(sessionId: string | null): ContextWindowEntry | undefined {
  // Subscribe to individual primitive values to ensure reactivity
  const size = useAppStore((state) =>
    sessionId ? state.contextWindow.bySessionId[sessionId]?.size : undefined
  );
  const used = useAppStore((state) =>
    sessionId ? state.contextWindow.bySessionId[sessionId]?.used : undefined
  );
  const remaining = useAppStore((state) =>
    sessionId ? state.contextWindow.bySessionId[sessionId]?.remaining : undefined
  );
  const efficiency = useAppStore((state) =>
    sessionId ? state.contextWindow.bySessionId[sessionId]?.efficiency : undefined
  );
  const timestamp = useAppStore((state) =>
    sessionId ? state.contextWindow.bySessionId[sessionId]?.timestamp : undefined
  );

  // Memoize the combined object
  const contextWindow = useMemo(() => {
    if (size === undefined) return undefined;
    return { size, used: used ?? 0, remaining: remaining ?? 0, efficiency: efficiency ?? 0, timestamp };
  }, [size, used, remaining, efficiency, timestamp]);

  const session = useAppStore((state) =>
    sessionId ? state.taskSessions.items[sessionId] : undefined
  );
  const setContextWindow = useAppStore((state) => state.setContextWindow);

  // Populate context window from session metadata if not already in store
  useEffect(() => {
    if (!sessionId || contextWindow) return;

    // Try to extract context_window from session metadata
    const metadata = session?.metadata;
    if (!metadata || typeof metadata !== 'object') return;

    const storedContextWindow = (metadata as Record<string, unknown>).context_window;
    if (!storedContextWindow || typeof storedContextWindow !== 'object') return;

    // Map stored context window to ContextWindowEntry
    const cw = storedContextWindow as Record<string, unknown>;
    const entry: ContextWindowEntry = {
      size: (cw.size as number) ?? 0,
      used: (cw.used as number) ?? 0,
      remaining: (cw.remaining as number) ?? 0,
      efficiency: (cw.efficiency as number) ?? 0,
      timestamp: (cw.timestamp as string) ?? undefined,
    };

    setContextWindow(sessionId, entry);
  }, [sessionId, contextWindow, session?.metadata, setContextWindow]);

  // Subscribe to session updates via WebSocket
  useEffect(() => {
    if (!sessionId) return;
    const client = getWebSocketClient();
    if (client) {
      const unsubscribe = client.subscribeSession(sessionId);
      return () => {
        unsubscribe();
        // Don't clear context window on cleanup - keep it cached
      };
    }
  }, [sessionId]);

  return contextWindow;
}

