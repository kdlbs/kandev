"use client";

import { useCallback, useEffect, useState } from "react";
import { useAppStore } from "@/components/state-provider";
import { getAgentRoute, updateAgentRouting } from "@/lib/api/domains/office-extended-api";
import type { AgentRouteData, AgentRoutingOverrides } from "@/lib/state/slices/office/types";

export type UseAgentRouteResult = {
  data: AgentRouteData | undefined;
  isLoading: boolean;
  error: string | null;
  refresh: () => Promise<void>;
  updateOverrides: (ov: AgentRoutingOverrides) => Promise<void>;
};

export function useAgentRoute(agentId: string | null): UseAgentRouteResult {
  const data = useAppStore((s) => (agentId ? s.office.agentRouting.byAgentId[agentId] : undefined));
  const setAgentRouting = useAppStore((s) => s.setAgentRouting);
  const [isLoading, setIsLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const refresh = useCallback(async () => {
    if (!agentId) return;
    setIsLoading(true);
    setError(null);
    try {
      const res = await getAgentRoute(agentId);
      setAgentRouting(agentId, res);
    } catch (e) {
      setError(e instanceof Error ? e.message : "Failed to load agent route");
    } finally {
      setIsLoading(false);
    }
  }, [agentId, setAgentRouting]);

  useEffect(() => {
    if (!agentId) return;
    if (data !== undefined) return;
    void refresh();
  }, [agentId, data, refresh]);

  const updateOverrides = useCallback(
    async (ov: AgentRoutingOverrides) => {
      if (!agentId) return;
      await updateAgentRouting(agentId, ov);
      await refresh();
    },
    [agentId, refresh],
  );

  return { data, isLoading, error, refresh, updateOverrides };
}
