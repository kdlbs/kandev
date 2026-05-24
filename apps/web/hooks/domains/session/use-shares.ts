import { useCallback, useEffect, useState } from "react";

import { listShares, type Share } from "@/lib/api/domains/share-api";

export interface UseSharesResult {
  shares: Share[];
  isLoading: boolean;
  error: Error | null;
  refresh: () => Promise<void>;
}

/**
 * Loads the list of share rows for a (taskId, sessionId) pair and exposes a
 * manual refresh callback. Local component state only — shares are a
 * low-frequency surface so we deliberately do NOT keep them in the Zustand
 * store. The dialog calls refresh() after create/revoke to re-sync.
 */
export function useShares(taskId: string | null, sessionId: string | null): UseSharesResult {
  const [shares, setShares] = useState<Share[]>([]);
  const [isLoading, setIsLoading] = useState(false);
  const [error, setError] = useState<Error | null>(null);

  const refresh = useCallback(async () => {
    if (!taskId || !sessionId) {
      // Reset everything when no session is selected so a stale error from
      // a previous session doesn't keep showing in the UI.
      setShares([]);
      setError(null);
      setIsLoading(false);
      return;
    }
    setIsLoading(true);
    setError(null);
    try {
      const resp = await listShares(taskId, sessionId);
      setShares(resp.shares ?? []);
    } catch (err) {
      setError(err instanceof Error ? err : new Error(String(err)));
    } finally {
      setIsLoading(false);
    }
  }, [taskId, sessionId]);

  useEffect(() => {
    void refresh();
  }, [refresh]);

  return { shares, isLoading, error, refresh };
}
