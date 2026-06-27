import { useCallback } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { qk } from "@/lib/query/keys";
import type { Agent, AgentProfile } from "@/lib/types/http";
import { useSettingsData } from "./use-settings-data";

type AgentsQueryData = { agents: Agent[]; total?: number };

export function upsertProfileInAgents(agents: Agent[], profile: AgentProfile): Agent[] {
  const agentId = profile.agentId;
  if (!agentId) return agents;
  let foundAgent = false;
  const nextAgents = agents.map((agent) => {
    if (agent.id !== agentId) return agent;
    foundAgent = true;
    const foundProfile = agent.profiles.some((item) => item.id === profile.id);
    return {
      ...agent,
      profiles: foundProfile
        ? agent.profiles.map((item) => (item.id === profile.id ? profile : item))
        : [...agent.profiles, profile],
    };
  });
  return foundAgent ? nextAgents : agents;
}

export function useAgentsQuerySync() {
  const queryClient = useQueryClient();
  const { settingsAgents } = useSettingsData(true);

  const setAgents = useCallback(
    (agents: Agent[]) => {
      queryClient.setQueryData<AgentsQueryData>(qk.settings.agents(), (previous) => ({
        ...(previous ?? {}),
        agents,
        total: agents.length,
      }));
    },
    [queryClient],
  );

  const upsertProfile = useCallback(
    (profile: AgentProfile) => {
      const nextAgents = upsertProfileInAgents(settingsAgents, profile);
      if (nextAgents === settingsAgents) {
        queryClient.invalidateQueries({ queryKey: qk.settings.agents() });
        return;
      }
      setAgents(nextAgents);
    },
    [queryClient, setAgents, settingsAgents],
  );

  return { settingsAgents, setAgents, upsertProfile };
}
