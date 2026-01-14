'use client';

import { useMemo, useState } from 'react';
import Link from 'next/link';
import { useParams, useRouter } from 'next/navigation';
import { IconPlus } from '@tabler/icons-react';
import { Button } from '@kandev/ui/button';
import { Card, CardContent, CardHeader, CardTitle } from '@kandev/ui/card';
import { Input } from '@kandev/ui/input';
import { Label } from '@kandev/ui/label';
import { Separator } from '@kandev/ui/separator';
import { Switch } from '@kandev/ui/switch';
import { Textarea } from '@kandev/ui/textarea';
import { useToast } from '@/components/toast-provider';
import { UnsavedChangesBadge, UnsavedSaveButton } from '@/components/settings/unsaved-indicator';
import {
  createAgentAction,
  createAgentProfileAction,
  deleteAgentProfileAction,
  updateAgentAction,
  updateAgentProfileAction,
} from '@/app/actions/agents';
import type { Agent, AgentDiscovery, AgentProfile } from '@/lib/types/http';
import { generateUUID } from '@/lib/utils';
import { useAppStore } from '@/components/state-provider';

type DraftAgent = Agent & { isNew?: boolean };
type DraftProfile = AgentProfile & { isNew?: boolean };

type AgentSetupFormProps = {
  initialAgent: DraftAgent;
  savedAgent: Agent | null;
  discoveryAgent: AgentDiscovery | undefined;
  onToastError: (error: unknown) => void;
};

const AGENT_LABELS: Record<string, string> = {
  claude: 'Claude',
  gemini: 'Gemini',
  codex: 'Codex',
  opencode: 'OpenCode',
  copilot: 'Copilot',
};

const createDraftProfile = (agentId: string): DraftProfile => ({
  id: `draft-${generateUUID()}`,
  agent_id: agentId,
  name: '',
  model: '',
  auto_approve: false,
  dangerously_skip_permissions: false,
  plan: '',
  created_at: new Date().toISOString(),
  updated_at: new Date().toISOString(),
  isNew: true,
});

const cloneAgent = (agent: Agent): DraftAgent => ({
  ...agent,
  workspace_id: agent.workspace_id ?? null,
  mcp_config_path: agent.mcp_config_path ?? '',
  profiles: agent.profiles.map((profile) => ({ ...profile })),
});

const ensureProfiles = (agent: DraftAgent): DraftAgent => {
  if (agent.profiles.length > 0) {
    return agent;
  }
  return {
    ...agent,
    profiles: [createDraftProfile(agent.id)],
  };
};

function AgentSetupForm({
  initialAgent,
  savedAgent,
  discoveryAgent,
  onToastError,
}: AgentSetupFormProps) {
  const router = useRouter();
  const settingsAgents = useAppStore((state) => state.settingsAgents.items);
  const setSettingsAgents = useAppStore((state) => state.setSettingsAgents);
  const setAgentProfiles = useAppStore((state) => state.setAgentProfiles);
  const [draftAgent, setDraftAgent] = useState<DraftAgent>(initialAgent);
  const [saveStatus, setSaveStatus] = useState<'idle' | 'loading' | 'success' | 'error'>('idle');

  const isProfileDirty = (draft: DraftProfile, saved?: AgentProfile) => {
    if (!saved) return true;
    return (
      draft.name !== saved.name ||
      draft.model !== saved.model ||
      draft.auto_approve !== saved.auto_approve ||
      draft.dangerously_skip_permissions !== saved.dangerously_skip_permissions ||
      draft.plan !== saved.plan
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
          label: `${agent.name} • ${profile.name}`,
          agent_id: agent.id,
        }))
      )
    );
  };

  const upsertAgent = (agent: Agent) => {
    const exists = settingsAgents.some((item) => item.id === agent.id);
    const nextAgents = exists
      ? settingsAgents.map((item) => (item.id === agent.id ? agent : item))
      : [...settingsAgents, agent];
    syncAgentsToStore(nextAgents);
  };

  const handleAddProfile = () => {
    setDraftAgent((current) => ({
      ...current,
      profiles: [...current.profiles, createDraftProfile(current.id)],
    }));
  };

  const handleRemoveProfile = (profileId: string) => {
    setDraftAgent((current) => {
      const remaining = current.profiles.filter((profile) => profile.id !== profileId);
      return {
        ...current,
        profiles: remaining.length > 0 ? remaining : [createDraftProfile(current.id)],
      };
    });
  };

  const handleProfileChange = (profileId: string, patch: Partial<DraftProfile>) => {
    setDraftAgent((current) => ({
      ...current,
      profiles: current.profiles.map((profile) =>
        profile.id === profileId ? { ...profile, ...patch } : profile
      ),
    }));
  };

  const handleAgentFieldChange = (patch: Partial<DraftAgent>) => {
    setDraftAgent((current) => ({ ...current, ...patch }));
  };

  const handleSave = async () => {
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
            plan: profile.plan,
          })),
        });
        if ((draftAgent.mcp_config_path ?? '') !== (created.mcp_config_path ?? '')) {
          created = await updateAgentAction(created.id, {
            mcp_config_path: draftAgent.mcp_config_path ?? '',
          });
        }
        upsertAgent(created);
        setDraftAgent(ensureProfiles(cloneAgent(created)));
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
        const nextProfiles: AgentProfile[] = [];
        for (const profile of draftAgent.profiles) {
          const savedProfile = savedProfilesById.get(profile.id);
          if (!savedProfile) {
            const createdProfile = await createAgentProfileAction(savedAgent.id, {
              name: profile.name,
              model: profile.model,
              auto_approve: profile.auto_approve,
              dangerously_skip_permissions: profile.dangerously_skip_permissions,
              plan: profile.plan,
            });
            nextProfiles.push(createdProfile);
            continue;
          }
          if (isProfileDirty(profile, savedProfile)) {
            const updatedProfile = await updateAgentProfileAction(profile.id, {
              name: profile.name,
              model: profile.model,
              auto_approve: profile.auto_approve,
              dangerously_skip_permissions: profile.dangerously_skip_permissions,
              plan: profile.plan,
            });
            nextProfiles.push(updatedProfile);
            continue;
          }
          nextProfiles.push(savedProfile);
        }
        for (const savedProfile of savedAgent.profiles) {
          const stillExists = draftAgent.profiles.some((profile) => profile.id === savedProfile.id);
          if (!stillExists) {
            await deleteAgentProfileAction(savedProfile.id);
          }
        }

        const nextAgent = {
          ...savedAgent,
          workspace_id: draftAgent.workspace_id ?? null,
          mcp_config_path: draftAgent.mcp_config_path ?? '',
          profiles: nextProfiles,
        };
        upsertAgent(nextAgent);
        setDraftAgent(ensureProfiles(cloneAgent(nextAgent)));
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
              {AGENT_LABELS[draftAgent.name] ?? draftAgent.name}
            </h2>
            <span className="text-xs text-muted-foreground border border-muted-foreground/30 rounded-full px-2 py-1">
              {discoveryAgent?.matched_path ?? 'Installation not detected'}
            </span>
          </div>
          <p className="text-sm text-muted-foreground mt-1">
            Configure profiles and defaults for this agent.
          </p>
        </div>
      </div>

      <Separator />

      <Card>
        <CardHeader className="flex flex-row items-center justify-between">
          <div className="flex items-center gap-2">
            <CardTitle>{AGENT_LABELS[draftAgent.name] ?? draftAgent.name} Profiles</CardTitle>
            {isAgentDirty && <UnsavedChangesBadge />}
          </div>
          <Button size="sm" variant="outline" onClick={handleAddProfile}>
            <IconPlus className="h-4 w-4 mr-2" />
            Add profile
          </Button>
        </CardHeader>
        <CardContent className="space-y-4">
          {draftAgent.supports_mcp && (
            <div className="space-y-2">
              <Label htmlFor="mcp-config-path">MCP Config Path</Label>
              <Input
                id="mcp-config-path"
                value={draftAgent.mcp_config_path ?? ''}
                onChange={(event) => handleAgentFieldChange({ mcp_config_path: event.target.value })}
                placeholder="~/.copilot/mcp-config.json"
              />
            </div>
          )}
          {draftAgent.profiles.map((profile) => (
            <Card key={profile.id} className="border-muted">
              <CardContent className="pt-6 space-y-4">
                <div className="flex items-center justify-between gap-4">
                  <div className="flex-1 space-y-2">
                    <Label>Profile name</Label>
                    <Input
                      value={profile.name}
                      onChange={(event) =>
                        handleProfileChange(profile.id, { name: event.target.value })
                      }
                      placeholder="Default profile"
                    />
                  </div>
                  <Button size="sm" variant="ghost" onClick={() => handleRemoveProfile(profile.id)}>
                    Remove
                  </Button>
                </div>

                <div className="space-y-2">
                  <Label>Model</Label>
                  <Input
                    value={profile.model}
                    onChange={(event) =>
                      handleProfileChange(profile.id, { model: event.target.value })
                    }
                    placeholder="model-id"
                  />
                </div>

                <div className="space-y-2">
                  <Label>Append Prompt</Label>
                  <Textarea
                    value={profile.plan}
                    onChange={(event) =>
                      handleProfileChange(profile.id, { plan: event.target.value })
                    }
                    placeholder="Extra text appended to the agent prompt"
                  />
                </div>

                <div className="grid gap-4 md:grid-cols-2">
                  <div className="flex items-center justify-between rounded-md border p-3">
                    <div className="space-y-1">
                      <Label>Auto-approve</Label>
                      <p className="text-xs text-muted-foreground">
                        Automatically approve tool calls.
                      </p>
                    </div>
                    <Switch
                      checked={profile.auto_approve}
                      onCheckedChange={(checked) =>
                        handleProfileChange(profile.id, { auto_approve: checked })
                      }
                    />
                  </div>

                  <div className="flex items-center justify-between rounded-md border p-3">
                    <div className="space-y-1">
                      <Label>Skip permissions</Label>
                      <p className="text-xs text-muted-foreground">
                        Skip permission checks when running tools.
                      </p>
                    </div>
                    <Switch
                      checked={profile.dangerously_skip_permissions}
                      onCheckedChange={(checked) =>
                        handleProfileChange(profile.id, { dangerously_skip_permissions: checked })
                      }
                    />
                  </div>
                </div>
              </CardContent>
            </Card>
          ))}
        </CardContent>
        <div className="flex justify-end px-6 pb-6">
          <UnsavedSaveButton
            isDirty={isAgentDirty}
            isLoading={saveStatus === 'loading'}
            status={saveStatus}
            onClick={handleSave}
          />
        </div>
      </Card>
    </div>
  );
}

export default function AgentSetupPage() {
  const { toast } = useToast();
  const params = useParams();
  const agentKey = Array.isArray(params.agentId) ? params.agentId[0] : params.agentId;
  const decodedKey = decodeURIComponent(agentKey ?? '');
  const discoveryAgents = useAppStore((state) => state.agentDiscovery.items);
  const savedAgents = useAppStore((state) => state.settingsAgents.items);

  const discoveryAgent = useMemo(() => {
    return discoveryAgents.find((agent) => agent.name === decodedKey);
  }, [decodedKey, discoveryAgents]);

  const savedAgent = useMemo(() => {
    return savedAgents.find((agent) => agent.id === decodedKey || agent.name === decodedKey) ?? null;
  }, [decodedKey, savedAgents]);

  const initialAgent = useMemo(() => {
    if (!decodedKey) return null;
    if (savedAgent) {
      return ensureProfiles(cloneAgent(savedAgent));
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
      return ensureProfiles(draft);
    }
    return null;
  }, [decodedKey, discoveryAgent, savedAgent]);

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
      key={initialAgent.id}
      initialAgent={initialAgent}
      savedAgent={savedAgent}
      discoveryAgent={discoveryAgent}
      onToastError={handleToastError}
    />
  );
}
