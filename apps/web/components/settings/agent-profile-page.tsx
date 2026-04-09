"use client";

import { useMemo, useState } from "react";
import Link from "next/link";
import { useParams } from "next/navigation";
import { IconTrash } from "@tabler/icons-react";
import { Badge } from "@kandev/ui/badge";
import { Button } from "@kandev/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@kandev/ui/card";
import { Separator } from "@kandev/ui/separator";
import { useToast } from "@/components/toast-provider";
import { UnsavedChangesBadge, UnsavedSaveButton } from "@/components/settings/unsaved-indicator";
import { ProfileFormFields } from "@/components/settings/profile-form-fields";
import { deleteAgentProfileAction, updateAgentProfileAction } from "@/app/actions/agents";
import type { ActiveSessionInfo } from "@/lib/types/agent-profile-errors";
import { AgentProfileDeleteDialog } from "@/components/settings/agent-profile-delete-dialog";
import type {
  Agent,
  AgentProfile,
  ModelConfig,
  PermissionSetting,
  PassthroughConfig,
} from "@/lib/types/http";
import { useAppStore } from "@/components/state-provider";
import { AgentLogo } from "@/components/agent-logo";
import { ProfileMcpConfigCard } from "@/app/settings/agents/[agentId]/profile-mcp-config-card";
import { CommandPreviewCard } from "@/app/settings/agents/[agentId]/profiles/[profileId]/command-preview-card";
import type { AgentProfileMcpConfig } from "@/lib/types/http";
import { useAgentProfileSettings } from "@/app/settings/agents/[agentId]/profiles/[profileId]/use-agent-profile-settings";

type ProfileEditorProps = {
  agent: Agent;
  profile: AgentProfile;
  modelConfig: ModelConfig;
  permissionSettings: Record<string, PermissionSetting>;
  passthroughConfig: PassthroughConfig | null;
  initialMcpConfig?: AgentProfileMcpConfig | null;
};

type SaveStatus = "idle" | "loading" | "success" | "error";

type ProfileEditorHeaderProps = {
  agentName: string;
  agentDisplayName: string;
  savedProfileName: string;
  isDirty: boolean;
  isLoading: boolean;
  saveStatus: SaveStatus;
  onSave: () => void;
};

function ProfileEditorHeader({
  agentName,
  agentDisplayName,
  savedProfileName,
  isDirty,
  isLoading,
  saveStatus,
  onSave,
}: ProfileEditorHeaderProps) {
  return (
    <div className="flex items-start justify-between">
      <div>
        <h2 className="text-2xl font-bold flex items-center gap-2">
          <AgentLogo agentName={agentName} size={28} className="shrink-0" />
          {agentDisplayName} • {savedProfileName}
        </h2>
        <p className="text-sm text-muted-foreground mt-1">{agentDisplayName} profile settings</p>
      </div>
      <div className="flex items-center gap-3">
        {isDirty && <UnsavedChangesBadge />}
        <UnsavedSaveButton
          isDirty={isDirty}
          isLoading={isLoading}
          status={saveStatus}
          onClick={onSave}
        />
      </div>
    </div>
  );
}

type DeleteProfileCardProps = {
  onDelete: () => void;
};

function DeleteProfileCard({ onDelete }: DeleteProfileCardProps) {
  return (
    <Card className="border-destructive">
      <CardHeader>
        <CardTitle className="text-destructive">Delete profile</CardTitle>
      </CardHeader>
      <CardContent className="flex items-center justify-between">
        <div>
          <p className="text-sm font-medium">Remove this profile</p>
          <p className="text-xs text-muted-foreground">This action cannot be undone.</p>
        </div>
        <Button variant="destructive" onClick={onDelete}>
          <IconTrash className="h-4 w-4 mr-2" />
          Delete
        </Button>
      </CardContent>
    </Card>
  );
}

type ProfileSettingsCardProps = {
  agent: Agent;
  draft: AgentProfile;
  onDraftChange: (patch: Partial<AgentProfile>) => void;
  modelConfig: ModelConfig;
  permissionSettings: Record<string, PermissionSetting>;
  passthroughConfig: PassthroughConfig | null;
};

function ProfileSettingsCard({
  agent,
  draft,
  onDraftChange,
  modelConfig,
  permissionSettings,
  passthroughConfig,
}: ProfileSettingsCardProps) {
  return (
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
            mode: draft.mode ?? "",
            allow_indexing: draft.allow_indexing,
            cli_passthrough: draft.cli_passthrough,
          }}
          onChange={onDraftChange}
          modelConfig={modelConfig}
          permissionSettings={permissionSettings}
          passthroughConfig={passthroughConfig}
          agentName={agent.name}
          lockPassthrough={Boolean(agent.tui_config)}
        />
      </CardContent>
    </Card>
  );
}

function useSyncAgentsToStore() {
  const setSettingsAgents = useAppStore((state) => state.setSettingsAgents);
  const setAgentProfiles = useAppStore((state) => state.setAgentProfiles);
  return (nextAgents: Agent[]) => {
    setSettingsAgents(nextAgents);
    setAgentProfiles(
      nextAgents.flatMap((agentItem) =>
        agentItem.profiles.map((agentProfile) => ({
          id: agentProfile.id,
          label: `${agentProfile.agent_display_name} • ${agentProfile.name}`,
          agent_id: agentItem.id,
          agent_name: agentItem.name,
          cli_passthrough: agentProfile.cli_passthrough,
        })),
      ),
    );
  };
}

function useProfileEditorState(profile: AgentProfile) {
  const [draft, setDraft] = useState<AgentProfile>({ ...profile });
  const [savedProfile, setSavedProfile] = useState<AgentProfile>(profile);
  const [saveStatus, setSaveStatus] = useState<"idle" | "loading" | "success" | "error">("idle");

  const isDirty = useMemo(
    () =>
      draft.name !== savedProfile.name ||
      draft.model !== savedProfile.model ||
      (draft.mode ?? "") !== (savedProfile.mode ?? "") ||
      draft.allow_indexing !== savedProfile.allow_indexing ||
      draft.cli_passthrough !== savedProfile.cli_passthrough,
    [draft, savedProfile],
  );

  return { draft, setDraft, savedProfile, setSavedProfile, saveStatus, setSaveStatus, isDirty };
}

const FALLBACK_ERROR = "Request failed";

function errorMessage(error: unknown): string {
  return error instanceof Error ? error.message : FALLBACK_ERROR;
}

type ProfileEditorActionsOptions = {
  agent: Agent;
  draft: AgentProfile;
  setSavedProfile: (p: AgentProfile) => void;
  setDraft: (p: AgentProfile) => void;
  setSaveStatus: (s: SaveStatus) => void;
  settingsAgents: Agent[];
  syncAgentsToStore: (agents: Agent[]) => void;
  toast: ReturnType<typeof useToast>["toast"];
};

function useProfileSave({
  agent,
  draft,
  setSavedProfile,
  setDraft,
  setSaveStatus,
  settingsAgents,
  syncAgentsToStore,
  toast,
}: ProfileEditorActionsOptions) {
  return async () => {
    if (!draft.name.trim()) {
      toast({
        title: "Profile name is required",
        description: "Please enter a profile name before saving.",
        variant: "error",
      });
      return;
    }
    // Model is optional — an empty profile model means "use the agent's
    // default", which is applied via ACP session/set_model at session start.
    setSaveStatus("loading");
    try {
      const updated = await updateAgentProfileAction(draft.id, {
        name: draft.name,
        model: draft.model,
        mode: draft.mode,
        allow_indexing: draft.allow_indexing,
        cli_passthrough: draft.cli_passthrough,
      });
      setSavedProfile(updated);
      setDraft(updated);
      const nextAgents = settingsAgents.map((agentItem: Agent) =>
        agentItem.id === agent.id
          ? {
              ...agentItem,
              profiles: agentItem.profiles.map((p: AgentProfile) =>
                p.id === updated.id ? updated : p,
              ),
            }
          : agentItem,
      );
      syncAgentsToStore(nextAgents);
      setSaveStatus("success");
    } catch (error) {
      setSaveStatus("error");
      toast({
        title: "Failed to save profile",
        description: errorMessage(error),
        variant: "error",
      });
    }
  };
}

function useProfileDelete(
  agent: Agent,
  draft: AgentProfile,
  settingsAgents: Agent[],
  syncAgentsToStore: (agents: Agent[]) => void,
  toast: ReturnType<typeof useToast>["toast"],
) {
  const [conflictSessions, setConflictSessions] = useState<ActiveSessionInfo[] | null>(null);

  const removeProfileFromStore = () => {
    const nextAgents = settingsAgents.map((agentItem: Agent) =>
      agentItem.id === agent.id
        ? {
            ...agentItem,
            profiles: agentItem.profiles.filter((p: AgentProfile) => p.id !== draft.id),
          }
        : agentItem,
    );
    syncAgentsToStore(nextAgents);
    window.location.assign("/settings/agents");
  };

  const handleDeleteProfile = async () => {
    const result = await deleteAgentProfileAction(draft.id);
    if (result.status === "ok") {
      removeProfileFromStore();
    } else if (result.status === "conflict") {
      setConflictSessions(result.activeSessions);
    } else {
      toast({ title: "Failed to delete profile", description: result.message, variant: "error" });
    }
  };

  const handleForceDelete = async () => {
    const result = await deleteAgentProfileAction(draft.id, true);
    setConflictSessions(null);
    if (result.status === "ok") {
      removeProfileFromStore();
    } else if (result.status === "error") {
      toast({ title: "Failed to delete profile", description: result.message, variant: "error" });
    }
  };

  return { handleDeleteProfile, conflictSessions, setConflictSessions, handleForceDelete };
}

function ProfileEditor({
  agent,
  profile,
  modelConfig,
  permissionSettings,
  passthroughConfig,
  initialMcpConfig,
}: ProfileEditorProps) {
  const { toast } = useToast();
  const settingsAgents = useAppStore((state) => state.settingsAgents.items);
  const syncAgentsToStore = useSyncAgentsToStore();
  const { draft, setDraft, savedProfile, setSavedProfile, saveStatus, setSaveStatus, isDirty } =
    useProfileEditorState(profile);
  const handleSave = useProfileSave({
    agent,
    draft,
    setSavedProfile,
    setDraft,
    setSaveStatus,
    settingsAgents,
    syncAgentsToStore,
    toast,
  });
  const { handleDeleteProfile, conflictSessions, setConflictSessions, handleForceDelete } =
    useProfileDelete(agent, draft, settingsAgents, syncAgentsToStore, toast);

  return (
    <div className="space-y-8">
      <ProfileEditorHeader
        agentName={agent.name}
        agentDisplayName={profile.agent_display_name}
        savedProfileName={savedProfile.name}
        isDirty={isDirty}
        isLoading={saveStatus === "loading"}
        saveStatus={saveStatus}
        onSave={handleSave}
      />

      <Separator />

      <ProfileSettingsCard
        agent={agent}
        draft={draft}
        onDraftChange={(patch) => setDraft({ ...draft, ...patch })}
        modelConfig={modelConfig}
        permissionSettings={permissionSettings}
        passthroughConfig={passthroughConfig}
      />

      <CommandPreviewCard
        agentName={agent.name}
        model={draft.model}
        permissionSettings={{
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
            title: "Failed to save MCP config",
            description: errorMessage(error),
            variant: "error",
          })
        }
      />

      <DeleteProfileCard onDelete={handleDeleteProfile} />

      <AgentProfileDeleteDialog
        activeSessions={conflictSessions}
        onOpenChange={(open) => {
          if (!open) setConflictSessions(null);
        }}
        onConfirm={handleForceDelete}
      />
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
  const agentKey = decodeURIComponent(agentParam ?? "");
  const profileId = profileParam ?? "";
  const { agent, profile, modelConfig, permissionSettings, passthroughConfig } =
    useAgentProfileSettings(agentKey, profileId);

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
