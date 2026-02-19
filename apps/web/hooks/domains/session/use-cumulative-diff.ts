import { useEffect, useCallback, useState } from "react";
import { getWebSocketClient } from "@/lib/ws/connection";
import type { CumulativeDiff } from "@/lib/state/slices/session-runtime/types";

const cumulativeDiffCache: Record<string, CumulativeDiff | null> = {};
const loadingState: Record<string, boolean> = {};

const listeners = new Set<(sessionId: string) => void>();

export function invalidateCumulativeDiffCache(sessionId: string) {
  delete cumulativeDiffCache[sessionId];
  listeners.forEach((fn) => fn(sessionId));
}

export function useCumulativeDiff(sessionId: string | null) {
  const [diff, setDiff] = useState<CumulativeDiff | null>(
    sessionId ? (cumulativeDiffCache[sessionId] ?? null) : null,
  );
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [invalidationCount, setInvalidationCount] = useState(0);

  const fetchCumulativeDiff = useCallback(async () => {
    if (!sessionId) return;
    if (loadingState[sessionId]) return;

    const client = getWebSocketClient();
    if (!client) return;

    setLoading(true);
    loadingState[sessionId] = true;
    setError(null);

    try {
      const response = await client.request<{ cumulative_diff?: CumulativeDiff }>(
        "session.cumulative_diff",
        { session_id: sessionId },
      );

      if (response?.cumulative_diff) {
        cumulativeDiffCache[sessionId] = response.cumulative_diff;
        setDiff(response.cumulative_diff);
      }
    } catch (err) {
      console.error("Failed to fetch cumulative diff:", err);
      setError(err instanceof Error ? err.message : "Failed to fetch cumulative diff");
    } finally {
      setLoading(false);
      loadingState[sessionId] = false;
    }
  }, [sessionId]);

  // Fetch on mount and when cache is invalidated
  useEffect(() => {
    if (!sessionId) return;
    fetchCumulativeDiff();
  }, [sessionId, invalidationCount, fetchCumulativeDiff]);

  // Subscribe to cache invalidation from WS handlers
  useEffect(() => {
    if (!sessionId) return;
    const handler = (invalidatedSessionId: string) => {
      if (invalidatedSessionId === sessionId) {
        setInvalidationCount((c) => c + 1);
      }
    };
    listeners.add(handler);
    return () => {
      listeners.delete(handler);
    };
  }, [sessionId]);

  // Sync cached state when sessionId changes
  useEffect(() => {
    if (sessionId) {
      setDiff(cumulativeDiffCache[sessionId] ?? null);
    } else {
      setDiff(null);
      setLoading(false);
    }
  }, [sessionId]);

  return {
    diff,
    loading,
    error,
    refetch: fetchCumulativeDiff,
  };
}
