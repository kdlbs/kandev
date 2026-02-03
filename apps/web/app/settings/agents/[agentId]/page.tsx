'use client';

import { useEffect, useMemo, useState } from 'react';
import Link from 'next/link';
import { useParams, useRouter, useSearchParams } from 'next/navigation';
import { IconPlus } from '@tabler/icons-react';
import { Button } from '@kandev/ui/button';
import { Card, CardContent, CardHeader, CardTitle } from '@kandev/ui/card';
import { Separator } from '@kandev/ui/separator';
import { useToast } from '@/components/toast-provider';
import { UnsavedChangesBadge, UnsavedSaveButton } from '@/components/settings/unsaved-indicator';
import { ProfileFormFields } from '@/components/settings/profile-form-fields';
import {
  createAgentAction,
  createAgentProfileAction,
  deleteAgentProfileAction,
  updateAgentAction,
  updateAgentProfileAction,
  updateAgentProfileMcpConfigAction,
} from '@/app/actions/agents';
import type { Agent, AgentDiscovery, AgentProfile, McpServerDef, AvailableAgent, PermissionSetting } from '@/lib/types/http';
import { generateUUID } from '@/lib/utils';
import { useAppStore } from '@/components/state-provider';
import { useAvailableAgents } from '@/hooks/domains/settings/use-available-agents';
import { ProfileMcpConfigCard } from './profile-mcp-config-card';

type DraftProfile = Omit<AgentProfile, 'allow_indexing'> & {
  allow_indexing?: boolean;
  isNew?: boolean;
  mcp_config?: {
    enabled: boolean;
    servers: string;
    dirty: boolean;
    error: string | null;
  };
};
type DraftAgent = Omit<Agent, 'profiles'> & { profiles: DraftProfile[]; isNew?: boolean };

const defaultMcpConfig: NonNullable<DraftProfile['mcp_config']> = {
  enabled: false,
  servers: '{\n  "mcpServers": {}\n}',
  dirty: false,
  error: null,
};

type AgentSetupFormProps = {
  initialAgent: DraftAgent;
  savedAgent: Agent | null;
  discoveryAgent: AgentDiscovery | undefined;
  onToastError: (error: unknown) => void;
  isCreateMode?: boolean;
};

const createDraftProfile = (
  agentId: string,
  agentDisplayName: string,
  defaultModel: string,
  permissionSettings?: Record<string, PermissionSetting>
): DraftProfile => ({
  id: `draft-${generateUUID()}`,
  agent_id: agentId,
  name: '',
  agent_display_name: agentDisplayName,
  model: defaultModel,
  auto_approve: permissionSettings?.auto_approve?.default ?? false,
  dangerously_skip_permissions: permissionSettings?.dangerously_skip_permissions?.default ?? false,
  allow_indexing: permissionSettings?.allow_indexing?.default ?? false,
  cli_passthrough: false,
  created_at: new Date().toISOString(),
  updated_at: new Date().toISOString(),
  isNew: true,
  mcp_config: { ...defaultMcpConfig },
});

const cloneAgent = (agent: Agent): DraftAgent => ({
  ...agent,
  workspace_id: agent.workspace_id ?? null,
  mcp_config_path: agent.mcp_config_path ?? '',
  profiles: agent.profiles.map((profile) => ({ ...profile })) as DraftProfile[],
});

const ensureProfiles = (
  agent: DraftAgent,
  agentDisplayName: string,
  defaultModel: string,
  permissionSettings?: Record<string, PermissionSetting>
): DraftAgent => {
  if (agent.profiles.length > 0) {
    return agent;
  }
  return {
    ...agent,
    profiles: [createDraftProfile(agent.id, agentDisplayName, defaultModel, permissionSettings)],
  };
};

function AgentSetupForm({
  initialAgent,
  savedAgent,
  discoveryAgent,
  onToastError,
  isCreateMode = false,
}: AgentSetupFormProps) {
  const router = useRouter();
  const settingsAgents = useAppStore((state) => state.settingsAgents.items);
  const setSettingsAgents = useAppStore((state) => state.setSettingsAgents);
  const setAgentProfiles = useAppStore((state) => state.setAgentProfiles);
  const availableAgents = useAvailableAgents().items;
  const [draftAgent, setDraftAgent] = useState<DraftAgent>(initialAgent);
  const [saveStatus, setSaveStatus] = useState<'idle' | 'loading' | 'success' | 'error'>('idle');
  const [newProfileId, setNewProfileId] = useState<string | null>(null);
  const resolveDisplayName = (name: string) =>
    availableAgents.find((item: AvailableAgent) => item.name === name)?.display_name ?? '';
  const hasInvalidMcpConfig = useMemo(() => {
    return draftAgent.profiles.some((profile) => Boolean(profile.mcp_config?.error));
  }, [draftAgent.profiles]);

  // Get model config and permission settings for the current agent
  const currentAvailableAgent = useMemo(() => {
    return availableAgents.find((item: AvailableAgent) => item.name === draftAgent.name) ?? null;
  }, [availableAgents, draftAgent.name]);

  const currentAgentModelConfig = useMemo(() => {
    return currentAvailableAgent?.model_config ?? { default_model: '', available_models: [], supports_dynamic_models: false };
  }, [currentAvailableAgent]);

  const permissionSettings = useMemo(() => {
    return currentAvailableAgent?.permission_settings ?? {};
  }, [currentAvailableAgent]);

  const passthroughConfig = useMemo(() => {
    return currentAvailableAgent?.passthrough_config ?? null;
  }, [currentAvailableAgent]);

  const isProfileDirty = (draft: DraftProfile, saved?: AgentProfile) => {
    if (!saved) return true;
    return (
      draft.name !== saved.name ||
      draft.model !== saved.model ||
      draft.auto_approve !== saved.auto_approve ||
      draft.dangerously_skip_permissions !== saved.dangerously_skip_permissions ||
      draft.allow_indexing !== saved.allow_indexing ||
      draft.cli_passthrough !== saved.cli_passthrough
    );
  };

  const isAgentDirty = useMemo(() => {
    if (!draftAgent) return false;
    if (!savedAgent) return true;
    if ((draftAgent.workspace_id ?? null) !== (savedAgent.workspace_id ?? null)) return true;
    if ((draftAgent.mcp_config_path ?? '') !== (savedAgent.mcp_config_path ?? '')) return true;
    if (draftAgent.profiles.length !== savedAgent.profiles.length) return true;
    const savedProfiles = new Map(savedAgent.profiles.map((profile) => [profile.id, profile]));
    for (const profile of draftAgent.profiles) {
      const savedProfile = savedProfiles.get(profile.id);
      if (!savedProfile || isProfileDirty(profile, savedProfile)) return true;
    }
    return false;
  }, [draftAgent, savedAgent]);

  const syncAgentsToStore = (nextAgents: Agent[]) => {
    setSettingsAgents(nextAgents);
    setAgentProfiles(
      nextAgents.flatMap((agent) =>
        agent.profiles.map((profile) => ({
          id: profile.id,
          label: `${profile.agent_display_name} • ${profile.name}`,
          agent_id: agent.id,
        }))
      )
    );
  };

  const upsertAgent = (agent: Agent) => {
    const exists = settingsAgents.some((item: Agent) => item.id === agent.id);
    const nextAgents = exists
      ? settingsAgents.map((item: Agent) => (item.id === agent.id ? agent : item))
      : [...settingsAgents, agent];
    syncAgentsToStore(nextAgents);
  };

  const handleAddProfile = () => {
    const draftId = `draft-${generateUUID()}`;
    setDraftAgent((current) => ({
      ...current,
      profiles: [
        ...current.profiles,
        { ...createDraftProfile(current.id, resolveDisplayName(current.name), currentAgentModelConfig.default_model, permissionSettings), id: draftId },
      ],
    }));
    setNewProfileId(draftId);
  };

  const handleRemoveProfile = (profileId: string) => {
    setDraftAgent((current) => {
      const remaining = current.profiles.filter((profile) => profile.id !== profileId);
      return {
        ...current,
        profiles:
          remaining.length > 0
            ? remaining
            : [createDraftProfile(current.id, resolveDisplayName(current.name), currentAgentModelConfig.default_model, permissionSettings)],
      };
    });
    if (newProfileId === profileId) {
      setNewProfileId(null);
    }
  };

  const handleProfileChange = (profileId: string, patch: Partial<DraftProfile>) => {
    setDraftAgent((current) => ({
      ...current,
      profiles: current.profiles.map((profile) =>
        profile.id === profileId ? { ...profile, ...patch } : profile
      ),
    }));
  };

  const handleProfileMcpChange = (
    profileId: string,
    patch: Partial<NonNullable<DraftProfile['mcp_config']>>
  ) => {
    setDraftAgent((current) => ({
      ...current,
      profiles: current.profiles.map((profile) =>
        profile.id === profileId
          ? { ...profile, mcp_config: { ...(profile.mcp_config ?? defaultMcpConfig), ...patch } }
          : profile
      ),
    }));
  };

  const parseProfileMcpServers = (raw: string) => {
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

  useEffect(() => {
    if (!newProfileId) return;
    const target = document.getElementById(`profile-card-${newProfileId}`);
    if (!target) return;
    target.scrollIntoView({ behavior: 'smooth', block: 'start' });
    const timeout = setTimeout(() => setNewProfileId(null), 1200);
    return () => clearTimeout(timeout);
  }, [newProfileId]);

  const handleSave = async () => {
    if (draftAgent.profiles.some((profile) => !profile.name.trim())) {
      onToastError(new Error('Profile name is required.'));
      return;
    }
    if (draftAgent.profiles.some((profile) => !profile.model.trim())) {
      onToastError(new Error('Model is required for all profiles.'));
      return;
    }
    if (hasInvalidMcpConfig) {
      onToastError(new Error('Fix invalid MCP JSON before saving.'));
      return;
    }
    setSaveStatus('loading');
    try {
      if (!savedAgent) {
        let created = await createAgentAction({
          name: draftAgent.name,
          workspace_id: draftAgent.workspace_id,
          profiles: draftAgent.profiles.map((profile) => ({
            name: profile.name,
            model: profile.model,
            auto_approve: profile.auto_approve,
            dangerously_skip_permissions: profile.dangerously_skip_permissions,
            allow_indexing: profile.allow_indexing ?? false,
            cli_passthrough: profile.cli_passthrough ?? false,
          })),
        });
        if (created.profiles.length === draftAgent.profiles.length) {
          for (let index = 0; index < draftAgent.profiles.length; index += 1) {
            const draftProfile = draftAgent.profiles[index];
            const createdProfile = created.profiles[index];
            if (
              draftProfile.mcp_config &&
              draftProfile.mcp_config.dirty &&
              draftProfile.mcp_config.servers.trim()
            ) {
              try {
                const servers = parseProfileMcpServers(draftProfile.mcp_config.servers);
                await updateAgentProfileMcpConfigAction(createdProfile.id, {
                  enabled: draftProfile.mcp_config.enabled,
                  mcpServers: servers,
                });
              } catch (error) {
                onToastError(error);
              }
            }
          }
        } else {
          for (const draftProfile of draftAgent.profiles) {
            if (
              !draftProfile.mcp_config ||
              !draftProfile.mcp_config.dirty ||
              !draftProfile.mcp_config.servers.trim()
            ) {
              continue;
            }
            const createdProfile = created.profiles.find(
              (profile) => profile.name === draftProfile.name
            );
            if (!createdProfile) continue;
            try {
              const servers = parseProfileMcpServers(draftProfile.mcp_config.servers);
              await updateAgentProfileMcpConfigAction(createdProfile.id, {
                enabled: draftProfile.mcp_config.enabled,
                mcpServers: servers,
              });
            } catch (error) {
              onToastError(error);
            }
          }
        }
        if ((draftAgent.mcp_config_path ?? '') !== (created.mcp_config_path ?? '')) {
          created = await updateAgentAction(created.id, {
            mcp_config_path: draftAgent.mcp_config_path ?? '',
          });
        }
        upsertAgent(created);
        setDraftAgent(ensureProfiles(cloneAgent(created), resolveDisplayName(created.name), currentAgentModelConfig.default_model, permissionSettings));
        router.replace(`/settings/agents/${encodeURIComponent(created.name)}`);
      } else {
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

        const savedProfilesById = new Map(savedAgent.profiles.map((profile) => [profile.id, profile]));
        // In create mode, preserve existing profiles; otherwise start fresh
        const nextProfiles: AgentProfile[] = isCreateMode ? [...savedAgent.profiles] : [];
        for (const profile of draftAgent.profiles) {
          const savedProfile = savedProfilesById.get(profile.id);
          if (!savedProfile) {
            const createdProfile = await createAgentProfileAction(savedAgent.id, {
              name: profile.name,
              model: profile.model,
              auto_approve: profile.auto_approve,
              dangerously_skip_permissions: profile.dangerously_skip_permissions,
              allow_indexing: profile.allow_indexing ?? false,
              cli_passthrough: profile.cli_passthrough ?? false,
            });
            if (profile.mcp_config && profile.mcp_config.dirty && profile.mcp_config.servers.trim()) {
              try {
                const servers = parseProfileMcpServers(profile.mcp_config.servers);
                await updateAgentProfileMcpConfigAction(createdProfile.id, {
                  enabled: profile.mcp_config.enabled,
                  mcpServers: servers,
                });
              } catch (error) {
                onToastError(error);
              }
            }
            nextProfiles.push(createdProfile);
            continue;
          }
          if (isProfileDirty(profile, savedProfile)) {
            const updatedProfile = await updateAgentProfileAction(profile.id, {
              name: profile.name,
              model: profile.model,
              auto_approve: profile.auto_approve,
              dangerously_skip_permissions: profile.dangerously_skip_permissions,
              allow_indexing: profile.allow_indexing ?? false,
            });
            nextProfiles.push(updatedProfile);
            continue;
          }
          nextProfiles.push(savedProfile);
        }
        // In create mode, don't delete existing profiles - we're only adding new ones
        if (!isCreateMode) {
          for (const savedProfile of savedAgent.profiles) {
            const stillExists = draftAgent.profiles.some((profile) => profile.id === savedProfile.id);
            if (!stillExists) {
              await deleteAgentProfileAction(savedProfile.id);
            }
          }
        }

        const nextAgent = {
          ...savedAgent,
          workspace_id: draftAgent.workspace_id ?? null,
          mcp_config_path: draftAgent.mcp_config_path ?? '',
          profiles: nextProfiles,
        };
        upsertAgent(nextAgent);
        setDraftAgent(ensureProfiles(cloneAgent(nextAgent), resolveDisplayName(nextAgent.name), currentAgentModelConfig.default_model, permissionSettings));
        // In create mode, redirect to manage mode after successful save
        if (isCreateMode) {
          router.replace(`/settings/agents/${encodeURIComponent(savedAgent.name)}`);
        }
      }
      setSaveStatus('success');
    } catch (error) {
      setSaveStatus('error');
      onToastError(error);
    }
  };

  return (
    <div className="space-y-8">
      <div className="flex items-start justify-between gap-6">
        <div>
          <div className="flex flex-wrap items-center gap-2">
            <h2 className="text-2xl font-bold">
              {draftAgent.profiles[0]?.agent_display_name ?? draftAgent.name}
            </h2>
            <span className="text-xs text-muted-foreground border border-muted-foreground/30 rounded-full px-2 py-1">
              {discoveryAgent?.matched_path ?? 'Installation not detected'}
            </span>
          </div>
          <p className="text-sm text-muted-foreground mt-1">
            {isCreateMode ? 'Create a new profile for this agent.' : 'Configure profiles and defaults for this agent.'}
          </p>
        </div>
      </div>

      <Separator />

      <Card>
        <CardHeader className="flex flex-row items-center justify-between">
          <div className="flex items-center gap-2">
            <CardTitle>
              {isCreateMode
                ? `Create ${draftAgent.profiles[0]?.agent_display_name ?? draftAgent.name} Profile`
                : `${draftAgent.profiles[0]?.agent_display_name ?? draftAgent.name} Profiles`}
            </CardTitle>
            {isAgentDirty && <UnsavedChangesBadge />}
          </div>
          <Button size="sm" variant="outline" onClick={handleAddProfile} className="cursor-pointer">
            <IconPlus className="h-4 w-4 mr-2" />
            Add profile
          </Button>
        </CardHeader>
        <CardContent className="space-y-4">
          {draftAgent.profiles.map((profile) => {
            const isNew = profile.id === newProfileId;
            return (
            <Card
              key={profile.id}
              id={`profile-card-${profile.id}`}
              className={isNew ? 'border-amber-400/70 shadow-sm' : 'border-muted'}
            >
              <CardContent className="pt-6 space-y-4">
                <ProfileFormFields
                  profile={{
                    name: profile.name,
                    model: profile.model,
                    auto_approve: profile.auto_approve,
                    dangerously_skip_permissions: profile.dangerously_skip_permissions,
                    allow_indexing: profile.allow_indexing,
                    cli_passthrough: profile.cli_passthrough,
                  }}
                  onChange={(patch) => handleProfileChange(profile.id, patch)}
                  modelConfig={currentAgentModelConfig}
                  permissionSettings={permissionSettings}
                  passthroughConfig={passthroughConfig}
                  agentName={draftAgent.name}
                  onRemove={() => handleRemoveProfile(profile.id)}
                  canRemove={draftAgent.profiles.length > 1}
                />

                <ProfileMcpConfigCard
                  profileId={profile.id}
                  supportsMcp={draftAgent.supports_mcp}
                  draftState={profile.id.startsWith('draft-') ? profile.mcp_config : undefined}
                  onDraftStateChange={(patch) => handleProfileMcpChange(profile.id, patch)}
                  onToastError={onToastError}
                />
              </CardContent>
            </Card>
          );
          })}
        </CardContent>
        <div className="flex justify-end px-6 pb-6">
          <UnsavedSaveButton
            isDirty={isAgentDirty}
            isLoading={saveStatus === 'loading'}
            status={saveStatus}
            onClick={handleSave}
            disabled={hasInvalidMcpConfig}
          />
        </div>
      </Card>

    </div>
  );
}

export default function AgentSetupPage() {
  const { toast } = useToast();
  const params = useParams();
  const searchParams = useSearchParams();
  const isCreateMode = searchParams.get('mode') === 'create';
  const agentKey = Array.isArray(params.agentId) ? params.agentId[0] : params.agentId;
  const decodedKey = decodeURIComponent(agentKey ?? '');
  const discoveryAgents = useAppStore((state) => state.agentDiscovery.items);
  const savedAgents = useAppStore((state) => state.settingsAgents.items);
  const availableAgents = useAvailableAgents().items;

  const discoveryAgent = useMemo(() => {
    return discoveryAgents.find((agent: AgentDiscovery) => agent.name === decodedKey);
  }, [decodedKey, discoveryAgents]);

  const savedAgent = useMemo(() => {
    return savedAgents.find((agent: Agent) => agent.id === decodedKey || agent.name === decodedKey) ?? null;
  }, [decodedKey, savedAgents]);

  const initialAgent = useMemo(() => {
    if (!decodedKey) return null;
    const resolveAvailableAgent = (name: string) =>
      availableAgents.find((item: AvailableAgent) => item.name === name);
    const resolveDisplayName = (name: string) =>
      resolveAvailableAgent(name)?.display_name ?? '';
    const resolveDefaultModel = (name: string) =>
      resolveAvailableAgent(name)?.model_config?.default_model ?? '';
    const resolvePermissionSettings = (name: string) =>
      resolveAvailableAgent(name)?.permission_settings;
    if (savedAgent) {
      // In create mode, start with a blank profile instead of loading existing profiles
      if (isCreateMode) {
        const draft: DraftAgent = {
          ...savedAgent,
          workspace_id: savedAgent.workspace_id ?? null,
          mcp_config_path: savedAgent.mcp_config_path ?? '',
          profiles: [],
          isNew: false,
        };
        return ensureProfiles(draft, resolveDisplayName(savedAgent.name), resolveDefaultModel(savedAgent.name), resolvePermissionSettings(savedAgent.name));
      }
      return ensureProfiles(cloneAgent(savedAgent), resolveDisplayName(savedAgent.name), resolveDefaultModel(savedAgent.name), resolvePermissionSettings(savedAgent.name));
    }
    if (discoveryAgent) {
      const draft: DraftAgent = {
        id: `draft-${generateUUID()}`,
        name: discoveryAgent.name,
        workspace_id: null,
        supports_mcp: discoveryAgent.supports_mcp,
        mcp_config_path: discoveryAgent.mcp_config_path ?? '',
        profiles: [],
        created_at: new Date().toISOString(),
        updated_at: new Date().toISOString(),
        isNew: true,
      };
      return ensureProfiles(draft, resolveDisplayName(draft.name), resolveDefaultModel(draft.name), resolvePermissionSettings(draft.name));
    }
    return null;
  }, [decodedKey, discoveryAgent, savedAgent, availableAgents, isCreateMode]);

  if (!initialAgent && discoveryAgents.length > 0) {
    return (
      <Card>
        <CardContent className="py-12 text-center">
          <p className="text-sm text-muted-foreground">Agent not found.</p>
          <Button className="mt-4" asChild>
            <Link href="/settings/agents">Back to Agents</Link>
          </Button>
        </CardContent>
      </Card>
    );
  }

  if (!initialAgent) {
    return null;
  }

  const handleToastError = (error: unknown) => {
    toast({
      title: 'Failed to save agent',
      description: error instanceof Error ? error.message : 'Request failed',
      variant: 'error',
    });
  };

  return (
    <AgentSetupForm
      key={isCreateMode ? `create-${initialAgent.id}` : initialAgent.id}
      initialAgent={initialAgent}
      savedAgent={savedAgent}
      discoveryAgent={discoveryAgent}
      onToastError={handleToastError}
      isCreateMode={isCreateMode}
    />
  );
}
