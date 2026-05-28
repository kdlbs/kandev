"use client";

import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { updateAgentRouting } from "@/lib/api/domains/office-routing-api";
import { officeQueryOptions } from "@/lib/query/query-options/office";
import { qk } from "@/lib/query/keys";
import type { AgentRouteData, AgentRoutingOverrides } from "@/lib/state/slices/office/types";

export type UseAgentRouteResult = {
  data: AgentRouteData | undefined;
  isLoading: boolean;
  error: string | null;
  refresh: () => Promise<void>;
  updateOverrides: (ov: AgentRoutingOverrides) => Promise<void>;
};

export function useAgentRoute(agentId: string | null): UseAgentRouteResult {
  const qc = useQueryClient();

  const { data, isLoading, error, refetch } = useQuery({
    ...officeQueryOptions.agentRouting(agentId ?? ""),
    enabled: !!agentId,
  });

  const updateMutation = useMutation({
    mutationFn: (ov: AgentRoutingOverrides) => updateAgentRouting(agentId!, ov),
    onSuccess: () => {
      if (agentId) void qc.invalidateQueries({ queryKey: qk.office.agentRouting(agentId) });
    },
  });

  function toErrorMessage(e: unknown): string | null {
    if (!e) return null;
    return e instanceof Error ? e.message : "Failed to load agent route";
  }

  return {
    data,
    isLoading,
    error: toErrorMessage(error),
    refresh: async () => {
      await refetch();
    },
    updateOverrides: async (ov: AgentRoutingOverrides) => {
      await updateMutation.mutateAsync(ov);
    },
  };
}
