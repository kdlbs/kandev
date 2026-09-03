"use client";

import { useCallback, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import Link from "@/components/routing/app-link";
import { useParams } from "@/lib/routing/client-router";
import { IconCopy, IconTrash } from "@tabler/icons-react";
import { Badge } from "@kandev/ui/badge";
import { Button } from "@kandev/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@kandev/ui/card";
import { Separator } from "@kandev/ui/separator";
import { Switch } from "@kandev/ui/switch";
import { useToast } from "@/components/toast-provider";
import { useSettingsSaveContributor } from "@/components/settings/settings-save-provider";
import { SettingsCard } from "@/components/settings/settings-card";
import { ProfileFormFields, type ProfileFormData } from "@/components/settings/profile-form-fields";
import { profilePermissionValues } from "@/lib/agent-permissions";
import { toAgentProfilePatch } from "@/app/settings/agents/[agentId]/agent-save-helpers";
import {
  AgentProfileDeleteConfirmation,
  AgentProfileDeleteConflictDialog,
  AgentProfileDisableConflictDialog,
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
import {
  profileSaveInvalidReason,
  useProfileDuplicateAction,
} from "@/components/settings/agent-profile-duplicate-action";
import { CustomCLIFlagsCard } from "@/components/settings/cli-flags-field";
import { ProfileEnabledHelp } from "@/components/settings/profile-enabled-help";

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
import type { UtilityAgentReference } from "@/lib/types/agent-profile-errors";
import { useAppStore } from "@/components/state-provider";
import { AgentLogo } from "@/components/agent-logo";
import { ProfileMcpConfigCard } from "@/app/settings/agents/[agentId]/profile-mcp-config-card";
import { CommandPreviewCard } from "@/app/settings/agents/[agentId]/profiles/[profileId]/command-preview-card";
import type { AgentProfileMcpConfig } from "@/lib/types/http";
import { useAgentProfileSettings } from "@/app/settings/agents/[agentId]/profiles/[profileId]/use-agent-profile-settings";
import { agentProfileDiscoveryTarget } from "@/lib/settings-discovery/dynamic-targets";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import { DynamicAgentProfileEditor } from "@/components/settings/dynamic-agent-profile-editor";

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
  enabled: boolean;
  onEnabledChange: (enabled: boolean) => void;
  onDuplicate: () => void;
  duplicating: boolean;
};

function ProfileEditorHeader({
  agentName,
  agentDisplayName,
  savedProfileName,
  enabled,
  onEnabledChange,
  onDuplicate,
  duplicating,
}: ProfileEditorHeaderProps) {
  const { t } = useTranslation();
  return (
    <div className="flex flex-col items-stretch justify-between gap-4 md:flex-row md:items-start">
      <div className="min-w-0">
        <h2 className="text-2xl font-bold flex min-w-0 items-center gap-2 wrap-break-word">
          <AgentLogo agentName={agentName} size={28} className="shrink-0" />
          {agentDisplayName} • {savedProfileName}
        </h2>
        <p className="text-sm text-muted-foreground mt-1">
          {t("agents:agentProfileSettings", { name: agentDisplayName })}
        </p>
      </div>
      <div className="flex w-full flex-col gap-2 md:w-auto md:flex-row md:items-center md:gap-3 md:shrink-0">
        <Button
          variant="outline"
          onClick={onDuplicate}
          data-testid="duplicate-profile-header"
          className="min-h-11 w-full md:w-auto"
          disabled={duplicating}
          aria-busy={duplicating}
          title={t("agents:duplicateProfileNamed", { name: savedProfileName })}
        >
          <IconCopy className="h-4 w-4 mr-2" />
          {t("agents:duplicate")}
        </Button>
        <div className="flex items-center gap-1 text-left sm:text-right">
          <p className="text-sm font-medium">{t("agents:enabled")}</p>
          <ProfileEnabledHelp />
        </div>
        <Switch
          checked={enabled}
          onCheckedChange={onEnabledChange}
          data-testid="profile-enabled-toggle"
          aria-label={enabled ? t("agents:disableProfile") : t("agents:enableProfile")}
        />
      </div>
    </div>
  );
}

type DeleteProfileCardProps = {
  onDelete: () => void;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onConfirm: () => void | Promise<void>;
};

function DeleteProfileCard({ onDelete, open, onOpenChange, onConfirm }: DeleteProfileCardProps) {
  const { t } = useTranslation();
  const { isFinePointer } = useResponsiveBreakpoint();
  const deleteAnchorRef = useRef<HTMLButtonElement>(null);
  const closeDeleteConfirmation = () => {
    onOpenChange(false);
    queueMicrotask(() => deleteAnchorRef.current?.focus());
  };
  return (
    <Card className="border-destructive">
      <CardHeader>
        <CardTitle className="text-destructive">{t("agents:deleteProfile")}</CardTitle>
      </CardHeader>
      <CardContent className="flex flex-wrap items-center justify-between gap-3">
        <div className="min-w-0">
          <p className="text-sm font-medium">{t("agents:removeThisProfile")}</p>
          <p className="text-xs text-muted-foreground">{t("agents:actionCannotBeUndone")}</p>
        </div>
        {!open || isFinePointer ? (
          <Button
            ref={deleteAnchorRef}
            variant="destructive"
            className="cursor-pointer"
            onClick={onDelete}
            data-testid="profile-delete-trigger"
          >
            <IconTrash className="h-4 w-4 mr-2" />
            {t("agents:delete")}
          </Button>
        ) : null}
        {!isFinePointer && open ? (
          <div className="basis-full min-w-0">
            <AgentProfileDeleteConfirmation
              open={open}
              isFinePointer={false}
              anchorRef={deleteAnchorRef}
              onOpenChange={onOpenChange}
              onCancel={closeDeleteConfirmation}
              onConfirm={onConfirm}
            />
          </div>
        ) : null}
      </CardContent>
      {isFinePointer ? (
        <AgentProfileDeleteConfirmation
          open={open}
          isFinePointer
          anchorRef={deleteAnchorRef}
          onOpenChange={onOpenChange}
          onCancel={closeDeleteConfirmation}
          onConfirm={onConfirm}
        />
      ) : null}
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
  onModelConfigResolutionPendingChange: (pending: boolean) => void;
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
  onModelConfigResolutionPendingChange,
}: ProfileSettingsCardProps) {
  const { t } = useTranslation();
  const handleFormChange = (patch: Partial<ProfileFormData>) => {
    onDraftChange(toAgentProfilePatch(patch));
  };
  const permissionValues = profilePermissionValues(draft, permissionSettings);
  const savedPermissionValues = profilePermissionValues(savedProfile, permissionSettings);

  return (
    <SettingsCard
      isDirty={isDirty}
      discoveryTargetId={agentProfileDiscoveryTarget(draft.id, "profile-settings")}
    >
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
            fallback_model: draft.fallbackModel ?? "",
            auto_fallback: draft.autoFallback ?? false,
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
            fallback_model: savedProfile.fallbackModel ?? "",
            auto_fallback: savedProfile.autoFallback ?? false,
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
          onModelConfigResolutionPendingChange={onModelConfigResolutionPendingChange}
          lockPassthrough={Boolean(agent.tui_config)}
          hideCustomCLIFlags
        />
      </CardContent>
    </SettingsCard>
  );
}

type ProfileDeleteDialogsProps = {
  conflict: AgentProfileDeleteConflict | null;
  setConflict: (c: AgentProfileDeleteConflict | null) => void;
  handleForceDelete: () => void;
};

function ProfileDeleteDialogs({
  conflict,
  setConflict,
  handleForceDelete,
}: ProfileDeleteDialogsProps) {
  return (
    <>
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
  onModelConfigResolutionPendingChange: (pending: boolean) => void;
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
  onModelConfigResolutionPendingChange,
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
        onModelConfigResolutionPendingChange={onModelConfigResolutionPendingChange}
      />

      <CustomCLIFlagsCard
        flags={draft.cliFlags ?? []}
        baselineFlags={savedProfile.cliFlags ?? []}
        onChange={(next) => updateDraft({ cliFlags: next })}
        permissionSettings={permissionSettings}
        discoveryTargetId={agentProfileDiscoveryTarget(draft.id, "cli-flags")}
      />

      <ProfileEnvVarsSection
        envVars={draft.envVars}
        baselineEnvVars={savedProfile.envVars}
        onChange={updateDraft}
        discoveryTargetId={agentProfileDiscoveryTarget(draft.id, "environment-variables")}
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
        discoveryTargetId={agentProfileDiscoveryTarget(draft.id, "command-preview")}
      />

      <ProfileMcpConfigCard
        profileId={draft.id}
        workspaceId={agent.workspace_id}
        supportsMcp={agent.supports_mcp}
        cliPassthrough={draft.cliPassthrough}
        mcpInjection={passthroughConfig?.mcp_injection}
        initialConfig={initialMcpConfig}
        onToastError={onToastError}
      />
    </>
  );
}

// eslint-disable-next-line max-lines-per-function
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
  const [modelConfigResolutionPending, setModelConfigResolutionPending] = useState(false);
  const settingsAgents = useAppStore((state) => state.settingsAgents.items);
  const syncAgentsToStore = useSyncAgentsToStore();
  const { items: secrets } = useSecrets();
  const { draft, setDraft, savedProfile, setSavedProfile, setSaveStatus, isDirty } =
    useProfileEditorState(profile, permissionSettings);
  const [utilityConflict, setUtilityConflict] = useState<UtilityAgentReference[]>([]);
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
    savedProfile,
    setSavedProfile,
    setDraft,
    setSaveStatus,
    settingsAgents,
    syncAgentsToStore,
    toast,
    onUtilityConflict: setUtilityConflict,
  });
  useSettingsSaveContributor({
    id: `agent-profile:${draft.id}`,
    revision: JSON.stringify(draft),
    isDirty,
    canSave: Boolean(draft.name.trim()) && !modelConfigResolutionPending,
    invalidReason: profileSaveInvalidReason(draft.name, modelConfigResolutionPending, t),
    save: () => handleSave(),
    discard: () => setDraft(savedProfile),
  });
  const deleteState = useProfileDelete(agent, draft, settingsAgents, syncAgentsToStore, toast);

  const { handleDuplicate: handleDuplicateProfile, duplicating } = useProfileDuplicateAction({
    agent,
    draft,
    modelConfigResolutionPending,
    translate: t,
  });

  return (
    <div className="space-y-8">
      <ProfileEditorHeader
        agentName={agent.name}
        agentDisplayName={profile.agentDisplayName ?? ""}
        savedProfileName={savedProfile.name}
        enabled={draft.enabled ?? true}
        onEnabledChange={(next) => updateDraft({ enabled: next })}
        onDuplicate={() => void handleDuplicateProfile()}
        duplicating={duplicating}
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
        onModelConfigResolutionPendingChange={setModelConfigResolutionPending}
      />

      <DeleteProfileCard
        onDelete={deleteState.requestDelete}
        open={deleteState.showDeleteConfirm}
        onOpenChange={deleteState.setShowDeleteConfirm}
        onConfirm={deleteState.handleDeleteProfile}
      />

      <AgentProfileDisableConflictDialog
        agents={utilityConflict}
        onCancel={() => setUtilityConflict([])}
        onConfirm={async () => {
          setUtilityConflict([]);
          await handleSave(true);
        }}
      />

      <ProfileDeleteDialogs
        conflict={deleteState.conflict}
        setConflict={deleteState.setConflict}
        handleForceDelete={deleteState.handleForceDelete}
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

  if (profile.kind === "dynamic" || agent.name === "dynamic") {
    return <DynamicAgentProfileEditor agent={agent} profile={profile} />;
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
