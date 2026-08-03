"use client";

import { useCallback } from "react";
import { useTranslation } from "react-i18next";
import Link from "@/components/routing/app-link";
import { useParams } from "@/lib/routing/client-router";
import { IconTrash } from "@tabler/icons-react";
import { Badge } from "@kandev/ui/badge";
import { Button } from "@kandev/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@kandev/ui/card";
import { Separator } from "@kandev/ui/separator";
import { useToast } from "@/components/toast-provider";
import { useSettingsSaveContributor } from "@/components/settings/settings-save-provider";
import { SettingsCard } from "@/components/settings/settings-card";
import { ProfileFormFields, type ProfileFormData } from "@/components/settings/profile-form-fields";
import { profilePermissionValues } from "@/lib/agent-permissions";
import { toAgentProfilePatch } from "@/app/settings/agents/[agentId]/agent-save-helpers";
import {
  AgentProfileDeleteConfirmDialog,
  AgentProfileDeleteConflictDialog,
  type AgentProfileDeleteConflict,
} from "@/components/settings/agent-profile-delete-dialog";
import {
  ProfileEnvVarsSection,
  areEnvVarsEqual,
} from "@/components/settings/profile-edit/profile-env-vars-section";
import {
  errorMessage,
  useProfileDelete,
  useProfileEditorState,
  useProfileSave,
  useSyncAgentsToStore,
} from "@/components/settings/agent-profile-page-state";
import { CustomCLIFlagsCard } from "@/components/settings/cli-flags-field";

export {
  ProfileEnvVarsEditor,
  ProfileEnvVarsSection,
} from "@/components/settings/profile-edit/profile-env-vars-section";
export { preserveNewerProfileDraft } from "@/components/settings/agent-profile-page-state";
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
  const { t } = useTranslation();
  return (
    <div className="flex items-start justify-between">
      <div>
        <h2 className="text-2xl font-bold flex items-center gap-2">
          <AgentLogo agentName={agentName} size={28} className="shrink-0" />
          {agentDisplayName} • {savedProfileName}
        </h2>
        <p className="text-sm text-muted-foreground mt-1">
          {t("agents:agentProfileSettings", { name: agentDisplayName })}
        </p>
      </div>
    </div>
  );
}

type DeleteProfileCardProps = {
  onDelete: () => void;
};

function DeleteProfileCard({ onDelete }: DeleteProfileCardProps) {
  const { t } = useTranslation();
  return (
    <Card className="border-destructive">
      <CardHeader>
        <CardTitle className="text-destructive">{t("agents:deleteProfile")}</CardTitle>
      </CardHeader>
      <CardContent className="flex items-center justify-between">
        <div>
          <p className="text-sm font-medium">{t("agents:removeThisProfile")}</p>
          <p className="text-xs text-muted-foreground">{t("agents:actionCannotBeUndone")}</p>
        </div>
        <Button variant="destructive" onClick={onDelete}>
          <IconTrash className="h-4 w-4 mr-2" />
          {t("agents:delete")}
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
  const { t } = useTranslation();
  const handleFormChange = (patch: Partial<ProfileFormData>) => {
    onDraftChange(toAgentProfilePatch(patch));
  };
  const permissionValues = profilePermissionValues(draft, permissionSettings);
  const savedPermissionValues = profilePermissionValues(savedProfile, permissionSettings);

  return (
    <SettingsCard isDirty={isDirty}>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <span>{t("agents:profileSettings")}</span>
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
            command_prefix: draft.commandPrefix ?? "",
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
            command_prefix: savedProfile.commandPrefix ?? "",
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
        commandPrefix={draft.commandPrefix}
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
  const { t } = useTranslation();
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
    invalidReason: draft.name.trim() ? undefined : t("agents:profileNameRequired"),
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
            title: t("agents:failedToSaveMcpConfig"),
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
  const { t } = useTranslation();
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
          <p className="text-sm text-muted-foreground">{t("agents:profileNotFound")}</p>
          <Button className="mt-4" asChild>
            <Link href="/settings/agents">{t("agents:backToAgents")}</Link>
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
