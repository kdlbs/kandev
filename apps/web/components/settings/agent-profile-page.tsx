'use client';

import { useMemo, useState } from 'react';
import Link from 'next/link';
import { useParams } from 'next/navigation';
import { IconTrash } from '@tabler/icons-react';
import { Badge } from '@kandev/ui/badge';
import { Button } from '@kandev/ui/button';
import { Card, CardContent, CardHeader, CardTitle } from '@kandev/ui/card';
import { Separator } from '@kandev/ui/separator';
import { useToast } from '@/components/toast-provider';
import { UnsavedChangesBadge, UnsavedSaveButton } from '@/components/settings/unsaved-indicator';
import { ProfileFormFields } from '@/components/settings/profile-form-fields';
import { deleteAgentProfileAction, updateAgentProfileAction } from '@/app/actions/agents';
import type { Agent, AgentProfile, ModelConfig, PermissionSetting, PassthroughConfig } from '@/lib/types/http';
import { useAppStore } from '@/components/state-provider';
import { ProfileMcpConfigCard } from '@/app/settings/agents/[agentId]/profile-mcp-config-card';
import { CommandPreviewCard } from '@/app/settings/agents/[agentId]/profiles/[profileId]/command-preview-card';
import type { AgentProfileMcpConfig } from '@/lib/types/http';
import { useAgentProfileSettings } from '@/app/settings/agents/[agentId]/profiles/[profileId]/use-agent-profile-settings';

type ProfileEditorProps = {
  agent: Agent;
  profile: AgentProfile;
  modelConfig: ModelConfig;
  permissionSettings: Record<string, PermissionSetting>;
  passthroughConfig: PassthroughConfig | null;
  initialMcpConfig?: AgentProfileMcpConfig | null;
};

function ProfileEditor({ agent, profile, modelConfig, permissionSettings, passthroughConfig, initialMcpConfig }: ProfileEditorProps) {
  const { toast } = useToast();
  const settingsAgents = useAppStore((state) => state.settingsAgents.items);
  const setSettingsAgents = useAppStore((state) => state.setSettingsAgents);
  const setAgentProfiles = useAppStore((state) => state.setAgentProfiles);
  const [draft, setDraft] = useState<AgentProfile>({ ...profile });
  const [savedProfile, setSavedProfile] = useState<AgentProfile>(profile);
  const [saveStatus, setSaveStatus] = useState<'idle' | 'loading' | 'success' | 'error'>('idle');
  const syncAgentsToStore = (nextAgents: Agent[]) => {
    setSettingsAgents(nextAgents);
    setAgentProfiles(
      nextAgents.flatMap((agentItem) =>
        agentItem.profiles.map((agentProfile) => ({
          id: agentProfile.id,
          label: `${agentProfile.agent_display_name} • ${agentProfile.name}`,
          agent_id: agentItem.id,
        }))
      )
    );
  };

  const isDirty = useMemo(() => {
    return (
      draft.name !== savedProfile.name ||
      draft.model !== savedProfile.model ||
      draft.auto_approve !== savedProfile.auto_approve ||
      draft.dangerously_skip_permissions !== savedProfile.dangerously_skip_permissions ||
      draft.allow_indexing !== savedProfile.allow_indexing ||
      draft.cli_passthrough !== savedProfile.cli_passthrough ||
      draft.plan !== savedProfile.plan
    );
  }, [draft, savedProfile]);

  const handleSave = async () => {
    if (!draft.name.trim()) {
      toast({
        title: 'Profile name is required',
        description: 'Please enter a profile name before saving.',
        variant: 'error',
      });
      return;
    }
    if (!draft.model.trim()) {
      toast({
        title: 'Model is required',
        description: 'Please select a model before saving.',
        variant: 'error',
      });
      return;
    }
    setSaveStatus('loading');
    try {
      const updated = await updateAgentProfileAction(draft.id, {
        name: draft.name,
        model: draft.model,
        auto_approve: draft.auto_approve,
        dangerously_skip_permissions: draft.dangerously_skip_permissions,
        allow_indexing: draft.allow_indexing,
        cli_passthrough: draft.cli_passthrough,
        plan: draft.plan,
      });
      setSavedProfile(updated);
      setDraft(updated);
      const nextAgents = settingsAgents.map((agentItem: Agent) =>
        agentItem.id === agent.id
          ? {
            ...agentItem,
            profiles: agentItem.profiles.map((profileItem: AgentProfile) =>
              profileItem.id === updated.id ? updated : profileItem
            ),
          }
          : agentItem
      );
      syncAgentsToStore(nextAgents);
      setSaveStatus('success');
    } catch (error) {
      setSaveStatus('error');
      toast({
        title: 'Failed to save profile',
        description: error instanceof Error ? error.message : 'Request failed',
        variant: 'error',
      });
    }
  };

  const handleDeleteProfile = async () => {
    try {
      await deleteAgentProfileAction(draft.id);
      const nextAgents = settingsAgents.map((agentItem: Agent) =>
        agentItem.id === agent.id
          ? {
            ...agentItem,
            profiles: agentItem.profiles.filter((profileItem: AgentProfile) => profileItem.id !== draft.id),
          }
          : agentItem
      );
      syncAgentsToStore(nextAgents);
      window.location.assign('/settings/agents');
    } catch (error) {
      toast({
        title: 'Failed to delete profile',
        description: error instanceof Error ? error.message : 'Request failed',
        variant: 'error',
      });
    }
  };

  return (
    <div className="space-y-8">
      <div className="flex items-start justify-between">
        <div>
          <h2 className="text-2xl font-bold">{profile.agent_display_name} • {savedProfile.name}</h2>
          <p className="text-sm text-muted-foreground mt-1">
            {profile.agent_display_name} profile settings
          </p>
        </div>
        <div className="flex items-center gap-3">
          {isDirty && <UnsavedChangesBadge />}
          <UnsavedSaveButton
            isDirty={isDirty}
            isLoading={saveStatus === 'loading'}
            status={saveStatus}
            onClick={handleSave}
          />
        </div>
      </div>

      <Separator />

      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            <span>Profile settings</span>
            {agent.supports_mcp && <Badge variant="secondary">MCP</Badge>}
          </CardTitle>
        </CardHeader>
        <CardContent className="space-y-4">
          <ProfileFormFields
            profile={{
              name: draft.name,
              model: draft.model,
              plan: draft.plan,
              auto_approve: draft.auto_approve,
              dangerously_skip_permissions: draft.dangerously_skip_permissions,
              allow_indexing: draft.allow_indexing,
              cli_passthrough: draft.cli_passthrough,
            }}
            onChange={(patch) => setDraft({ ...draft, ...patch })}
            modelConfig={modelConfig}
            permissionSettings={permissionSettings}
            passthroughConfig={passthroughConfig}
            agentName={agent.name}
          />
        </CardContent>
      </Card>

      <CommandPreviewCard
        agentName={agent.name}
        model={draft.model}
        permissionSettings={{
          auto_approve: draft.auto_approve,
          dangerously_skip_permissions: draft.dangerously_skip_permissions,
          allow_indexing: draft.allow_indexing,
        }}
        cliPassthrough={draft.cli_passthrough}
      />

      <ProfileMcpConfigCard
        profileId={profile.id}
        supportsMcp={agent.supports_mcp}
        initialConfig={initialMcpConfig}
        onToastError={(error) =>
          toast({
            title: 'Failed to save MCP config',
            description: error instanceof Error ? error.message : 'Request failed',
            variant: 'error',
          })
        }
      />


      <Card className="border-destructive">
        <CardHeader>
          <CardTitle className="text-destructive">Delete profile</CardTitle>
        </CardHeader>
        <CardContent className="flex items-center justify-between">
          <div>
            <p className="text-sm font-medium">Remove this profile</p>
            <p className="text-xs text-muted-foreground">This action cannot be undone.</p>
          </div>
          <Button variant="destructive" onClick={handleDeleteProfile}>
            <IconTrash className="h-4 w-4 mr-2" />
            Delete
          </Button>
        </CardContent>
      </Card>
    </div>
  );
}

type AgentProfilePageClientProps = {
  initialMcpConfig?: AgentProfileMcpConfig | null;
};

export function AgentProfilePage({ initialMcpConfig }: AgentProfilePageClientProps) {
  const params = useParams();
  const agentParam = Array.isArray(params.agentId) ? params.agentId[0] : params.agentId;
  const profileParam = Array.isArray(params.profileId) ? params.profileId[0] : params.profileId;
  const agentKey = decodeURIComponent(agentParam ?? '');
  const profileId = profileParam ?? '';
  const { agent, profile, modelConfig, permissionSettings, passthroughConfig } = useAgentProfileSettings(agentKey, profileId);

  if (!agent || !profile) {
    return (
      <Card>
        <CardContent className="py-12 text-center">
          <p className="text-sm text-muted-foreground">Profile not found.</p>
          <Button className="mt-4" asChild>
            <Link href="/settings/agents">Back to Agents</Link>
          </Button>
        </CardContent>
      </Card>
    );
  }

  return (
    <ProfileEditor
      key={profile.id}
      agent={agent}
      profile={profile}
      modelConfig={modelConfig}
      permissionSettings={permissionSettings}
      passthroughConfig={passthroughConfig}
      initialMcpConfig={initialMcpConfig}
    />
  );
}
