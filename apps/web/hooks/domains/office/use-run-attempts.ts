"use client";

import { useCallback } from "react";
import { useQuery } from "@tanstack/react-query";
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
  const query = useQuery(officeRunAttemptsQueryOptions(runId ?? ""));

  const refresh = useCallback(async () => {
    if (!runId) return;
    await query.refetch();
  }, [query, runId]);

  const queryAttempts = query.data?.attempts ?? EMPTY_ATTEMPTS;
  const error = queryErrorMessage(query.error);

  return { attempts: queryAttempts, isLoading: query.isPending, error, refresh };
}
