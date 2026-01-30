import { useEffect, useCallback, useState, useRef } from 'react';
import { getWebSocketClient } from '@/lib/ws/connection';
import type { CumulativeDiff } from '@/lib/state/slices/session-runtime/types';

// Local state cache for cumulative diffs (not stored in global store since it's on-demand)
const cumulativeDiffCache: Record<string, CumulativeDiff | null> = {};
const loadingState: Record<string, boolean> = {};
// Version counter to trigger refetch when cache is invalidated
const cacheVersion: Record<string, number> = {};

/**
 * Invalidate the cumulative diff cache for a session.
 * Call this when git status changes (commits, snapshots, etc.)
 */
export function invalidateCumulativeDiffCache(sessionId: string) {
  delete cumulativeDiffCache[sessionId];
  cacheVersion[sessionId] = (cacheVersion[sessionId] ?? 0) + 1;
}

/**
 * Hook to fetch cumulative diff for a session.
 * Returns the total diff from base branch to current HEAD.
 */
export function useCumulativeDiff(sessionId: string | null) {
  const [diff, setDiff] = useState<CumulativeDiff | null>(
    sessionId ? cumulativeDiffCache[sessionId] ?? null : null
  );
  const [loading, setLoading] = useState(
    sessionId ? loadingState[sessionId] ?? false : false
  );
  const [error, setError] = useState<string | null>(null);
  // Track the version we last fetched for
  const lastFetchedVersionRef = useRef<number | null>(null);

  const fetchCumulativeDiff = useCallback(async () => {
    if (!sessionId) return;

    const client = getWebSocketClient();
    if (!client) return;

    setLoading(true);
    loadingState[sessionId] = true;
    setError(null);

    try {
      const response = await client.request<{ cumulative_diff?: CumulativeDiff }>(
        'session.cumulative_diff',
        { session_id: sessionId }
      );

      if (response?.cumulative_diff) {
        cumulativeDiffCache[sessionId] = response.cumulative_diff;
        setDiff(response.cumulative_diff);
        lastFetchedVersionRef.current = cacheVersion[sessionId] ?? 0;
      }
    } catch (err) {
      console.error('Failed to fetch cumulative diff:', err);
      setError(err instanceof Error ? err.message : 'Failed to fetch cumulative diff');
    } finally {
      setLoading(false);
      loadingState[sessionId] = false;
    }
  }, [sessionId]);

  // Fetch on mount if not cached, or refetch if cache was invalidated
  useEffect(() => {
    if (!sessionId) return;

    const currentVersion = cacheVersion[sessionId] ?? 0;
    const cachedDiff = cumulativeDiffCache[sessionId];
    const needsFetch = !cachedDiff || lastFetchedVersionRef.current !== currentVersion;

    if (needsFetch && !loading) {
      fetchCumulativeDiff();
    }
  }, [sessionId, loading, fetchCumulativeDiff]);

  // Poll for cache invalidation (check every 500ms if cache was invalidated)
  useEffect(() => {
    if (!sessionId) return;

    const interval = setInterval(() => {
      const currentVersion = cacheVersion[sessionId] ?? 0;
      if (lastFetchedVersionRef.current !== currentVersion && !loadingState[sessionId]) {
        // Cache was invalidated, clear local state to trigger refetch
        setDiff(null);
      }
    }, 500);

    return () => clearInterval(interval);
  }, [sessionId]);

  // Update local state when sessionId changes
  useEffect(() => {
    if (sessionId) {
      setDiff(cumulativeDiffCache[sessionId] ?? null);
      setLoading(loadingState[sessionId] ?? false);
      lastFetchedVersionRef.current = cacheVersion[sessionId] ?? 0;
    } else {
      setDiff(null);
      setLoading(false);
      lastFetchedVersionRef.current = null;
    }
  }, [sessionId]);

  return {
    diff,
    loading,
    error,
    refetch: fetchCumulativeDiff,
  };
}

