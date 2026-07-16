"use client";

import { IconPlus, IconTrash } from "@tabler/icons-react";
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
import { CardContent, CardHeader, CardTitle } from "@kandev/ui/card";
import { UnsavedSaveButton } from "@/components/settings/unsaved-indicator";
import { SettingsCard } from "@/components/settings/settings-card";
import { ProfileFormFields } from "@/components/settings/profile-form-fields";
import { ProfileEnvVarsSection } from "@/components/settings/agent-profile-page";
import { CustomCLIFlagsCard } from "@/components/settings/cli-flags-field";
import type { Agent, ModelConfig, PermissionSetting, PassthroughConfig } from "@/lib/types/http";
import { ProfileMcpConfigCard } from "./profile-mcp-config-card";
import { profilePermissionValues } from "@/lib/agent-permissions";
import { toAgentProfilePatch, type DraftProfile, type DraftAgent } from "./agent-save-helpers";

export type AgentHeaderProps = {
  displayName: string;
  matchedPath: string | null | undefined;
  isCreateMode: boolean;
  savedAgent: Agent | null;
  onDelete?: () => void;
};

export function AgentHeader({
  displayName,
  matchedPath,
  isCreateMode,
  savedAgent,
  onDelete,
}: AgentHeaderProps) {
  return (
    <div className="flex items-start justify-between gap-6">
      <div>
        <div className="flex flex-wrap items-center gap-2">
          <h2 className="text-2xl font-bold">{displayName}</h2>
          <span className="text-xs text-muted-foreground border border-muted-foreground/30 rounded-full px-2 py-1">
            {matchedPath ?? "Installation not detected"}
          </span>
        </div>
        <p className="text-sm text-muted-foreground mt-1">
          {isCreateMode
            ? "Create a new profile for this agent."
            : "Configure profiles and defaults for this agent."}
        </p>
      </div>
      {savedAgent?.tui_config && onDelete && (
        <AlertDialog>
          <AlertDialogTrigger asChild>
            <Button variant="destructive" size="sm" className="cursor-pointer">
              <IconTrash className="h-4 w-4 mr-2" />
              Delete Agent
            </Button>
          </AlertDialogTrigger>
          <AlertDialogContent>
            <AlertDialogHeader>
              <AlertDialogTitle>Delete {displayName}?</AlertDialogTitle>
              <AlertDialogDescription>
                This will permanently remove the agent and all its profiles. This action cannot be
                undone.
              </AlertDialogDescription>
            </AlertDialogHeader>
            <AlertDialogFooter>
              <AlertDialogCancel className="cursor-pointer">Cancel</AlertDialogCancel>
              <AlertDialogAction onClick={onDelete} className="cursor-pointer">
                Delete
              </AlertDialogAction>
            </AlertDialogFooter>
          </AlertDialogContent>
        </AlertDialog>
      )}
    </div>
  );
}

export type ProfileCardItemProps = {
  profile: DraftProfile;
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
  const permissionValues = profilePermissionValues(profile, permissionSettings);
  return (
    <SettingsCard id={`profile-card-${profile.id}`} isDirty={isNew}>
      <CardContent className="pt-6 space-y-4">
        <ProfileFormFields
          profile={{
            name: profile.name,
            model: profile.model,
            mode: profile.mode ?? "",
            config_options: profile.configOptions ?? {},
            auto_approve: permissionValues.auto_approve,
            allow_indexing: permissionValues.allow_indexing,
            cli_passthrough: profile.cliPassthrough ?? false,
            cli_flags: profile.cliFlags ?? [],
          }}
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
          onChange={(next) => onProfileChange(profile.id, { cliFlags: next })}
          permissionSettings={permissionSettings}
        />
        <ProfileEnvVarsSection
          envVars={profile.envVars}
          onChange={(patch) => onProfileChange(profile.id, patch)}
        />
        <ProfileMcpConfigCard
          profileId={profile.id}
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
  newProfileId: string | null;
  currentAgentModelConfig: ModelConfig;
  permissionSettings: Record<string, PermissionSetting>;
  passthroughConfig: PassthroughConfig | null;
  saveStatus: "idle" | "loading" | "success" | "error";
  hasInvalidMcpConfig: boolean;
  onAddProfile: () => void;
  onProfileChange: (profileId: string, patch: Partial<DraftProfile>) => void;
  onProfileMcpChange: (
    profileId: string,
    patch: Partial<NonNullable<DraftProfile["mcp_config"]>>,
  ) => void;
  onRemoveProfile: (profileId: string) => void;
  onToastError: (error: unknown) => void;
  onSave: () => void;
};

export function ProfilesCard({
  displayName,
  isCreateMode,
  isAgentDirty,
  draftAgent,
  newProfileId,
  currentAgentModelConfig,
  permissionSettings,
  passthroughConfig,
  saveStatus,
  hasInvalidMcpConfig,
  onAddProfile,
  onProfileChange,
  onProfileMcpChange,
  onRemoveProfile,
  onToastError,
  onSave,
}: ProfilesCardProps) {
  return (
    <SettingsCard isDirty={isAgentDirty}>
      <CardHeader className="flex flex-row items-center justify-between">
        <CardTitle>
          {isCreateMode ? `Create ${displayName} Profile` : `${displayName} Profiles`}
        </CardTitle>
        <Button size="sm" variant="outline" onClick={onAddProfile} className="cursor-pointer">
          <IconPlus className="h-4 w-4 mr-2" />
          Add profile
        </Button>
      </CardHeader>
      <CardContent className="space-y-4">
        {draftAgent.profiles.map((profile) => (
          <ProfileCardItem
            key={profile.id}
            profile={profile}
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
      {isCreateMode && (
        <div className="flex justify-end px-6 pb-6">
          <UnsavedSaveButton
            isDirty={isAgentDirty}
            isLoading={saveStatus === "loading"}
            status={saveStatus}
            onClick={onSave}
            disabled={hasInvalidMcpConfig}
            cleanLabel="Create agent"
          />
        </div>
      )}
    </SettingsCard>
  );
}
