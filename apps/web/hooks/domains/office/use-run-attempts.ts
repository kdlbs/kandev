"use client";

import { useCallback, useEffect } from "react";
import { useQuery } from "@tanstack/react-query";
import { useAppStore } from "@/components/state-provider";
import { officeRunAttemptsQueryOptions } from "@/lib/query/query-options/office";
import type { RouteAttempt } from "@/lib/state/slices/office/types";
import { queryErrorMessage } from "./query-error";

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
  const query = useQuery(officeRunAttemptsQueryOptions(runId ?? ""));

  const refresh = useCallback(async () => {
    if (!runId) return;
    await query.refetch();
  }, [query, runId]);

  useEffect(() => {
    if (!runId || !query.data) return;
    setRunAttempts(runId, query.data.attempts ?? []);
  }, [query.data, runId, setRunAttempts]);

  const queryAttempts = query.data?.attempts ?? attempts;
  const error = queryErrorMessage(query.error);

  return { attempts: queryAttempts, isLoading: query.isPending, error, refresh };
}
