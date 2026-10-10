import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import { insertFirstInAgentGroup } from "@/lib/settings/agent-profile-order";
import {
  orderProfilesForSelection,
  toSelectorProfileOptions,
} from "@/lib/settings/agent-profile-selector-order";
import { syncSavedAgentToStore } from "@/app/settings/agents/[agentId]/agent-save-store-sync";
import { parseTurnTimestamp } from "@/lib/state/slices/session/turn-actions";
import { wasSettingsAgentRemoved } from "@/lib/state/settings-agent-removals";
import type { Agent, AgentProfile } from "@/lib/types/http";

export type AgentCreationPublication = {
  profiles: AgentProfile[];
  ownerCreated?: boolean;
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
  const newProfiles = orderProfilesForSelection(
    publication.profiles.filter((profile) => !profiles.has(profile.id)),
  );
  for (const profile of publication.profiles) {
    profiles.set(profile.id, acceptedProfile(profiles.get(profile.id), profile));
  }
  return {
    ...current,
    ...publication.agentPatch,
    profiles: [
      ...newProfiles,
      ...[...profiles.values()].filter(
        (profile) => !newProfiles.some((created) => created.id === profile.id),
      ),
    ],
  };
}

export function useAgentCreationStoreSync() {
  const storeApi = useAppStoreApi();
  const setSettingsAgents = useAppStore((state) => state.setSettingsAgents);
  const setAgentProfiles = useAppStore((state) => state.setAgentProfiles);

  const getAgentProfilesVersion = () => storeApi.getState().agentProfiles.version;
  const upsertAgent = (
    agent: Agent,
    creation?: AgentCreationPublication,
    profileVersionAtSaveStart = getAgentProfilesVersion(),
  ) => {
    if (!creation) return syncSavedAgentToStore(storeApi, agent, profileVersionAtSaveStart);
    const agents = storeApi.getState().settingsAgents.items;
    const current = agents.find((item) => item.id === agent.id);
    if (!current && (!creation.ownerCreated || wasSettingsAgentRemoved(storeApi, agent.id))) return;
    const target = creation && current ? publishCreatedProfiles(current, creation) : agent;
    const next = current
      ? agents.map((item) => (item.id === agent.id ? target : item))
      : [...agents, target];
    setSettingsAgents(next);
    let options = storeApi.getState().agentProfiles.items;
    for (const accepted of toSelectorProfileOptions([target]).toReversed()) {
      options = options.some((option) => option.id === accepted.id)
        ? options.map((option) => (option.id === accepted.id ? accepted : option))
        : insertFirstInAgentGroup(options, target.id, accepted);
    }
    setAgentProfiles(options);
    storeApi.getState().bumpAgentProfilesVersion();
    return target;
  };

  return { getAgentProfilesVersion, upsertAgent };
}
