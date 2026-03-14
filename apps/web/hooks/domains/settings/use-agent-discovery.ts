import { useEffect, useRef } from "react";
import { useAppStore } from "@/components/state-provider";
import { listAgentDiscovery } from "@/lib/api";

const DISCOVERY_TIMEOUT_MS = 20_000;

export function useAgentDiscovery(enabled = true) {
  const agentDiscovery = useAppStore((state) => state.agentDiscovery);
  const setAgentDiscovery = useAppStore((state) => state.setAgentDiscovery);
  const setAgentDiscoveryLoading = useAppStore((state) => state.setAgentDiscoveryLoading);
  const fetchingRef = useRef(false);

  useEffect(() => {
    if (!enabled) return;
    if (agentDiscovery.loaded) return;
    if (fetchingRef.current) return;
    fetchingRef.current = true;
    setAgentDiscoveryLoading(true);

    const controller = new AbortController();
    const timeoutId = setTimeout(() => controller.abort(), DISCOVERY_TIMEOUT_MS);

    listAgentDiscovery({ cache: "no-store", init: { signal: controller.signal } })
      .then((response) => {
        setAgentDiscovery(response.agents);
      })
      .catch(() => {
        setAgentDiscovery([]);
      })
      .finally(() => {
        fetchingRef.current = false;
        clearTimeout(timeoutId);
      });

    return () => {
      clearTimeout(timeoutId);
      controller.abort();
    };
  }, [enabled, agentDiscovery.loaded, setAgentDiscovery, setAgentDiscoveryLoading]);

  return agentDiscovery;
}
