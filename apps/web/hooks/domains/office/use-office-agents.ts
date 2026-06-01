"use client";

import { useMemo } from "react";
import { useQuery } from "@tanstack/react-query";
import { useAppStore } from "@/components/state-provider";
import { officeQueryOptions } from "@/lib/query/query-options/office";
import type { AgentProfile } from "@/lib/state/slices/office/types";

const EMPTY_AGENTS: AgentProfile[] = [];

/**
 * Agent profiles for the active workspace, read from TanStack Query.
 *
 * Replaces the legacy `useAppStore(s => s.office.agentProfiles)` mirror read.
 * The active workspace id still comes from the Zustand UI slice (client-only
 * state); the agent data itself is fetched + cached by TQ and kept fresh by
 * the office WS bridge (which invalidates `qk.office.agents(wsId)`).
 */
export function useOfficeAgents(): AgentProfile[] {
  const workspaceId = useAppStore((s) => s.workspaces.activeId);
  const { data } = useQuery({
    ...officeQueryOptions.agents(workspaceId ?? ""),
    enabled: !!workspaceId,
  });
  return data ?? EMPTY_AGENTS;
}

/**
 * Resolves an agent profile's display name by id for the active workspace.
 * Returns `undefined` when the id is null/absent or the agent isn't loaded,
 * so callers can apply their own fallback (e.g. the session-snapshot name).
 */
export function useAgentName(agentId: string | null | undefined): string | undefined {
  const agents = useOfficeAgents();
  return useMemo(() => {
    if (!agentId) return undefined;
    return agents.find((a) => a.id === agentId)?.name;
  }, [agents, agentId]);
}

/**
 * Resolves a full agent profile by id for the active workspace, or
 * `undefined` when absent.
 */
export function useAgentProfile(agentId: string | null | undefined): AgentProfile | undefined {
  const agents = useOfficeAgents();
  return useMemo(() => {
    if (!agentId) return undefined;
    return agents.find((a) => a.id === agentId);
  }, [agents, agentId]);
}
