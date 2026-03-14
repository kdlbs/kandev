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
    let timedOut = false;
    const controller = new AbortController();
    const timeoutId = setTimeout(() => {
      timedOut = true;
      controller.abort();
    }, DISCOVERY_TIMEOUT_MS);

    listAgentDiscovery({ cache: "no-store", init: { signal: controller.signal } })
      .then((response) => {
        if (cancelled) return;
        setAgentDiscovery(response.agents);
      })
      .catch(() => {
        // Only set empty state for real errors (timeout, network failure).
        // Skip on cleanup abort to avoid poisoning the cache with empty data
        // (React strict mode double-mounts would otherwise set loaded=true
        // with no agents before the second mount can fetch).
        if (!cancelled || timedOut) setAgentDiscovery([]);
      })
      .finally(() => {
        clearTimeout(timeoutId);
      });

    return () => {
      cancelled = true;
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
