"use client";

import { useAppStore } from "@/components/state-provider";
import { useFeature } from "@/hooks/domains/features/use-feature";
import type { AgentProfileOption } from "@/lib/state/slices";

/**
 * True when a profile has no capability issues: an unset status (no host-utility
 * probe recorded yet, e.g. the e2e mock agent), "ok", or the in-flight "probing"
 * state. If `selectedId` is provided and the profile is the selected one, it is
 * still considered healthy so the user can see what's currently set instead of
 * seeing a blank select.
 */
export function isHealthyAgentProfile(profile: AgentProfileOption, selectedId?: string): boolean {
  return (
    !profile.capability_status ||
    profile.capability_status === "ok" ||
    profile.capability_status === "probing" ||
    profile.id === selectedId
  );
}

/**
 * Returns agent profiles that are healthy (no capability issues). If `selectedId`
 * is provided and the selected profile is unhealthy, it is still included so the
 * user can see what's currently set instead of seeing a blank select.
 */
export function useHealthyAgentProfiles(selectedId?: string): AgentProfileOption[] {
  const agentProfiles = useAppStore((s) => s.agentProfiles.items);
  const dynamicRoutingEnabled = useFeature("dynamicAgentRouting");
  return agentProfiles.filter(
    (p) => (dynamicRoutingEnabled || p.kind !== "dynamic") && isHealthyAgentProfile(p, selectedId),
  );
}
