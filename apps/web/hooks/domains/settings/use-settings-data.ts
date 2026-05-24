"use client";

import { useQuery } from "@tanstack/react-query";
import { settingsQueryOptions } from "@/lib/query/query-options/settings";

/**
 * Loads executors, agents (+ agent profiles), and available agents.
 * In the TQ world, each data type is its own query — dedup and stale-time
 * handle the "load once" semantics that the old Zustand loading flags provided.
 *
 * `enabled` prop is preserved for callers that conditionally activate loading.
 */
export function useSettingsData(enabled = true) {
  useQuery({ ...settingsQueryOptions.executors(), enabled });
  useQuery({ ...settingsQueryOptions.agents(), enabled });
  useQuery({ ...settingsQueryOptions.agentProfiles(), enabled });
  useQuery({ ...settingsQueryOptions.availableAgents(), enabled });
}
