import { useEffect, useCallback, useState } from "react";
import { useAppStore } from "@/components/state-provider";
import { getWebSocketClient } from "@/lib/ws/connection";
import type { CumulativeDiff } from "@/lib/state/slices/session-runtime/types";

const cumulativeDiffCache: Record<string, CumulativeDiff | null> = {};
const loadingState: Record<string, boolean> = {};

const listeners = new Set<(envKey: string) => void>();

/**
 * Invalidate the cumulative diff cache for the given environment key.
 * Callers should resolve sessionId → envKey before calling this.
 */
export function invalidateCumulativeDiffCache(envKey: string) {
  delete cumulativeDiffCache[envKey];
  listeners.forEach((fn) => fn(envKey));
}

export function useCumulativeDiff(sessionId: string | null) {
  // Resolve to environment key so sessions sharing the same environment share the cache.
  const envKey = useAppStore((state) => {
    if (!sessionId) return null;
    return state.environmentIdBySessionId[sessionId] ?? sessionId;
  });

  const [diff, setDiff] = useState<CumulativeDiff | null>(
    envKey ? (cumulativeDiffCache[envKey] ?? null) : null,
  );
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [invalidationCount, setInvalidationCount] = useState(0);

  const fetchCumulativeDiff = useCallback(async () => {
    if (!sessionId || !envKey) return;
    if (loadingState[envKey]) return;

    const client = getWebSocketClient();
    if (!client) return;

    setLoading(true);
    loadingState[envKey] = true;
    setError(null);

    try {
      // Backend routes by session_id, but we cache by envKey
      const response = await client.request<{ cumulative_diff?: CumulativeDiff }>(
        "session.cumulative_diff",
        { session_id: sessionId },
      );

      if (response?.cumulative_diff) {
        cumulativeDiffCache[envKey] = response.cumulative_diff;
        setDiff(response.cumulative_diff);
      }
    } catch (err) {
      console.error("Failed to fetch cumulative diff:", err);
      setError(err instanceof Error ? err.message : "Failed to fetch cumulative diff");
    } finally {
      setLoading(false);
      loadingState[envKey] = false;
    }
  }, [sessionId, envKey]);

  // Fetch on mount and when cache is invalidated
  useEffect(() => {
    if (!envKey) return;
    fetchCumulativeDiff();
  }, [envKey, invalidationCount, fetchCumulativeDiff]);

  // Subscribe to cache invalidation from WS handlers
  useEffect(() => {
    if (!envKey) return;
    const handler = (invalidatedEnvKey: string) => {
      if (invalidatedEnvKey === envKey) {
        setInvalidationCount((c) => c + 1);
      }
    };
    listeners.add(handler);
    return () => {
      listeners.delete(handler);
    };
  }, [envKey]);

  // Sync cached state when envKey changes
  useEffect(() => {
    if (envKey) {
      setDiff(cumulativeDiffCache[envKey] ?? null);
    } else {
      setDiff(null);
      setLoading(false);
    }
  }, [envKey]);

  return {
    diff,
    loading,
    error,
    refetch: fetchCumulativeDiff,
  };
}
