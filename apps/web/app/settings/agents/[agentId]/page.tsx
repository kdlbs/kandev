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
import type { Agent, AgentDiscovery, AvailableAgent, PermissionSetting, ModelConfig, PassthroughConfig } from '@/lib/types/http';
import { buildDefaultPermissions } from '@/lib/agent-permissions';
import { generateUUID } from '@/lib/utils';
import { useAppStore } from '@/components/state-provider';
import { useAvailableAgents } from '@/hooks/domains/settings/use-available-agents';
import { ProfileMcpConfigCard } from './profile-mcp-config-card';
import { saveNewAgent, saveExistingAgent, isProfileDirty } from './agent-save-helpers';
import type { DraftProfile, DraftAgent } from './agent-save-helpers';

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
  ...buildDefaultPermissions(permissionSettings ?? {}),
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
  if (agent.profiles.length > 0) return agent;
  return {
    ...agent,
    profiles: [createDraftProfile(agent.id, agentDisplayName, defaultModel, permissionSettings)],
  };
};

type ProfileCardItemProps = {
  profile: DraftProfile;
  isNew: boolean;
  draftAgent: DraftAgent;
  currentAgentModelConfig: ModelConfig;
  permissionSettings: Record<string, PermissionSetting>;
  passthroughConfig: PassthroughConfig | null;
  onProfileChange: (profileId: string, patch: Partial<DraftProfile>) => void;
  onProfileMcpChange: (profileId: string, patch: Partial<NonNullable<DraftProfile['mcp_config']>>) => void;
  onRemoveProfile: (profileId: string) => void;
  onToastError: (error: unknown) => void;
};

function ProfileCardItem({
  profile, isNew, draftAgent, currentAgentModelConfig, permissionSettings,
  passthroughConfig, onProfileChange, onProfileMcpChange, onRemoveProfile, onToastError,
}: ProfileCardItemProps) {
  return (
    <Card
      id={`profile-card-${profile.id}`}
      className={isNew ? 'border-amber-400/70 shadow-sm' : 'border-muted'}
    >
      <CardContent className="pt-6 space-y-4">
        <ProfileFormFields
          profile={{
            name: profile.name, model: profile.model, auto_approve: profile.auto_approve,
            dangerously_skip_permissions: profile.dangerously_skip_permissions,
            allow_indexing: profile.allow_indexing ?? false, cli_passthrough: profile.cli_passthrough,
          }}
          onChange={(patch) => onProfileChange(profile.id, patch)}
          modelConfig={currentAgentModelConfig}
          permissionSettings={permissionSettings}
          passthroughConfig={passthroughConfig}
          agentName={draftAgent.name}
          onRemove={() => onRemoveProfile(profile.id)}
          canRemove={draftAgent.profiles.length > 1}
        />
        <ProfileMcpConfigCard
          profileId={profile.id}
          supportsMcp={draftAgent.supports_mcp}
          draftState={profile.id.startsWith('draft-') ? profile.mcp_config : undefined}
          onDraftStateChange={(patch) => onProfileMcpChange(profile.id, patch)}
          onToastError={onToastError}
        />
      </CardContent>
    </Card>
  );
}

function useAgentFormState(initialAgent: DraftAgent, savedAgent: Agent | null, availableAgents: AvailableAgent[]) {
  const [draftAgent, setDraftAgent] = useState<DraftAgent>(initialAgent);
  const [saveStatus, setSaveStatus] = useState<'idle' | 'loading' | 'success' | 'error'>('idle');

  const resolveDisplayName = (name: string) =>
    availableAgents.find((item: AvailableAgent) => item.name === name)?.display_name ?? '';

  const currentAvailableAgent = useMemo(() => {
    return availableAgents.find((item: AvailableAgent) => item.name === draftAgent.name) ?? null;
  }, [availableAgents, draftAgent.name]);

  const currentAgentModelConfig = useMemo(() => {
    return currentAvailableAgent?.model_config ?? { default_model: '', available_models: [], supports_dynamic_models: false };
  }, [currentAvailableAgent]);

  const permissionSettings = useMemo(() => currentAvailableAgent?.permission_settings ?? {}, [currentAvailableAgent]);
  const passthroughConfig = useMemo(() => currentAvailableAgent?.passthrough_config ?? null, [currentAvailableAgent]);

  const hasInvalidMcpConfig = useMemo(() => {
    return draftAgent.profiles.some((profile) => Boolean(profile.mcp_config?.error));
  }, [draftAgent.profiles]);

  const isAgentDirty = useMemo(() => {
    if (!draftAgent || !savedAgent) return !savedAgent;
    if ((draftAgent.workspace_id ?? null) !== (savedAgent.workspace_id ?? null)) return true;
    if ((draftAgent.mcp_config_path ?? '') !== (savedAgent.mcp_config_path ?? '')) return true;
    if (draftAgent.profiles.length !== savedAgent.profiles.length) return true;
    const savedProfiles = new Map(savedAgent.profiles.map((p) => [p.id, p]));
    for (const profile of draftAgent.profiles) {
      if (!savedProfiles.has(profile.id) || isProfileDirty(profile, savedProfiles.get(profile.id))) return true;
    }
    return false;
  }, [draftAgent, savedAgent]);

  return {
    draftAgent, setDraftAgent, saveStatus, setSaveStatus,
    resolveDisplayName, currentAgentModelConfig, permissionSettings, passthroughConfig,
    hasInvalidMcpConfig, isAgentDirty,
  };
}

type AgentHeaderProps = {
  displayName: string;
  matchedPath: string | null | undefined;
  isCreateMode: boolean;
};

function AgentHeader({ displayName, matchedPath, isCreateMode }: AgentHeaderProps) {
  return (
    <div className="flex items-start justify-between gap-6">
      <div>
        <div className="flex flex-wrap items-center gap-2">
          <h2 className="text-2xl font-bold">{displayName}</h2>
          <span className="text-xs text-muted-foreground border border-muted-foreground/30 rounded-full px-2 py-1">
            {matchedPath ?? 'Installation not detected'}
          </span>
        </div>
        <p className="text-sm text-muted-foreground mt-1">
          {isCreateMode ? 'Create a new profile for this agent.' : 'Configure profiles and defaults for this agent.'}
        </p>
      </div>
    </div>
  );
}

function useAgentStoreSync() {
  const settingsAgents = useAppStore((state) => state.settingsAgents.items);
  const setSettingsAgents = useAppStore((state) => state.setSettingsAgents);
  const setAgentProfiles = useAppStore((state) => state.setAgentProfiles);

  const syncAgentsToStore = (nextAgents: Agent[]) => {
    setSettingsAgents(nextAgents);
    setAgentProfiles(
      nextAgents.flatMap((agent) =>
        agent.profiles.map((profile) => ({
          id: profile.id, label: `${profile.agent_display_name} • ${profile.name}`,
          agent_id: agent.id, agent_name: agent.name, cli_passthrough: profile.cli_passthrough,
        }))
      )
    );
  };

  const upsertAgent = (agent: Agent) => {
    const exists = settingsAgents.some((item: Agent) => item.id === agent.id);
    syncAgentsToStore(exists ? settingsAgents.map((item: Agent) => (item.id === agent.id ? agent : item)) : [...settingsAgents, agent]);
  };

  return { upsertAgent };
}

function useProfileHandlers(
  setDraftAgent: (fn: (current: DraftAgent) => DraftAgent) => void,
  resolveDisplayName: (name: string) => string,
  defaultModel: string,
  permissionSettings: Record<string, PermissionSetting>,
) {
  const [newProfileId, setNewProfileId] = useState<string | null>(null);

  const handleAddProfile = () => {
    const draftId = `draft-${generateUUID()}`;
    setDraftAgent((current) => ({
      ...current,
      profiles: [...current.profiles, { ...createDraftProfile(current.id, resolveDisplayName(current.name), defaultModel, permissionSettings), id: draftId }],
    }));
    setNewProfileId(draftId);
  };

  const handleRemoveProfile = (profileId: string) => {
    setDraftAgent((current) => {
      const remaining = current.profiles.filter((p) => p.id !== profileId);
      return { ...current, profiles: remaining.length > 0 ? remaining : [createDraftProfile(current.id, resolveDisplayName(current.name), defaultModel, permissionSettings)] };
    });
    if (newProfileId === profileId) setNewProfileId(null);
  };

  const handleProfileChange = (profileId: string, patch: Partial<DraftProfile>) => {
    setDraftAgent((current) => ({ ...current, profiles: current.profiles.map((p) => p.id === profileId ? { ...p, ...patch } : p) }));
  };

  const handleProfileMcpChange = (profileId: string, patch: Partial<NonNullable<DraftProfile['mcp_config']>>) => {
    setDraftAgent((current) => ({
      ...current,
      profiles: current.profiles.map((p) => p.id === profileId ? { ...p, mcp_config: { ...(p.mcp_config ?? defaultMcpConfig), ...patch } } : p),
    }));
  };

  useEffect(() => {
    if (!newProfileId) return;
    const target = document.getElementById(`profile-card-${newProfileId}`);
    if (!target) return;
    target.scrollIntoView({ behavior: 'smooth', block: 'start' });
    const timeout = setTimeout(() => setNewProfileId(null), 1200);
    return () => clearTimeout(timeout);
  }, [newProfileId]);

  return { newProfileId, handleAddProfile, handleRemoveProfile, handleProfileChange, handleProfileMcpChange };
}

function AgentSetupForm({ initialAgent, savedAgent, discoveryAgent, onToastError, isCreateMode = false }: AgentSetupFormProps) {
  const router = useRouter();
  const availableAgents = useAvailableAgents().items;
  const { upsertAgent } = useAgentStoreSync();

  const {
    draftAgent, setDraftAgent, saveStatus, setSaveStatus,
    resolveDisplayName, currentAgentModelConfig, permissionSettings, passthroughConfig,
    hasInvalidMcpConfig, isAgentDirty,
  } = useAgentFormState(initialAgent, savedAgent, availableAgents);

  const { newProfileId, handleAddProfile, handleRemoveProfile, handleProfileChange, handleProfileMcpChange } =
    useProfileHandlers(setDraftAgent, resolveDisplayName, currentAgentModelConfig.default_model, permissionSettings);

  const handleSave = async () => {
    if (draftAgent.profiles.some((p) => !p.name.trim())) { onToastError(new Error('Profile name is required.')); return; }
    if (draftAgent.profiles.some((p) => !p.model.trim())) { onToastError(new Error('Model is required for all profiles.')); return; }
    if (hasInvalidMcpConfig) { onToastError(new Error('Fix invalid MCP JSON before saving.')); return; }
    setSaveStatus('loading');
    const callbacks = { onToastError, currentAgentModelConfig, permissionSettings, resolveDisplayName, upsertAgent, setDraftAgent, ensureProfiles, cloneAgent, replaceRoute: (path: string) => router.replace(path) };
    try {
      if (!savedAgent) { await saveNewAgent(draftAgent, callbacks); }
      else { await saveExistingAgent(draftAgent, savedAgent, isCreateMode, callbacks); }
      setSaveStatus('success');
    } catch (error) { setSaveStatus('error'); onToastError(error); }
  };

  const displayName = draftAgent.profiles[0]?.agent_display_name ?? draftAgent.name;

  return (
    <div className="space-y-8">
      <AgentHeader displayName={displayName} matchedPath={discoveryAgent?.matched_path} isCreateMode={isCreateMode} />
      <Separator />
      <Card>
        <CardHeader className="flex flex-row items-center justify-between">
          <div className="flex items-center gap-2">
            <CardTitle>{isCreateMode ? `Create ${displayName} Profile` : `${displayName} Profiles`}</CardTitle>
            {isAgentDirty && <UnsavedChangesBadge />}
          </div>
          <Button size="sm" variant="outline" onClick={handleAddProfile} className="cursor-pointer">
            <IconPlus className="h-4 w-4 mr-2" />
            Add profile
          </Button>
        </CardHeader>
        <CardContent className="space-y-4">
          {draftAgent.profiles.map((profile) => (
            <ProfileCardItem
              key={profile.id} profile={profile} isNew={profile.id === newProfileId}
              draftAgent={draftAgent} currentAgentModelConfig={currentAgentModelConfig}
              permissionSettings={permissionSettings} passthroughConfig={passthroughConfig}
              onProfileChange={handleProfileChange} onProfileMcpChange={handleProfileMcpChange}
              onRemoveProfile={handleRemoveProfile} onToastError={onToastError}
            />
          ))}
        </CardContent>
        <div className="flex justify-end px-6 pb-6">
          <UnsavedSaveButton isDirty={isAgentDirty} isLoading={saveStatus === 'loading'} status={saveStatus} onClick={handleSave} disabled={hasInvalidMcpConfig} />
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

  const discoveryAgent = useMemo(() => discoveryAgents.find((a: AgentDiscovery) => a.name === decodedKey), [decodedKey, discoveryAgents]);
  const savedAgent = useMemo(() => savedAgents.find((a: Agent) => a.id === decodedKey || a.name === decodedKey) ?? null, [decodedKey, savedAgents]);

  const initialAgent = useMemo(() => {
    if (!decodedKey) return null;
    const resolve = (name: string) => availableAgents.find((item: AvailableAgent) => item.name === name);
    const dn = (name: string) => resolve(name)?.display_name ?? '';
    const dm = (name: string) => resolve(name)?.model_config?.default_model ?? '';
    const ps = (name: string) => resolve(name)?.permission_settings;
    if (savedAgent) {
      if (isCreateMode) {
        return ensureProfiles({ ...savedAgent, workspace_id: savedAgent.workspace_id ?? null, mcp_config_path: savedAgent.mcp_config_path ?? '', profiles: [], isNew: false }, dn(savedAgent.name), dm(savedAgent.name), ps(savedAgent.name));
      }
      return ensureProfiles(cloneAgent(savedAgent), dn(savedAgent.name), dm(savedAgent.name), ps(savedAgent.name));
    }
    if (discoveryAgent) {
      const draft: DraftAgent = { id: `draft-${generateUUID()}`, name: discoveryAgent.name, workspace_id: null, supports_mcp: discoveryAgent.supports_mcp, mcp_config_path: discoveryAgent.mcp_config_path ?? '', profiles: [], created_at: new Date().toISOString(), updated_at: new Date().toISOString(), isNew: true };
      return ensureProfiles(draft, dn(draft.name), dm(draft.name), ps(draft.name));
    }
    return null;
  }, [decodedKey, discoveryAgent, savedAgent, availableAgents, isCreateMode]);

  if (!initialAgent && discoveryAgents.length > 0) {
    return (
      <Card>
        <CardContent className="py-12 text-center">
          <p className="text-sm text-muted-foreground">Agent not found.</p>
          <Button className="mt-4" asChild><Link href="/settings/agents">Back to Agents</Link></Button>
        </CardContent>
      </Card>
    );
  }

  if (!initialAgent) return null;

  const handleToastError = (error: unknown) => {
    toast({ title: 'Failed to save agent', description: error instanceof Error ? error.message : 'Request failed', variant: 'error' });
  };

  return (
    <AgentSetupForm
      key={isCreateMode ? `create-${initialAgent.id}` : initialAgent.id}
      initialAgent={initialAgent} savedAgent={savedAgent} discoveryAgent={discoveryAgent}
      onToastError={handleToastError} isCreateMode={isCreateMode}
    />
  );
}
