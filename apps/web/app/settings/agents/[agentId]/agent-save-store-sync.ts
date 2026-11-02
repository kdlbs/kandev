import type { Agent } from "@/lib/types/http";
import type { AppState } from "@/lib/state/store";
import type { StoreApi } from "zustand";
import { parseTurnTimestamp } from "@/lib/state/slices/session/turn-actions";
import {
  orderProfilesForSelection,
  toSelectorProfileOptions,
} from "@/lib/settings/agent-profile-selector-order";

export function syncSavedAgentToStore(
  store: StoreApi<AppState>,
  agent: Agent,
  profileVersionAtSaveStart: number,
  allowMissingAgent = false,
) {
  const state = store.getState();
  const settingsAgents = state.settingsAgents.items;
  const existing = settingsAgents.find((item) => item.id === agent.id);
  if (!existing && state.agentProfiles.version !== profileVersionAtSaveStart && !allowMissingAgent)
    return;
  const { profiles, membershipChanged } = reconcileSavedProfiles(
    existing,
    agent,
    state.agentProfiles.version === profileVersionAtSaveStart,
  );

  const reconciled = existing
    ? {
        ...existing,
        name: agent.name,
        workspace_id: agent.workspace_id,
        mcp_config_path: agent.mcp_config_path,
        profiles,
      }
    : { ...agent, profiles };
  const nextAgents = existing
    ? settingsAgents.map((item) => (item.id === agent.id ? reconciled : item))
    : [...settingsAgents, reconciled];
  state.setSettingsAgents(nextAgents);
  state.setAgentProfiles(reconcileSavedOptions(state.agentProfiles.items, existing, reconciled));
  if (membershipChanged) state.bumpAgentProfilesVersion();
  return reconciled;
}

function acceptedSavedProfile(
  current: Agent["profiles"][number],
  incoming: Agent["profiles"][number] | undefined,
) {
  const currentTime = parseTurnTimestamp(current.updatedAt);
  const incomingTime = parseTurnTimestamp(incoming?.updatedAt);
  return incoming && currentTime !== null && (incomingTime === null || incomingTime < currentTime)
    ? current
    : incoming;
}

function reconcileSavedProfiles(existing: Agent | undefined, incoming: Agent, fresh: boolean) {
  if (!fresh)
    return { profiles: existing?.profiles ?? incoming.profiles, membershipChanged: false };
  const incomingById = new Map(incoming.profiles.map((profile) => [profile.id, profile]));
  const existingIds = new Set(existing?.profiles.map((profile) => profile.id) ?? []);
  const newProfiles = incoming.profiles.filter((profile) => !existingIds.has(profile.id));
  const existingProfiles = (existing?.profiles ?? [])
    .map((profile) => acceptedSavedProfile(profile, incomingById.get(profile.id)))
    .filter((profile): profile is Agent["profiles"][number] => profile !== undefined);
  const membershipChanged =
    newProfiles.length > 0 ||
    (existing !== undefined && existing.profiles.some((profile) => !incomingById.has(profile.id)));
  return {
    profiles: [...orderProfilesForSelection(newProfiles), ...existingProfiles],
    membershipChanged,
  };
}

function reconcileSavedOptions(
  options: AppState["agentProfiles"]["items"],
  existing: Agent | undefined,
  reconciled: Agent,
) {
  const savedProfileOptions = toSelectorProfileOptions([reconciled]);
  const savedIds = new Set(savedProfileOptions.map((profile) => profile.id));
  const previousIds = new Set<string>(existing?.profiles.map((profile) => profile.id) ?? []);
  const nextOptions: typeof options = [];
  let inserted = false;
  for (const option of options) {
    if (option.agent_id !== reconciled.id) {
      nextOptions.push(option);
      continue;
    }
    if (!inserted) {
      nextOptions.push(...savedProfileOptions);
      inserted = true;
    }
    if (!savedIds.has(option.id) && (option.workspace_id || !previousIds.has(option.id)))
      nextOptions.push(option);
  }
  if (!inserted) nextOptions.push(...savedProfileOptions);
  return nextOptions;
}
