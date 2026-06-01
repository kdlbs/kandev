"use client";

import { useQuery, useQueryClient } from "@tanstack/react-query";
import { automationsQueryOptions } from "@/lib/query/query-options/automations";
import { qk } from "@/lib/query/keys";

export function useAutomationRuns(automationId: string | null) {
  const qc = useQueryClient();
  const safeId = automationId ?? "";

  const { data, isLoading } = useQuery({
    ...automationsQueryOptions.runs(safeId),
    enabled: !!automationId,
  });

  const runs = data ?? [];

  const refresh = () => {
    if (automationId) {
      void qc.invalidateQueries({ queryKey: qk.automations.runs(safeId) });
    }
  };

  return { runs, loading: isLoading, refresh };
}
