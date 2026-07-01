"use client";

import { useCallback } from "react";
import { useQuery } from "@tanstack/react-query";
import { officeAgentRouteQueryOptions } from "@/lib/query/query-options/office";
import { updateAgentRouting } from "@/lib/api/domains/office-extended-api";
import type { AgentRouteData, AgentRoutingOverrides } from "@/lib/state/slices/office/types";
import { queryErrorMessage } from "./query-error";

export type UseAgentRouteResult = {
  data: AgentRouteData | undefined;
  isLoading: boolean;
  error: string | null;
  refresh: () => Promise<void>;
  updateOverrides: (ov: AgentRoutingOverrides) => Promise<void>;
};

export function useAgentRoute(agentId: string | null): UseAgentRouteResult {
  const query = useQuery(officeAgentRouteQueryOptions(agentId ?? ""));

  const refresh = useCallback(async () => {
    if (!agentId) return;
    await query.refetch();
  }, [agentId, query]);

  const updateOverrides = useCallback(
    async (ov: AgentRoutingOverrides) => {
      if (!agentId) return;
      await updateAgentRouting(agentId, ov);
      await refresh();
    },
    [agentId, refresh],
  );

  const data = query.data;
  const error = queryErrorMessage(query.error);

  return { data, isLoading: query.isPending, error, refresh, updateOverrides };
}
