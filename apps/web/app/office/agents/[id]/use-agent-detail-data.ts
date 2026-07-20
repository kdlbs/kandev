"use client";

import { useCallback } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useAppStore } from "@/components/state-provider";
import { useOfficeAgentsData } from "@/hooks/domains/office/use-office-data";
import { qk } from "@/lib/query/keys";
import {
  officeRoutinesQueryOptions,
  officeSkillsQueryOptions,
} from "@/lib/query/query-options/office";
import type { AgentProfile } from "@/lib/state/slices/office/types";

export function useOfficeAgentProfiles() {
  const workspaceId = useAppStore((s) => s.workspaces.activeId);
  return useOfficeAgentsData(workspaceId).data?.agents ?? [];
}

export function useOfficeAgentProfile(agentId: string) {
  return useOfficeAgentProfiles().find((agent) => agent.id === agentId);
}

export function useActiveOfficeRoutines() {
  const workspaceId = useAppStore((s) => s.workspaces.activeId);
  const routinesQuery = useQuery(officeRoutinesQueryOptions(workspaceId ?? ""));
  return routinesQuery.data?.routines ?? [];
}

export function useActiveOfficeSkills() {
  const workspaceId = useAppStore((s) => s.workspaces.activeId);
  const skillsQuery = useQuery(officeSkillsQueryOptions(workspaceId ?? ""));
  return skillsQuery.data?.skills ?? [];
}

export function usePatchOfficeAgentProfileCache() {
  const workspaceId = useAppStore((s) => s.workspaces.activeId);
  const queryClient = useQueryClient();

  return useCallback(
    (agentId: string, patch: Partial<AgentProfile>) => {
      if (!workspaceId) return;

      queryClient.setQueryData<{ agents: AgentProfile[] }>(
        qk.office.agents(workspaceId),
        (current) => {
          if (!current) return current;
          return {
            ...current,
            agents: current.agents.map((agent) =>
              agent.id === agentId ? { ...agent, ...patch } : agent,
            ),
          };
        },
      );
    },
    [queryClient, workspaceId],
  );
}
