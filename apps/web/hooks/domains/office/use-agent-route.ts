"use client";

import { useCallback, useEffect } from "react";
import { useQuery } from "@tanstack/react-query";
import { useAppStore } from "@/components/state-provider";
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
  const storeData = useAppStore((s) =>
    agentId ? s.office.agentRouting.byAgentId[agentId] : undefined,
  );
  const setAgentRouting = useAppStore((s) => s.setAgentRouting);
  const query = useQuery(officeAgentRouteQueryOptions(agentId ?? ""));

  const refresh = useCallback(async () => {
    if (!agentId) return;
    await query.refetch();
  }, [agentId, query]);

  useEffect(() => {
    if (!agentId || !query.data) return;
    setAgentRouting(agentId, query.data);
  }, [agentId, query.data, setAgentRouting]);

  const updateOverrides = useCallback(
    async (ov: AgentRoutingOverrides) => {
      if (!agentId) return;
      await updateAgentRouting(agentId, ov);
      await refresh();
    },
    [agentId, refresh],
  );

  const data = query.data ?? storeData;
  const error = queryErrorMessage(query.error);

  return { data, isLoading: query.isPending, error, refresh, updateOverrides };
}
