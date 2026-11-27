import { useEffect, useMemo } from "react";
import { useAppStore } from "@/components/state-provider";
import { getWebSocketClient } from "@/lib/ws/connection";
import type { ContextWindowEntry } from "@/lib/state/store";
import {
  mergeCompactionCountValue,
  mergeCompactionReportingValue,
  reconcileContextWindowMetadata,
  sameContextWindowEntry,
} from "@/lib/state/slices/session-runtime/context-window";

export function useSessionContextWindow(
  sessionId: string | null,
  detailActive = true,
): ContextWindowEntry | undefined {
  // Subscribe to individual primitive values to ensure reactivity
  const size = useAppStore((state) =>
    sessionId ? state.contextWindow.bySessionId[sessionId]?.size : undefined,
  );
  const used = useAppStore((state) =>
    sessionId ? state.contextWindow.bySessionId[sessionId]?.used : undefined,
  );
  const remaining = useAppStore((state) =>
    sessionId ? state.contextWindow.bySessionId[sessionId]?.remaining : undefined,
  );
  const efficiency = useAppStore((state) =>
    sessionId ? state.contextWindow.bySessionId[sessionId]?.efficiency : undefined,
  );
  const compactionCount = useAppStore((state) =>
    sessionId ? state.contextWindow.bySessionId[sessionId]?.compactionCount : undefined,
  );
  const compactionReporting = useAppStore((state) =>
    sessionId ? state.contextWindow.bySessionId[sessionId]?.compactionReporting : undefined,
  );
  const source = useAppStore((state) =>
    sessionId ? state.contextWindow.bySessionId[sessionId]?.source : undefined,
  );
  const timestamp = useAppStore((state) =>
    sessionId ? state.contextWindow.bySessionId[sessionId]?.timestamp : undefined,
  );

  // Memoize the combined object
  const contextWindow = useMemo(() => {
    if (size === undefined) return undefined;
    return {
      size,
      used: used ?? 0,
      remaining: remaining ?? 0,
      efficiency: efficiency ?? 0,
      compactionCount: compactionCount ?? 0,
      compactionReporting,
      source,
      timestamp,
    };
  }, [size, used, remaining, efficiency, compactionCount, compactionReporting, source, timestamp]);

  const session = useAppStore((state) =>
    sessionId ? state.taskSessions.items[sessionId] : undefined,
  );
  const setContextWindow = useAppStore((state) => state.setContextWindow);
  const connectionStatus = useAppStore((state) => state.connection.status);

  // Reconcile the retained session metadata with the live cache. This is the
  // shared freshness boundary for hydration, deferred HTTP snapshots, and a
  // missed count-only notification.
  useEffect(() => {
    if (!sessionId) return;
    const metadata = session?.metadata;
    if (!metadata || typeof metadata !== "object") return;
    const record = metadata as Record<string, unknown>;
    const hasGauge = Object.prototype.hasOwnProperty.call(record, "context_window");
    const count = mergeCompactionCountValue(
      record.context_compaction_count,
      contextWindow?.compactionCount,
    );
    const reporting = mergeCompactionReportingValue(
      record.context_compaction_reporting,
      contextWindow?.compactionReporting,
    );

    // Hydration never clears a live gauge from a stale null snapshot; live WS
    // events own explicit invalidation.
    const next = reconcileContextWindowMetadata({
      current: contextWindow,
      gauge: record.context_window,
      hasGauge,
      count,
      reporting,
      timestamp: undefined,
      allowExplicitClear: false,
    });
    if (next === undefined || next === null || sameContextWindowEntry(next, contextWindow)) return;
    setContextWindow(sessionId, next);
  }, [sessionId, contextWindow, session?.metadata, setContextWindow]);

  // Subscribe to session updates via WebSocket
  useEffect(() => {
    if (!detailActive) return;
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
  }, [detailActive, sessionId, connectionStatus]);

  return contextWindow;
}
