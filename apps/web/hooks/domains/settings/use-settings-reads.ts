"use client";

import { useCallback } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { settingsQueryOptions } from "@/lib/query/query-options/settings";
import { qk } from "@/lib/query/keys";
import type { Agent, Executor } from "@/lib/types/http";
import type { AgentProfile } from "@/lib/types/agent-profile";
import type { AgentProfileOption, InstallJob } from "@/lib/types/settings";
import { toAgentProfileOption } from "@/lib/types/settings";

/**
 * Read-side hooks for the settings domain that observe the TanStack Query
 * cache. The cache is populated by SSR seeds (StateHydrator), the per-domain
 * query fetches, and the WS→TQ bridge. These replace the direct
 * `useAppStore(state.<settings field>)` reads from the removed Zustand mirror.
 *
 * Pass `enabled: false` to observe-only (read whatever a sibling consumer
 * already fetched without triggering a fetch of this query).
 */

export function useExecutors(enabled = true): Executor[] {
  const { data } = useQuery({ ...settingsQueryOptions.executors(), enabled });
  return data ?? [];
}

export function useSettingsAgents(enabled = true): Agent[] {
  const { data } = useQuery({ ...settingsQueryOptions.agents(), enabled });
  return data ?? [];
}

export function useAgentProfiles(enabled = true): AgentProfileOption[] {
  const { data } = useQuery({ ...settingsQueryOptions.agentProfiles(), enabled });
  return data ?? [];
}

/**
 * Returns a callback that optimistically upserts an agent profile into the
 * `qk.settings.agentProfiles()` TQ cache after an inline create/edit, so the
 * picker reflects the change immediately without waiting for the WS bridge.
 */
export function useUpsertAgentProfileOption(): (saved: AgentProfile) => void {
  const qc = useQueryClient();
  return useCallback(
    (saved: AgentProfile) => {
      const agents = qc.getQueryData<Agent[]>(qk.settings.agents()) ?? [];
      const stub = agents.find((a) => a.id === saved.agentId) ?? {
        id: saved.agentId ?? "",
        name: saved.agentId ?? "",
      };
      const option = toAgentProfileOption(stub, saved);
      qc.setQueryData<AgentProfileOption[]>(qk.settings.agentProfiles(), (prev) => {
        const items = prev ?? [];
        return [...items.filter((p) => p.id !== option.id), option];
      });
    },
    [qc],
  );
}

/**
 * Returns a setter that writes the executors list into the
 * `qk.settings.executors()` cache (replaces the Zustand `setExecutors` action
 * used by the executor settings pages for optimistic CRUD updates).
 */
export function useSetExecutors(): (next: Executor[]) => void {
  const qc = useQueryClient();
  return useCallback(
    (next: Executor[]) => {
      qc.setQueryData<Executor[]>(qk.settings.executors(), next);
    },
    [qc],
  );
}

/**
 * Writes the agents list into `qk.settings.agents()` and derives the
 * `qk.settings.agentProfiles()` cache from it. Replaces the paired Zustand
 * `setSettingsAgents` + `setAgentProfiles` writes after agent/profile saves.
 */
export function useSetAgentsAndProfiles(): (next: Agent[]) => void {
  const qc = useQueryClient();
  return useCallback(
    (next: Agent[]) => {
      qc.setQueryData<Agent[]>(qk.settings.agents(), next);
      qc.setQueryData<AgentProfileOption[]>(
        qk.settings.agentProfiles(),
        next.flatMap((agent) =>
          agent.profiles.map((profile) => toAgentProfileOption(agent, profile)),
        ),
      );
    },
    [qc],
  );
}

export function useInstallJobsByAgent(enabled = true): Record<string, InstallJob> {
  const { data } = useQuery({ ...settingsQueryOptions.installJobs(), enabled });
  const jobs = data ?? [];
  const byAgent: Record<string, InstallJob> = {};
  for (const job of jobs) {
    // If two jobs target the same agent (a current run + a stale finished
    // snapshot in retention), prefer the newest start.
    const existing = byAgent[job.agent_name];
    if (!existing || Date.parse(job.started_at) > Date.parse(existing.started_at)) {
      byAgent[job.agent_name] = job;
    }
  }
  return byAgent;
}
