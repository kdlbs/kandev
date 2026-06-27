"use client";

import { useCallback } from "react";
import { useQuery } from "@tanstack/react-query";
import { automationRunsQueryOptions } from "@/lib/query/query-options/automations";
import type { AutomationRun } from "@/lib/types/automation";

const EMPTY_RUNS: AutomationRun[] = [];

export function useAutomationRuns(automationId: string | null) {
  const query = useQuery({
    ...automationRunsQueryOptions(automationId ?? ""),
    enabled: Boolean(automationId),
  });
  const runs = query.data ?? EMPTY_RUNS;
  const refetch = query.refetch;

  const refresh = useCallback(() => {
    if (!automationId) return;
    void refetch();
  }, [automationId, refetch]);

  return { runs, loading: query.isFetching && !query.isSuccess, refresh };
}
