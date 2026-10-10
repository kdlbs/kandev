import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import { toAgentProfileOption } from "@/lib/state/slices/settings/types";
import { parseTurnTimestamp } from "@/lib/state/slices/session/turn-actions";
import type { Agent, AgentProfile } from "@/lib/types/http";

export type AgentCreationPublication = {
  profiles: AgentProfile[];
  agentPatch?: Pick<Agent, "workspace_id" | "mcp_config_path">;
};

function acceptedProfile(current: AgentProfile | undefined, accepted: AgentProfile) {
  const currentTime = parseTurnTimestamp(current?.updatedAt);
  const acceptedTime = parseTurnTimestamp(accepted.updatedAt);
  if (!current || currentTime === null || (acceptedTime !== null && currentTime <= acceptedTime))
    return accepted;
  return "mcp_config" in accepted ? { ...current, mcp_config: accepted.mcp_config } : current;
}

function publishCreatedProfiles(current: Agent, publication: AgentCreationPublication): Agent {
  const profiles = new Map(current.profiles.map((profile) => [profile.id, profile]));
  for (const profile of publication.profiles) {
    profiles.set(profile.id, acceptedProfile(profiles.get(profile.id), profile));
  }
  return { ...current, ...publication.agentPatch, profiles: [...profiles.values()] };
}

export function useAgentCreationStoreSync() {
  const storeApi = useAppStoreApi();
  const setSettingsAgents = useAppStore((state) => state.setSettingsAgents);
  const setAgentProfiles = useAppStore((state) => state.setAgentProfiles);

  const upsertAgent = (agent: Agent, creation?: AgentCreationPublication) => {
    const agents = storeApi.getState().settingsAgents.items;
    const current = agents.find((item) => item.id === agent.id);
    if (creation && !current) return;
    const target = creation && current ? publishCreatedProfiles(current, creation) : agent;
    const next = current
      ? agents.map((item) => (item.id === agent.id ? target : item))
      : [...agents, target];
    setSettingsAgents(next);
    setAgentProfiles(
      next.flatMap((item) => item.profiles.map((profile) => toAgentProfileOption(item, profile))),
    );
    return target;
  };

  return { upsertAgent };
}
