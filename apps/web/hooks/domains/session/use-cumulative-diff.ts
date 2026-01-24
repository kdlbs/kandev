import { useEffect, useCallback, useState } from 'react';
import { getWebSocketClient } from '@/lib/ws/connection';
import type { CumulativeDiff } from '@/lib/state/slices/session-runtime/types';

// Local state cache for cumulative diffs (not stored in global store since it's on-demand)
const cumulativeDiffCache: Record<string, CumulativeDiff | null> = {};
const loadingState: Record<string, boolean> = {};

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
      }
    } catch (err) {
      console.error('Failed to fetch cumulative diff:', err);
      setError(err instanceof Error ? err.message : 'Failed to fetch cumulative diff');
    } finally {
      setLoading(false);
      loadingState[sessionId] = false;
    }
  }, [sessionId]);

  // Fetch on mount if not cached
  useEffect(() => {
    if (sessionId && !diff && !loading) {
      fetchCumulativeDiff();
    }
  }, [sessionId, diff, loading, fetchCumulativeDiff]);

  // Update local state when sessionId changes
  useEffect(() => {
    if (sessionId) {
      setDiff(cumulativeDiffCache[sessionId] ?? null);
      setLoading(loadingState[sessionId] ?? false);
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

