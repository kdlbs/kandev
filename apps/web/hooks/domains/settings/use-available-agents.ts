import { useEffect } from "react";
import { useAppStore } from "@/components/state-provider";
import { listAvailableAgents } from "@/lib/api";
import type { AvailableAgent } from "@/lib/types/http";

const CAPABILITY_REVALIDATION_DELAY_MS = 250;
const CAPABILITY_REVALIDATION_MAX_ATTEMPTS = 240;
let capabilityRevalidation: Promise<void> | null = null;

function hasPendingDynamicCapabilities(agents: AvailableAgent[]): boolean {
  return agents.some(
    (agent) =>
      agent.model_config.supports_dynamic_models &&
      (agent.model_config.status === "not_configured" || agent.model_config.status === "probing"),
  );
}

export function useAvailableAgents(enabled = true) {
  const availableAgents = useAppStore((state) => state.availableAgents);
  const setAvailableAgents = useAppStore((state) => state.setAvailableAgents);
  const setAvailableAgentsLoading = useAppStore((state) => state.setAvailableAgentsLoading);

  useEffect(() => {
    if (!enabled) return;
    if (availableAgents.loaded || availableAgents.loading) return;
    setAvailableAgentsLoading(true);
    listAvailableAgents({ cache: "no-store" })
      .then((response) => {
        setAvailableAgents(response.agents, response.tools ?? []);
      })
      .catch(() => setAvailableAgents([]));
  }, [
    availableAgents.loaded,
    availableAgents.loading,
    enabled,
    setAvailableAgents,
    setAvailableAgentsLoading,
  ]);

  useEffect(() => {
    if (
      !enabled ||
      !availableAgents.loaded ||
      availableAgents.loading ||
      !hasPendingDynamicCapabilities(availableAgents.items) ||
      capabilityRevalidation
    ) {
      return;
    }

    capabilityRevalidation = (async () => {
      for (let attempt = 0; attempt <= CAPABILITY_REVALIDATION_MAX_ATTEMPTS; attempt += 1) {
        const response = await listAvailableAgents({ cache: "no-store" });
        setAvailableAgents(response.agents, response.tools ?? []);
        if (!hasPendingDynamicCapabilities(response.agents)) return;
        if (attempt === CAPABILITY_REVALIDATION_MAX_ATTEMPTS) return;
        await new Promise((resolve) => setTimeout(resolve, CAPABILITY_REVALIDATION_DELAY_MS));
      }
    })()
      .catch(() => undefined)
      .finally(() => {
        capabilityRevalidation = null;
      });
  }, [
    availableAgents.items,
    availableAgents.loaded,
    availableAgents.loading,
    enabled,
    setAvailableAgents,
  ]);

  return availableAgents;
}
