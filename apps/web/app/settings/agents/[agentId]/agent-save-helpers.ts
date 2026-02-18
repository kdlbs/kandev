import {
  createAgentAction,
  createAgentProfileAction,
  deleteAgentProfileAction,
  updateAgentAction,
  updateAgentProfileAction,
  updateAgentProfileMcpConfigAction,
} from '@/app/actions/agents';
import type { Agent, AgentProfile, McpServerDef, PermissionSetting, ModelConfig } from '@/lib/types/http';
import { permissionsToProfilePatch } from '@/lib/agent-permissions';

type DraftMcpConfig = {
  enabled: boolean;
  servers: string;
  dirty: boolean;
  error: string | null;
};

export type DraftProfile = Omit<AgentProfile, 'allow_indexing'> & {
  allow_indexing?: boolean;
  isNew?: boolean;
  mcp_config?: DraftMcpConfig;
};

export type DraftAgent = Omit<Agent, 'profiles'> & { profiles: DraftProfile[]; isNew?: boolean };

export const parseProfileMcpServers = (raw: string): Record<string, McpServerDef> => {
  if (!raw.trim()) return {};
  const parsed = JSON.parse(raw) as unknown;
  if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) {
    throw new Error('MCP servers config must be a JSON object');
  }
  if ('mcpServers' in parsed) {
    const nested = (parsed as { mcpServers?: unknown }).mcpServers;
    if (!nested || typeof nested !== 'object' || Array.isArray(nested)) {
      throw new Error('mcpServers must be a JSON object');
    }
    return nested as Record<string, McpServerDef>;
  }
  return parsed as Record<string, McpServerDef>;
};

type SaveMcpForProfileParams = {
  draftProfile: DraftProfile;
  targetProfileId: string;
  onToastError: (error: unknown) => void;
};

async function saveMcpForProfile({ draftProfile, targetProfileId, onToastError }: SaveMcpForProfileParams) {
  if (!draftProfile.mcp_config?.dirty || !draftProfile.mcp_config.servers.trim()) return;
  try {
    const servers = parseProfileMcpServers(draftProfile.mcp_config.servers);
    await updateAgentProfileMcpConfigAction(targetProfileId, {
      enabled: draftProfile.mcp_config.enabled,
      mcpServers: servers,
    });
  } catch (error) {
    onToastError(error);
  }
}

async function saveMcpForCreatedProfiles(
  draftAgent: DraftAgent,
  created: Agent,
  onToastError: (error: unknown) => void
) {
  if (created.profiles.length === draftAgent.profiles.length) {
    for (let index = 0; index < draftAgent.profiles.length; index += 1) {
      await saveMcpForProfile({
        draftProfile: draftAgent.profiles[index],
        targetProfileId: created.profiles[index].id,
        onToastError,
      });
    }
    return;
  }
  for (const draftProfile of draftAgent.profiles) {
    const createdProfile = created.profiles.find((profile) => profile.name === draftProfile.name);
    if (!createdProfile) continue;
    await saveMcpForProfile({
      draftProfile,
      targetProfileId: createdProfile.id,
      onToastError,
    });
  }
}

export type EnsureProfilesFn = (
  agent: DraftAgent,
  displayName: string,
  defaultModel: string,
  permissions?: Record<string, PermissionSetting>
) => DraftAgent;

export type CloneAgentFn = (agent: Agent) => DraftAgent;

export type SaveAgentCallbacks = {
  onToastError: (error: unknown) => void;
  currentAgentModelConfig: ModelConfig;
  permissionSettings: Record<string, PermissionSetting>;
  resolveDisplayName: (name: string) => string;
  upsertAgent: (agent: Agent) => void;
  setDraftAgent: (agent: DraftAgent) => void;
  ensureProfiles: EnsureProfilesFn;
  cloneAgent: CloneAgentFn;
  replaceRoute: (path: string) => void;
};

export async function saveNewAgent(
  draftAgent: DraftAgent,
  callbacks: SaveAgentCallbacks
) {
  let created = await createAgentAction({
    name: draftAgent.name,
    workspace_id: draftAgent.workspace_id,
    profiles: draftAgent.profiles.map((profile) => ({
      name: profile.name,
      model: profile.model,
      ...permissionsToProfilePatch(profile),
      cli_passthrough: profile.cli_passthrough ?? false,
    })),
  });

  await saveMcpForCreatedProfiles(draftAgent, created, callbacks.onToastError);

  if ((draftAgent.mcp_config_path ?? '') !== (created.mcp_config_path ?? '')) {
    created = await updateAgentAction(created.id, {
      mcp_config_path: draftAgent.mcp_config_path ?? '',
    });
  }
  callbacks.upsertAgent(created);
  callbacks.setDraftAgent(
    callbacks.ensureProfiles(
      callbacks.cloneAgent(created),
      callbacks.resolveDisplayName(created.name),
      callbacks.currentAgentModelConfig.default_model,
      callbacks.permissionSettings
    )
  );
  callbacks.replaceRoute(`/settings/agents/${encodeURIComponent(created.name)}`);
}

async function saveExistingAgentPatch(draftAgent: DraftAgent, savedAgent: Agent) {
  const agentPatch: { workspace_id?: string | null; mcp_config_path?: string | null } = {};
  if ((draftAgent.workspace_id ?? null) !== (savedAgent.workspace_id ?? null)) {
    agentPatch.workspace_id = draftAgent.workspace_id ?? null;
  }
  if ((draftAgent.mcp_config_path ?? '') !== (savedAgent.mcp_config_path ?? '')) {
    agentPatch.mcp_config_path = draftAgent.mcp_config_path ?? '';
  }
  if (Object.keys(agentPatch).length > 0) {
    await updateAgentAction(savedAgent.id, agentPatch);
  }
}

async function saveExistingProfiles(
  draftAgent: DraftAgent,
  savedAgent: Agent,
  isCreateMode: boolean,
  onToastError: (error: unknown) => void
): Promise<AgentProfile[]> {
  const savedProfilesById = new Map(savedAgent.profiles.map((p) => [p.id, p]));
  const nextProfiles: AgentProfile[] = isCreateMode ? [...savedAgent.profiles] : [];

  for (const profile of draftAgent.profiles) {
    const savedProfile = savedProfilesById.get(profile.id);
    if (!savedProfile) {
      const createdProfile = await createAgentProfileAction(savedAgent.id, {
        name: profile.name,
        model: profile.model,
        ...permissionsToProfilePatch(profile),
        cli_passthrough: profile.cli_passthrough ?? false,
      });
      await saveMcpForProfile({ draftProfile: profile, targetProfileId: createdProfile.id, onToastError });
      nextProfiles.push(createdProfile);
      continue;
    }
    if (isProfileDirty(profile, savedProfile)) {
      const updatedProfile = await updateAgentProfileAction(profile.id, {
        name: profile.name,
        model: profile.model,
        ...permissionsToProfilePatch(profile),
      });
      nextProfiles.push(updatedProfile);
      continue;
    }
    nextProfiles.push(savedProfile);
  }
  return nextProfiles;
}

async function deleteRemovedProfiles(draftAgent: DraftAgent, savedAgent: Agent) {
  for (const savedProfile of savedAgent.profiles) {
    const stillExists = draftAgent.profiles.some((p) => p.id === savedProfile.id);
    if (!stillExists) {
      await deleteAgentProfileAction(savedProfile.id);
    }
  }
}

export async function saveExistingAgent(
  draftAgent: DraftAgent,
  savedAgent: Agent,
  isCreateMode: boolean,
  callbacks: SaveAgentCallbacks
) {
  await saveExistingAgentPatch(draftAgent, savedAgent);

  const nextProfiles = await saveExistingProfiles(draftAgent, savedAgent, isCreateMode, callbacks.onToastError);

  if (!isCreateMode) {
    await deleteRemovedProfiles(draftAgent, savedAgent);
  }

  const nextAgent = {
    ...savedAgent,
    workspace_id: draftAgent.workspace_id ?? null,
    mcp_config_path: draftAgent.mcp_config_path ?? '',
    profiles: nextProfiles,
  };
  callbacks.upsertAgent(nextAgent);
  callbacks.setDraftAgent(
    callbacks.ensureProfiles(
      callbacks.cloneAgent(nextAgent),
      callbacks.resolveDisplayName(nextAgent.name),
      callbacks.currentAgentModelConfig.default_model,
      callbacks.permissionSettings
    )
  );
  if (isCreateMode) {
    callbacks.replaceRoute(`/settings/agents/${encodeURIComponent(savedAgent.name)}`);
  }
}

import { arePermissionsDirty } from '@/lib/agent-permissions';

export function isProfileDirty(draft: DraftProfile, saved?: AgentProfile): boolean {
  if (!saved) return true;
  return (
    draft.name !== saved.name ||
    draft.model !== saved.model ||
    arePermissionsDirty(draft, saved) ||
    draft.cli_passthrough !== saved.cli_passthrough
  );
}
