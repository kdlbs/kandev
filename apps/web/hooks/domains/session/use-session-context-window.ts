import { useEffect, useMemo } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useAppStore } from "@/components/state-provider";
import { qk } from "@/lib/query/keys";
import { contextWindowQueryOptions } from "@/lib/query/query-options";
import { getWebSocketClient } from "@/lib/ws/connection";
import type { ContextWindowEntry } from "@/lib/state/store";

export function useSessionContextWindow(sessionId: string | null): ContextWindowEntry | undefined {
  const queryClient = useQueryClient();
  const contextWindowQuery = useQuery(contextWindowQueryOptions(sessionId ?? ""));
  // Subscribe to individual primitive values to ensure reactivity
  const storeSize = useAppStore((state) =>
    sessionId ? state.contextWindow.bySessionId[sessionId]?.size : undefined,
  );
  const storeUsed = useAppStore((state) =>
    sessionId ? state.contextWindow.bySessionId[sessionId]?.used : undefined,
  );
  const storeRemaining = useAppStore((state) =>
    sessionId ? state.contextWindow.bySessionId[sessionId]?.remaining : undefined,
  );
  const storeEfficiency = useAppStore((state) =>
    sessionId ? state.contextWindow.bySessionId[sessionId]?.efficiency : undefined,
  );
  const storeSource = useAppStore((state) =>
    sessionId ? state.contextWindow.bySessionId[sessionId]?.source : undefined,
  );
  const storeTimestamp = useAppStore((state) =>
    sessionId ? state.contextWindow.bySessionId[sessionId]?.timestamp : undefined,
  );

  // Memoize the combined object
  const storeContextWindow = useMemo(() => {
    if (storeSize === undefined) return undefined;
    return {
      size: storeSize,
      used: storeUsed ?? 0,
      remaining: storeRemaining ?? 0,
      efficiency: storeEfficiency ?? 0,
      source: storeSource,
      timestamp: storeTimestamp,
    };
  }, [storeSize, storeUsed, storeRemaining, storeEfficiency, storeSource, storeTimestamp]);
  const contextWindow = contextWindowQuery.data ?? storeContextWindow;

  const session = useAppStore((state) =>
    sessionId ? state.taskSessions.items[sessionId] : undefined,
  );
  const setContextWindow = useAppStore((state) => state.setContextWindow);
  const connectionStatus = useAppStore((state) => state.connection.status);

  // Populate context window from session metadata if not already in store
  useEffect(() => {
    if (!sessionId || contextWindow) return;

    // Try to extract context_window from session metadata
    const metadata = session?.metadata;
    if (!metadata || typeof metadata !== "object") return;

    const storedContextWindow = (metadata as Record<string, unknown>).context_window;
    if (!storedContextWindow || typeof storedContextWindow !== "object") return;

    // Map stored context window to ContextWindowEntry
    const cw = storedContextWindow as Record<string, unknown>;
    const entry: ContextWindowEntry = {
      size: (cw.size as number) ?? 0,
      used: (cw.used as number) ?? 0,
      remaining: (cw.remaining as number) ?? 0,
      efficiency: (cw.efficiency as number) ?? 0,
      source: (cw.source as ContextWindowEntry["source"]) ?? undefined,
      timestamp: (cw.timestamp as string) ?? undefined,
    };

    setContextWindow(sessionId, entry);
    queryClient.setQueryData(qk.sessionRuntime.contextWindow(sessionId), entry);
  }, [sessionId, contextWindow, session?.metadata, setContextWindow, queryClient]);

  // Subscribe to session updates via WebSocket
  useEffect(() => {
    if (!sessionId) return;
    if (connectionStatus !== "connected") return;
    const client = getWebSocketClient();
    if (client) {
      const unsubscribe = client.subscribeSession(sessionId);
      return () => {
        unsubscribe();
        // Don't clear context window on cleanup - keep it cached
      };
    }
  }, [sessionId, connectionStatus]);

  return contextWindow;
}
