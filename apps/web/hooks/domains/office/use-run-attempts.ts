"use client";

import { useCallback, useEffect, useState } from "react";
import { useAppStore } from "@/components/state-provider";
import { getRunAttempts } from "@/lib/api/domains/office-runs-api";
import type { RouteAttempt } from "@/lib/state/slices/office/types";

export type UseRunAttemptsResult = {
  attempts: RouteAttempt[];
  isLoading: boolean;
  error: string | null;
  refresh: () => Promise<void>;
};

const EMPTY_ATTEMPTS: RouteAttempt[] = [];

export function useRunAttempts(runId: string | null): UseRunAttemptsResult {
  const attempts = useAppStore((s) =>
    runId ? (s.office.runAttempts.byRunId[runId] ?? EMPTY_ATTEMPTS) : EMPTY_ATTEMPTS,
  );
  const setRunAttempts = useAppStore((s) => s.setRunAttempts);
  const [isLoading, setIsLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [fetched, setFetched] = useState(false);

  const refresh = useCallback(async () => {
    if (!runId) return;
    setIsLoading(true);
    setError(null);
    try {
      const res = await getRunAttempts(runId);
      setRunAttempts(runId, res.attempts ?? []);
      setFetched(true);
    } catch (e) {
      setError(e instanceof Error ? e.message : "Failed to load route attempts");
    } finally {
      setIsLoading(false);
    }
  }, [runId, setRunAttempts]);

  useEffect(() => {
    if (!runId || fetched) return;
    void refresh();
  }, [runId, fetched, refresh]);

  return { attempts, isLoading, error, refresh };
}
