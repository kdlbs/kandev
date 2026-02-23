import { useEffect } from "react";
import { useAppStore } from "@/components/state-provider";
import { listAgentDiscovery } from "@/lib/api";

export function useAgentDiscovery(enabled = true) {
  const agentDiscovery = useAppStore((state) => state.agentDiscovery);
  const setAgentDiscovery = useAppStore((state) => state.setAgentDiscovery);
  const setAgentDiscoveryLoading = useAppStore((state) => state.setAgentDiscoveryLoading);

  useEffect(() => {
    if (!enabled) return;
    if (agentDiscovery.loaded || agentDiscovery.loading) return;
    setAgentDiscoveryLoading(true);
    listAgentDiscovery({ cache: "no-store" })
      .then((response) => {
        setAgentDiscovery(response.agents);
      })
      .catch(() => setAgentDiscovery([]));
  }, [
    agentDiscovery.loaded,
    agentDiscovery.loading,
    enabled,
    setAgentDiscovery,
    setAgentDiscoveryLoading,
  ]);

  return agentDiscovery;
}
