"use client";

import { useTranslation } from "react-i18next";
import { IconPlus, IconTrash } from "@tabler/icons-react";
import Link from "@/components/routing/app-link";
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
  AlertDialogTrigger,
} from "@kandev/ui/alert-dialog";
import { Button } from "@kandev/ui/button";
import { CardContent } from "@kandev/ui/card";
import { SettingsCard } from "@/components/settings/settings-card";
import { SettingsCardHeader } from "@/components/settings/settings-card-header";
import { SettingsPageHeader, SETTINGS_TYPOGRAPHY } from "@/components/settings/settings-typography";
import { settingsActionClassName } from "@/components/settings/settings-control";
import { ProfileFormFields, type ProfileFormData } from "@/components/settings/profile-form-fields";
import { ProfileEnvVarsSection } from "@/components/settings/agent-profile-page";
import { CustomCLIFlagsCard } from "@/components/settings/cli-flags-field";
import type { Agent, ModelConfig, PermissionSetting, PassthroughConfig } from "@/lib/types/http";
import { ProfileMcpConfigCard } from "./profile-mcp-config-card";
import { profilePermissionValues } from "@/lib/agent-permissions";
import { DynamicAgentProfileEditor } from "@/components/settings/dynamic-agent-profile-editor";
import {
  isProfileDirty,
  toAgentProfilePatch,
  type DraftProfile,
  type DraftAgent,
} from "./agent-save-helpers";

function profileFormData(
  profile: DraftProfile,
  permissionSettings: Record<string, PermissionSetting>,
): ProfileFormData {
  const permissions = profilePermissionValues(profile, permissionSettings);
  return {
    name: profile.name,
    model: profile.model,
    mode: profile.mode ?? "",
    config_options: profile.configOptions ?? {},
    auto_approve: permissions.auto_approve,
    allow_indexing: permissions.allow_indexing,
    cli_passthrough: profile.cliPassthrough ?? false,
    cli_flags: profile.cliFlags ?? [],
    command_prefix: profile.commandPrefix ?? "",
  };
}

export type AgentHeaderProps = {
  displayName: string;
  matchedPath: string | null | undefined;
  isCreateMode: boolean;
  savedAgent: Agent | null;
  showInstallationStatus?: boolean;
  onDelete?: () => void;
};

export function AgentHeader({
  displayName,
  matchedPath,
  isCreateMode,
  savedAgent,
  showInstallationStatus = true,
  onDelete,
}: AgentHeaderProps) {
  const { t } = useTranslation();
  return (
    <SettingsPageHeader
      title={
        <span className="flex min-w-0 flex-wrap items-center gap-2">
          <span className="min-w-0 break-words">{displayName}</span>
          {showInstallationStatus && (
            <span
              className={
                SETTINGS_TYPOGRAPHY.meta +
                " max-w-full truncate rounded-full border border-muted-foreground/30 px-2 py-1"
              }
            >
              {matchedPath ?? t("agents:installationNotDetected")}
            </span>
          )}
        </span>
      }
      description={
        isCreateMode ? t("agents:createProfileIntro") : t("agents:configureProfilesIntro")
      }
      actions={
        savedAgent?.tui_config && onDelete ? (
          <AlertDialog>
            <AlertDialogTrigger asChild>
              <Button
                variant="destructive"
                size="sm"
                className={settingsActionClassName("cursor-pointer")}
              >
                <IconTrash className="h-4 w-4 mr-2" />
                {t("agents:deleteAgent")}
              </Button>
            </AlertDialogTrigger>
            <AlertDialogContent>
              <AlertDialogHeader>
                <AlertDialogTitle>
                  {t("agents:deleteAgentTitle", { name: displayName })}
                </AlertDialogTitle>
                <AlertDialogDescription>
                  {t("agents:deleteAgentDescription")}
                </AlertDialogDescription>
              </AlertDialogHeader>
              <AlertDialogFooter>
                <AlertDialogCancel className="cursor-pointer">
                  {t("common:cancel")}
                </AlertDialogCancel>
                <AlertDialogAction onClick={onDelete} className="cursor-pointer">
                  {t("agents:delete")}
                </AlertDialogAction>
              </AlertDialogFooter>
            </AlertDialogContent>
          </AlertDialog>
        ) : undefined
      }
    />
  );
}

export type ProfileCardItemProps = {
  profile: DraftProfile;
  savedProfile?: DraftProfile;
  isNew: boolean;
  draftAgent: DraftAgent;
  currentAgentModelConfig: ModelConfig;
  permissionSettings: Record<string, PermissionSetting>;
  passthroughConfig: PassthroughConfig | null;
  onProfileChange: (profileId: string, patch: Partial<DraftProfile>) => void;
  onProfileMcpChange: (
    profileId: string,
    patch: Partial<NonNullable<DraftProfile["mcp_config"]>>,
  ) => void;
  onRemoveProfile: (profileId: string) => void;
  onToastError: (error: unknown) => void;
};

export function ProfileCardItem({
  profile,
  savedProfile,
  isNew,
  draftAgent,
  currentAgentModelConfig,
  permissionSettings,
  passthroughConfig,
  onProfileChange,
  onProfileMcpChange,
  onRemoveProfile,
  onToastError,
}: ProfileCardItemProps) {
  const formProfile = profileFormData(profile, permissionSettings);
  const baselineProfile = savedProfile
    ? profileFormData(savedProfile, permissionSettings)
    : undefined;
  const dirty =
    isNew ||
    !savedProfile ||
    Boolean(profile.mcp_config?.dirty) ||
    isProfileDirty(profile, savedProfile);
  return (
    <SettingsCard id={`profile-card-${profile.id}`} isDirty={dirty}>
      <CardContent className="pt-6 space-y-4">
        <ProfileFormFields
          profile={formProfile}
          baselineProfile={baselineProfile}
          onChange={(patch) => onProfileChange(profile.id, toAgentProfilePatch(patch))}
          modelConfig={currentAgentModelConfig}
          permissionSettings={permissionSettings}
          passthroughConfig={passthroughConfig}
          agentName={draftAgent.name}
          onRemove={() => onRemoveProfile(profile.id)}
          canRemove={draftAgent.profiles.length > 1}
          lockPassthrough={Boolean(draftAgent.tui_config)}
          hideCustomCLIFlags
        />
        <CustomCLIFlagsCard
          flags={profile.cliFlags ?? []}
          baselineFlags={savedProfile?.cliFlags}
          onChange={(next) => onProfileChange(profile.id, { cliFlags: next })}
          permissionSettings={permissionSettings}
        />
        <ProfileEnvVarsSection
          envVars={profile.envVars}
          baselineEnvVars={savedProfile?.envVars}
          onChange={(patch) => onProfileChange(profile.id, patch)}
        />
        <ProfileMcpConfigCard
          profileId={profile.id}
          workspaceId={draftAgent.workspace_id}
          supportsMcp={draftAgent.supports_mcp}
          cliPassthrough={profile.cliPassthrough ?? false}
          mcpInjection={passthroughConfig?.mcp_injection}
          draftState={profile.mcp_config}
          onDraftStateChange={(patch) => onProfileMcpChange(profile.id, patch)}
          onToastError={onToastError}
        />
      </CardContent>
    </SettingsCard>
  );
}

export type ProfilesCardProps = {
  displayName: string;
  isCreateMode: boolean;
  isAgentDirty: boolean;
  draftAgent: DraftAgent;
  savedAgent: Agent | null;
  newProfileId: string | null;
  currentAgentModelConfig: ModelConfig;
  permissionSettings: Record<string, PermissionSetting>;
  passthroughConfig: PassthroughConfig | null;
  onAddProfile: () => void;
  onProfileChange: (profileId: string, patch: Partial<DraftProfile>) => void;
  onProfileMcpChange: (
    profileId: string,
    patch: Partial<NonNullable<DraftProfile["mcp_config"]>>,
  ) => void;
  onRemoveProfile: (profileId: string) => void;
  onToastError: (error: unknown) => void;
};

function dynamicProfileRoute(agent: Agent, profile: DraftProfile): string {
  return `/settings/agents/${encodeURIComponent(agent.name)}/profiles/${encodeURIComponent(profile.id)}`;
}

function DynamicProfileRouteRow({ agent, profile }: { agent: Agent; profile: DraftProfile }) {
  const { t } = useTranslation();
  const candidateCount = profile.dynamic?.candidates.length ?? 0;
  return (
    <Link
      href={dynamicProfileRoute(agent, profile)}
      className="flex min-h-11 min-w-0 items-center justify-between gap-3 rounded-md border p-3 transition-colors hover:border-foreground/30 hover:bg-muted/50"
      data-testid={`dynamic-profile-route-${profile.id}`}
    >
      <span className="min-w-0 truncate text-sm font-medium">
        {profile.name || t("agents:profileName")}
      </span>
      <span className="shrink-0 text-xs text-muted-foreground">
        {t("agents:dynamicCandidates")}: {candidateCount}
      </span>
    </Link>
  );
}

export function ProfilesCard({
  displayName,
  isCreateMode,
  isAgentDirty,
  draftAgent,
  savedAgent,
  newProfileId,
  currentAgentModelConfig,
  permissionSettings,
  passthroughConfig,
  onAddProfile,
  onProfileChange,
  onProfileMcpChange,
  onRemoveProfile,
  onToastError,
}: ProfilesCardProps) {
  const { t } = useTranslation();
  const dynamicProfiles = draftAgent.profiles.filter(
    (profile) => profile.kind === "dynamic" || draftAgent.name === "dynamic",
  );

  if (dynamicProfiles.length > 0) {
    return (
      <SettingsCard isDirty={isAgentDirty}>
        <SettingsCardHeader
          title={t("agents:dynamicProfileSettings")}
          actions={
            !isCreateMode && (
              <Button
                size="sm"
                variant="outline"
                onClick={onAddProfile}
                className={settingsActionClassName("cursor-pointer")}
              >
                <IconPlus className="mr-2 h-4 w-4" />
                {t("agents:addProfile")}
              </Button>
            )
          }
        />
        <CardContent className="space-y-6">
          {dynamicProfiles.map((profile) => (
            <div key={profile.id}>
              {profile.isNew ? (
                <DynamicAgentProfileEditor
                  agent={draftAgent}
                  profile={profile}
                  isCreating={Boolean(profile.isNew)}
                  onDraftChange={(patch) => onProfileChange(profile.id, patch)}
                />
              ) : (
                <DynamicProfileRouteRow agent={draftAgent} profile={profile} />
              )}
            </div>
          ))}
        </CardContent>
      </SettingsCard>
    );
  }

  return (
    <SettingsCard isDirty={isAgentDirty}>
      <SettingsCardHeader
        title={
          isCreateMode
            ? t("agents:createAgentProfileTitle", { name: displayName })
            : t("agents:agentProfilesTitle", { name: displayName })
        }
        actions={
          <Button
            size="sm"
            variant="outline"
            onClick={onAddProfile}
            className={settingsActionClassName("cursor-pointer")}
          >
            <IconPlus className="h-4 w-4 mr-2" />
            {t("agents:addProfile")}
          </Button>
        }
      />
      <CardContent className="space-y-4">
        {draftAgent.profiles.map((profile) => (
          <ProfileCardItem
            key={profile.id}
            profile={profile}
            savedProfile={savedAgent?.profiles.find((saved) => saved.id === profile.id)}
            isNew={profile.id === newProfileId}
            draftAgent={draftAgent}
            currentAgentModelConfig={currentAgentModelConfig}
            permissionSettings={permissionSettings}
            passthroughConfig={passthroughConfig}
            onProfileChange={onProfileChange}
            onProfileMcpChange={onProfileMcpChange}
            onRemoveProfile={onRemoveProfile}
            onToastError={onToastError}
          />
        ))}
      </CardContent>
    </SettingsCard>
  );
}
