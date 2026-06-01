"use client";

import { useQuery } from "@tanstack/react-query";
import { settingsQueryOptions } from "@/lib/query/query-options/settings";

/**
 * Triggers the settings-domain TanStack Query fetches (executors, agents,
 * agent profiles, available agents) for callers that need the data populated
 * in the cache. Reads happen via `useExecutors` / `useAgentProfiles` /
 * `useSettingsAgents` / `useAvailableAgents`; this hook only drives the fetch.
 *
 * `enabled` is preserved for callers that conditionally activate loading.
 */
export function useSettingsData(enabled = true) {
  useQuery({ ...settingsQueryOptions.executors(), enabled });
  useQuery({ ...settingsQueryOptions.agents(), enabled });
  useQuery({ ...settingsQueryOptions.agentProfiles(), enabled });
  useQuery({ ...settingsQueryOptions.availableAgents(), enabled });
}
