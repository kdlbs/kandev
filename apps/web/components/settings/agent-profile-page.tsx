"use client";

import { useCallback, useMemo, useState } from "react";
import Link from "@/components/routing/app-link";
import { useParams } from "@/lib/routing/client-router";
import { IconTrash } from "@tabler/icons-react";
import { areCLIFlagsEqual } from "@/lib/cli-flags";
import { areConfigOptionsEqual } from "@/lib/config-options";
import { Badge } from "@kandev/ui/badge";
import { Button } from "@kandev/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@kandev/ui/card";
import { Separator } from "@kandev/ui/separator";
import { useToast } from "@/components/toast-provider";
import { useSettingsSaveContributor } from "@/components/settings/settings-save-provider";
import { SettingsCard } from "@/components/settings/settings-card";
import { ProfileFormFields, type ProfileFormData } from "@/components/settings/profile-form-fields";
import {
  arePermissionsDirty,
  permissionsToProfilePatch,
  profilePermissionValues,
} from "@/lib/agent-permissions";
import { toAgentProfilePatch } from "@/app/settings/agents/[agentId]/agent-save-helpers";
import { deleteAgentProfileAction, updateAgentProfileAction } from "@/app/actions/agents";
import {
  AgentProfileDeleteConfirmDialog,
  AgentProfileDeleteConflictDialog,
  type AgentProfileDeleteConflict,
} from "@/components/settings/agent-profile-delete-dialog";
import {
  ProfileEnvVarsSection,
  areEnvVarsEqual,
} from "@/components/settings/profile-edit/profile-env-vars-section";
import { CustomCLIFlagsCard } from "@/components/settings/cli-flags-field";

export {
  ProfileEnvVarsEditor,
  ProfileEnvVarsSection,
} from "@/components/settings/profile-edit/profile-env-vars-section";
import { useSecrets } from "@/hooks/domains/settings/use-secrets";
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
};

function ProfileEditorHeader({
  agentName,
  agentDisplayName,
  savedProfileName,
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
  savedProfile: AgentProfile;
  isDirty: boolean;
  onDraftChange: (patch: Partial<AgentProfile>) => void;
  modelConfig: ModelConfig;
  permissionSettings: Record<string, PermissionSetting>;
  passthroughConfig: PassthroughConfig | null;
};

function ProfileSettingsCard({
  agent,
  draft,
  savedProfile,
  isDirty,
  onDraftChange,
  modelConfig,
  permissionSettings,
  passthroughConfig,
}: ProfileSettingsCardProps) {
  const handleFormChange = (patch: Partial<ProfileFormData>) => {
    onDraftChange(toAgentProfilePatch(patch));
  };
  const permissionValues = profilePermissionValues(draft, permissionSettings);
  const savedPermissionValues = profilePermissionValues(savedProfile, permissionSettings);

  return (
    <SettingsCard isDirty={isDirty}>
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
            config_options: draft.configOptions ?? {},
            auto_approve: permissionValues.auto_approve,
            allow_indexing: permissionValues.allow_indexing,
            cli_passthrough: draft.cliPassthrough,
            cli_flags: draft.cliFlags ?? [],
          }}
          baselineProfile={{
            name: savedProfile.name,
            model: savedProfile.model,
            mode: savedProfile.mode ?? "",
            config_options: savedProfile.configOptions ?? {},
            auto_approve: savedPermissionValues.auto_approve,
            allow_indexing: savedPermissionValues.allow_indexing,
            cli_passthrough: savedProfile.cliPassthrough,
            cli_flags: savedProfile.cliFlags ?? [],
          }}
          onChange={handleFormChange}
          modelConfig={modelConfig}
          permissionSettings={permissionSettings}
          passthroughConfig={passthroughConfig}
          agentName={agent.name}
          lockPassthrough={Boolean(agent.tui_config)}
          hideCustomCLIFlags
        />
      </CardContent>
    </SettingsCard>
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
          label: `${agentProfile.agentDisplayName ?? ""} • ${agentProfile.name}`,
          agent_id: agentItem.id,
          agent_name: agentItem.name,
          cli_passthrough: agentProfile.cliPassthrough ?? false,
        })),
      ),
    );
  };
}

function useProfileEditorState(
  profile: AgentProfile,
  permissionSettings: Record<string, PermissionSetting>,
) {
  const [draft, setDraft] = useState<AgentProfile>({ ...profile });
  const [savedProfile, setSavedProfile] = useState<AgentProfile>(profile);
  const [saveStatus, setSaveStatus] = useState<"idle" | "loading" | "success" | "error">("idle");

  const isDirty = useMemo(
    () =>
      draft.name !== savedProfile.name ||
      draft.model !== savedProfile.model ||
      (draft.mode ?? "") !== (savedProfile.mode ?? "") ||
      !areConfigOptionsEqual(draft.configOptions, savedProfile.configOptions) ||
      arePermissionsDirty(draft, savedProfile, permissionSettings) ||
      draft.cliPassthrough !== savedProfile.cliPassthrough ||
      !areCLIFlagsEqual(draft.cliFlags ?? [], savedProfile.cliFlags ?? []) ||
      !areEnvVarsEqual(draft.envVars, savedProfile.envVars),
    [draft, savedProfile, permissionSettings],
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
  setDraft: React.Dispatch<React.SetStateAction<AgentProfile>>;
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
    // default", which is applied through ACP session model selection at session start.
    setSaveStatus("loading");
    try {
      const updated = await updateAgentProfileAction(draft.id, {
        name: draft.name,
        model: draft.model,
        mode: draft.mode,
        config_options: draft.configOptions ?? {},
        ...permissionsToProfilePatch(draft),
        cli_passthrough: draft.cliPassthrough,
        cli_flags: draft.cliFlags,
        env_vars: draft.envVars ?? [],
      });
      setSavedProfile(updated);
      setDraft((current) => preserveNewerProfileDraft(current, draft, updated));
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
      throw error;
    }
  };
}

export function preserveNewerProfileDraft(
  current: AgentProfile,
  submitted: AgentProfile,
  saved: AgentProfile,
): AgentProfile {
  return current === submitted ? saved : current;
}

function useProfileDelete(
  agent: Agent,
  draft: AgentProfile,
  settingsAgents: Agent[],
  syncAgentsToStore: (agents: Agent[]) => void,
  toast: ReturnType<typeof useToast>["toast"],
) {
  const [showDeleteConfirm, setShowDeleteConfirm] = useState(false);
  const [conflict, setConflict] = useState<AgentProfileDeleteConflict | null>(null);

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

  const requestDelete = () => {
    setShowDeleteConfirm(true);
  };

  const handleDeleteProfile = async () => {
    setShowDeleteConfirm(false);
    const result = await deleteAgentProfileAction(draft.id);
    if (result.status === "ok") {
      removeProfileFromStore();
    } else if (result.status === "conflict") {
      setConflict({
        activeSessions: result.activeSessions,
        watchers: result.watchers,
        routingTiers: result.routingTiers,
      });
    } else {
      toast({ title: "Failed to delete profile", description: result.message, variant: "error" });
    }
  };

  const handleForceDelete = async () => {
    const result = await deleteAgentProfileAction(draft.id, true);
    setConflict(null);
    if (result.status === "ok") {
      removeProfileFromStore();
    } else if (result.status === "conflict") {
      setConflict({
        activeSessions: result.activeSessions,
        watchers: result.watchers,
        routingTiers: result.routingTiers,
      });
    } else if (result.status === "error") {
      toast({ title: "Failed to delete profile", description: result.message, variant: "error" });
    }
  };

  return {
    requestDelete,
    showDeleteConfirm,
    setShowDeleteConfirm,
    handleDeleteProfile,
    conflict,
    setConflict,
    handleForceDelete,
  };
}

type ProfileDeleteDialogsProps = {
  showDeleteConfirm: boolean;
  setShowDeleteConfirm: (open: boolean) => void;
  handleDeleteProfile: () => void;
  conflict: AgentProfileDeleteConflict | null;
  setConflict: (c: AgentProfileDeleteConflict | null) => void;
  handleForceDelete: () => void;
};

function ProfileDeleteDialogs({
  showDeleteConfirm,
  setShowDeleteConfirm,
  handleDeleteProfile,
  conflict,
  setConflict,
  handleForceDelete,
}: ProfileDeleteDialogsProps) {
  return (
    <>
      <AgentProfileDeleteConfirmDialog
        open={showDeleteConfirm}
        onOpenChange={(open) => {
          if (!open) setShowDeleteConfirm(false);
        }}
        onConfirm={handleDeleteProfile}
      />

      <AgentProfileDeleteConflictDialog
        conflict={conflict}
        onOpenChange={(open) => {
          if (!open) setConflict(null);
        }}
        onConfirm={handleForceDelete}
      />
    </>
  );
}

type ProfileEditorBodyProps = {
  agent: Agent;
  draft: AgentProfile;
  savedProfile: AgentProfile;
  isDirty: boolean;
  updateDraft: (patch: Partial<AgentProfile>) => void;
  modelConfig: ModelConfig;
  permissionSettings: Record<string, PermissionSetting>;
  passthroughConfig: PassthroughConfig | null;
  secrets: { id: string; name: string }[];
  initialMcpConfig?: AgentProfileMcpConfig | null;
  onToastError: (error: unknown) => void;
};

function ProfileEditorBody({
  agent,
  draft,
  savedProfile,
  isDirty,
  updateDraft,
  modelConfig,
  permissionSettings,
  passthroughConfig,
  secrets,
  initialMcpConfig,
  onToastError,
}: ProfileEditorBodyProps) {
  return (
    <>
      <ProfileSettingsCard
        agent={agent}
        draft={draft}
        savedProfile={savedProfile}
        isDirty={isDirty}
        onDraftChange={updateDraft}
        modelConfig={modelConfig}
        permissionSettings={permissionSettings}
        passthroughConfig={passthroughConfig}
      />

      <CustomCLIFlagsCard
        flags={draft.cliFlags ?? []}
        baselineFlags={savedProfile.cliFlags ?? []}
        onChange={(next) => updateDraft({ cliFlags: next })}
        permissionSettings={permissionSettings}
      />

      <ProfileEnvVarsSection
        envVars={draft.envVars}
        baselineEnvVars={savedProfile.envVars}
        onChange={updateDraft}
      />

      <CommandPreviewCard
        agentName={agent.name}
        model={draft.model}
        permissionSettings={{ allow_indexing: draft.allowIndexing }}
        cliPassthrough={draft.cliPassthrough}
        cliFlags={draft.cliFlags ?? []}
        envVars={draft.envVars}
        secrets={secrets}
      />

      <ProfileMcpConfigCard
        profileId={draft.id}
        supportsMcp={agent.supports_mcp}
        cliPassthrough={draft.cliPassthrough}
        mcpInjection={passthroughConfig?.mcp_injection}
        initialConfig={initialMcpConfig}
        onToastError={onToastError}
      />
    </>
  );
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
  const { items: secrets } = useSecrets();
  const { draft, setDraft, savedProfile, setSavedProfile, setSaveStatus, isDirty } =
    useProfileEditorState(profile, permissionSettings);
  const updateDraft = useCallback(
    (patch: Partial<AgentProfile>) => {
      setDraft((current) => {
        if (patch.envVars !== undefined && areEnvVarsEqual(patch.envVars, current.envVars)) {
          // envVars unchanged — apply the rest of the patch (if any) but skip envVars.
          const { envVars: _ignored, ...rest } = patch;
          if (Object.keys(rest).length === 0) return current;
          return { ...current, ...rest };
        }
        return { ...current, ...patch };
      });
    },
    [setDraft],
  );
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
  useSettingsSaveContributor({
    id: `agent-profile:${draft.id}`,
    revision: JSON.stringify(draft),
    isDirty,
    canSave: Boolean(draft.name.trim()),
    invalidReason: draft.name.trim() ? undefined : "Profile name is required.",
    save: handleSave,
    discard: () => setDraft(savedProfile),
  });
  const {
    requestDelete,
    showDeleteConfirm,
    setShowDeleteConfirm,
    handleDeleteProfile,
    conflict,
    setConflict,
    handleForceDelete,
  } = useProfileDelete(agent, draft, settingsAgents, syncAgentsToStore, toast);

  return (
    <div className="space-y-8">
      <ProfileEditorHeader
        agentName={agent.name}
        agentDisplayName={profile.agentDisplayName ?? ""}
        savedProfileName={savedProfile.name}
      />

      <Separator />

      <ProfileEditorBody
        agent={agent}
        draft={draft}
        savedProfile={savedProfile}
        isDirty={isDirty}
        updateDraft={updateDraft}
        modelConfig={modelConfig}
        permissionSettings={permissionSettings}
        passthroughConfig={passthroughConfig}
        secrets={secrets}
        initialMcpConfig={initialMcpConfig}
        onToastError={(error) =>
          toast({
            title: "Failed to save MCP config",
            description: errorMessage(error),
            variant: "error",
          })
        }
      />

      <DeleteProfileCard onDelete={requestDelete} />

      <ProfileDeleteDialogs
        showDeleteConfirm={showDeleteConfirm}
        setShowDeleteConfirm={setShowDeleteConfirm}
        handleDeleteProfile={handleDeleteProfile}
        conflict={conflict}
        setConflict={setConflict}
        handleForceDelete={handleForceDelete}
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
