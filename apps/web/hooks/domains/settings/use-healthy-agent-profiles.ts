"use client";

import { useQuery } from "@tanstack/react-query";
import { settingsQueryOptions } from "@/lib/query/query-options/settings";
import type { AgentProfileOption } from "@/lib/types/settings";

/**
 * Returns agent profiles that are healthy (no capability issues). If `selectedId`
 * is provided and the selected profile is unhealthy, it is still included so the
 * user can see what's currently set instead of seeing a blank select.
 */
export function useHealthyAgentProfiles(selectedId?: string): AgentProfileOption[] {
  const { data: profiles = [] } = useQuery(settingsQueryOptions.agentProfiles());
  return profiles.filter(
    (p) =>
      !p.capability_status ||
      p.capability_status === "ok" ||
      p.capability_status === "probing" ||
      p.id === selectedId,
  );
}
