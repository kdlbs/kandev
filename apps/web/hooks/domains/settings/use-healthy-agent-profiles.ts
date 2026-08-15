"use client";

import { useAppStore } from "@/components/state-provider";
import { useFeature } from "@/hooks/domains/features/use-feature";
import type { AgentProfileOption } from "@/lib/state/slices";

/**
 * Returns agent profiles that are healthy (no capability issues). If `selectedId`
 * is provided and the selected profile is unhealthy, it is still included so the
 * user can see what's currently set instead of seeing a blank select.
 */
export function useHealthyAgentProfiles(selectedId?: string): AgentProfileOption[] {
  const agentProfiles = useAppStore((s) => s.agentProfiles.items);
  const dynamicRoutingEnabled = useFeature("dynamicAgentRouting");
  return agentProfiles.filter(
    (p) =>
      (dynamicRoutingEnabled || p.kind !== "dynamic") &&
      (!p.capability_status ||
        p.capability_status === "ok" ||
        p.capability_status === "probing" ||
        p.id === selectedId),
  );
}
