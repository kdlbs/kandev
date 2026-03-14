import { useEffect } from "react";
import { useAppStore } from "@/components/state-provider";
import { listAgentDiscovery } from "@/lib/api";

const DISCOVERY_TIMEOUT_MS = 20_000;

export function useAgentDiscovery(enabled = true) {
  const agentDiscovery = useAppStore((state) => state.agentDiscovery);
  const setAgentDiscovery = useAppStore((state) => state.setAgentDiscovery);
  const setAgentDiscoveryLoading = useAppStore((state) => state.setAgentDiscoveryLoading);

  useEffect(() => {
    if (!enabled) return;
    if (agentDiscovery.loaded || agentDiscovery.loading) return;
    setAgentDiscoveryLoading(true);

    let cancelled = false;
    const controller = new AbortController();
    const timeoutId = setTimeout(() => controller.abort(), DISCOVERY_TIMEOUT_MS);

    listAgentDiscovery({ cache: "no-store", init: { signal: controller.signal } })
      .then((response) => {
        if (cancelled) return;
        setAgentDiscovery(response.agents);
      })
      .catch(() => {
        if (!cancelled) setAgentDiscovery([]);
      })
      .finally(() => {
        clearTimeout(timeoutId);
      });

    return () => {
      cancelled = true;
      setAgentDiscoveryLoading(false);
      clearTimeout(timeoutId);
      controller.abort();
    };
  }, [
    agentDiscovery.loaded,
    agentDiscovery.loading,
    enabled,
    setAgentDiscovery,
    setAgentDiscoveryLoading,
  ]);

  return agentDiscovery;
}
