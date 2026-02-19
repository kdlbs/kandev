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
import { Card, CardContent, CardHeader, CardTitle } from "@kandev/ui/card";
import { UnsavedChangesBadge, UnsavedSaveButton } from "@/components/settings/unsaved-indicator";
import { ProfileFormFields } from "@/components/settings/profile-form-fields";
import type {
  Agent,
  ModelConfig,
  PermissionSetting,
  PassthroughConfig,
} from "@/lib/types/http";
import { ProfileMcpConfigCard } from "./profile-mcp-config-card";
import type { DraftProfile, DraftAgent } from "./agent-save-helpers";

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
  return (
    <Card
      id={`profile-card-${profile.id}`}
      className={isNew ? "border-amber-400/70 shadow-sm" : "border-muted"}
    >
      <CardContent className="pt-6 space-y-4">
        <ProfileFormFields
          profile={{
            name: profile.name,
            model: profile.model,
            auto_approve: profile.auto_approve,
            dangerously_skip_permissions: profile.dangerously_skip_permissions,
            allow_indexing: profile.allow_indexing ?? false,
            cli_passthrough: profile.cli_passthrough,
          }}
          onChange={(patch) => onProfileChange(profile.id, patch)}
          modelConfig={currentAgentModelConfig}
          permissionSettings={permissionSettings}
          passthroughConfig={passthroughConfig}
          agentName={draftAgent.name}
          onRemove={() => onRemoveProfile(profile.id)}
          canRemove={draftAgent.profiles.length > 1}
          lockPassthrough={Boolean(draftAgent.tui_config)}
        />
        <ProfileMcpConfigCard
          profileId={profile.id}
          supportsMcp={draftAgent.supports_mcp}
          draftState={profile.id.startsWith("draft-") ? profile.mcp_config : undefined}
          onDraftStateChange={(patch) => onProfileMcpChange(profile.id, patch)}
          onToastError={onToastError}
        />
      </CardContent>
    </Card>
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
    <Card>
      <CardHeader className="flex flex-row items-center justify-between">
        <div className="flex items-center gap-2">
          <CardTitle>
            {isCreateMode ? `Create ${displayName} Profile` : `${displayName} Profiles`}
          </CardTitle>
          {isAgentDirty && <UnsavedChangesBadge />}
        </div>
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
      <div className="flex justify-end px-6 pb-6">
        <UnsavedSaveButton
          isDirty={isAgentDirty}
          isLoading={saveStatus === "loading"}
          status={saveStatus}
          onClick={onSave}
          disabled={hasInvalidMcpConfig}
        />
      </div>
    </Card>
  );
}
