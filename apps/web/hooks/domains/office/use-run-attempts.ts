"use client";

import { useQuery } from "@tanstack/react-query";
import { officeQueryOptions } from "@/lib/query/query-options/office";
import type { RouteAttempt } from "@/lib/state/slices/office/types";

export type UseRunAttemptsResult = {
  attempts: RouteAttempt[];
  isLoading: boolean;
  error: string | null;
  refresh: () => Promise<void>;
};

const EMPTY_ATTEMPTS: RouteAttempt[] = [];

export function useRunAttempts(runId: string | null): UseRunAttemptsResult {
  const { data, isLoading, error, refetch } = useQuery({
    ...officeQueryOptions.runAttempts(runId ?? ""),
    enabled: !!runId,
  });

  function toErrorMessage(e: unknown): string | null {
    if (!e) return null;
    return e instanceof Error ? e.message : "Failed to load route attempts";
  }

  return {
    attempts: data ?? EMPTY_ATTEMPTS,
    isLoading,
    error: toErrorMessage(error),
    refresh: async () => {
      await refetch();
    },
  };
}
