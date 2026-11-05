"use client";

import type { ReactNode } from "react";
import { CommandPreviewCard } from "@/app/settings/agents/[agentId]/profiles/[profileId]/command-preview-card";
import { profilePermissionValues } from "@/lib/agent-permissions";
import { agentProfileDiscoveryTarget } from "@/lib/settings-discovery/dynamic-targets";
import { useTranslation } from "react-i18next";
import { CommandPrefixField } from "./command-prefix-field";
import { SettingsGroup } from "./settings-group";
import type { ProfileFormData } from "./profile-form-fields";

export function ProfileExecutionSection({
  sandboxControl,
  profile,
  agentName,
  profileId,
  baselineProfile,
  onChange,
}: {
  sandboxControl?: ReactNode;
  profile: ProfileFormData;
  agentName: string;
  profileId?: string;
  baselineProfile?: ProfileFormData;
  onChange: (patch: Partial<ProfileFormData>) => void;
}) {
  const { t } = useTranslation();
  return (
    <SettingsGroup
      title={t("agents:executionHeading")}
      description={t("agents:executionHelp")}
      data-testid="profile-execution-section"
      collapsible={false}
    >
      <CommandPrefixField profile={profile} baselineProfile={baselineProfile} onChange={onChange} />
      {sandboxControl}
      <CommandPreviewCard
        discoveryTargetId={
          profileId ? agentProfileDiscoveryTarget(profileId, "command-preview") : undefined
        }
        agentName={agentName}
        model={profile.model}
        permissionSettings={profilePermissionValues(profile)}
        cliPassthrough={profile.cli_passthrough}
        cliFlags={profile.cli_flags}
        commandPrefix={profile.command_prefix}
        copilotSelectedPlugins={profile.copilot_selected_plugins}
      />
    </SettingsGroup>
  );
}
