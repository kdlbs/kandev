"use client";

import { useTranslation } from "react-i18next";
import { IconPencil, IconPlus, IconTrash } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { CardContent } from "@kandev/ui/card";
import type { UtilityAgent } from "@/lib/api/domains/utility-api";
import type { AgentProfileOption } from "@/lib/state/slices/settings/types";
import { SettingsCard } from "@/components/settings/settings-card";
import { useFeature } from "@/hooks/domains/features/use-feature";
import { SETTINGS_TARGETS } from "@/lib/settings-discovery/catalog/standalone";
import { isUtilityAgentDirty } from "@/components/settings/utility-dirty";
import {
  UtilityAgentProfilePicker,
  utilityProfileEligibility,
} from "@/components/settings/utility-agent-profile-picker";
import { SettingsCardHeader } from "@/components/settings/settings-card-header";
import { SettingsFieldLabel } from "@/components/settings/settings-typography";
import { settingsActionClassName } from "@/components/settings/settings-control";

export const USE_DEFAULT = "__USE_DEFAULT__";
const UNCONFIGURED = "__UNCONFIGURED__";

export type BuiltinActionProfileSelection = {
  value: string;
  unavailableValue?: string;
};

export function getBuiltinActionProfileSelection(
  agent: UtilityAgent,
): BuiltinActionProfileSelection {
  const profileId = agent.agent_profile_id || "";
  if (agent.profile_binding_state === "unconfigured") {
    const unavailableValue = profileId || UNCONFIGURED;
    return { value: unavailableValue, unavailableValue };
  }
  if (agent.profile_binding_state !== "inherit" && profileId) {
    return { value: profileId };
  }
  return { value: USE_DEFAULT };
}

export function DefaultModelSection({
  profiles,
  profileId,
  onProfileChange,
  isDirty,
}: {
  profiles: AgentProfileOption[];
  profileId: string;
  onProfileChange: (profileId: string) => void;
  isDirty: boolean;
}) {
  const { t } = useTranslation();
  const dynamicRoutingEnabled = useFeature("dynamicAgentRouting");
  const eligibleProfiles = profiles.filter((profile) =>
    utilityProfileEligibility(profile, dynamicRoutingEnabled),
  );
  const selected = eligibleProfiles.find((profile) => profile.id === profileId);
  return (
    <SettingsCard
      isDirty={isDirty}
      discoveryTargetId={SETTINGS_TARGETS.utilityDefaultModel}
      data-testid="utility-default-model-card"
    >
      <SettingsCardHeader title={t("settings:utilityDefaultModelTitle")} />
      <CardContent className="space-y-3">
        <p className="text-sm text-muted-foreground">
          {t("settings:utilityDefaultModelDescription")}
        </p>
        <div className="space-y-2">
          <SettingsFieldLabel>{t("settings:utilityAgentProfile")}</SettingsFieldLabel>
          <UtilityAgentProfilePicker
            profiles={profiles}
            value={profileId || USE_DEFAULT}
            onValueChange={(value) => onProfileChange(value === USE_DEFAULT ? "" : value)}
            fallback={{ value: USE_DEFAULT, label: t("settings:utilityNoDefaultProfile") }}
            unavailableValue={profileId && !selected ? profileId : undefined}
            testId="utility-profile-picker-default"
            triggerClassName="w-full max-w-sm font-normal"
          />
          {profileId && !selected && (
            <p className="text-xs text-destructive">{t("settings:utilityProfileNeedsRepair")}</p>
          )}
        </div>
      </CardContent>
    </SettingsCard>
  );
}

export function BuiltinActionRow({
  agent,
  profiles,
  defaultLabel,
  onProfileChange,
  onEdit,
  isDirty,
}: {
  agent: UtilityAgent;
  profiles: AgentProfileOption[];
  defaultLabel: string;
  onProfileChange: (agent: UtilityAgent, value: string) => void;
  onEdit: (agent: UtilityAgent) => void;
  isDirty: boolean;
}) {
  const { t } = useTranslation();
  const selection = getBuiltinActionProfileSelection(agent);
  return (
    <div
      className="flex flex-col gap-2 py-2 px-2 rounded hover:bg-muted/50 md:flex-row md:items-center md:gap-4"
      data-testid={`utility-action-row-${agent.id}`}
      data-settings-dirty={isDirty}
    >
      <div className="min-w-0 md:flex-1">
        <div className="text-sm font-medium leading-5 truncate">{agent.name}</div>
        <p className="text-xs text-muted-foreground truncate">{agent.description}</p>
      </div>
      <div className="flex items-center gap-2">
        <UtilityAgentProfilePicker
          profiles={profiles}
          value={selection.value}
          onValueChange={(value) => onProfileChange(agent, value)}
          fallback={{ value: USE_DEFAULT, label: defaultLabel }}
          unavailableValue={selection.unavailableValue}
          unavailableLabel={
            selection.unavailableValue ? t("settings:utilityProfileNeedsRepair") : undefined
          }
          testId={`utility-profile-picker-action-${agent.id}`}
          triggerClassName="min-w-0 flex-1 md:w-[280px] md:flex-none"
        />
        <Button
          variant="ghost"
          size="sm"
          onClick={() => onEdit(agent)}
          className="h-7 w-7 p-0 shrink-0 cursor-pointer text-muted-foreground hover:text-foreground"
        >
          <IconPencil className="h-3.5 w-3.5" />
        </Button>
      </div>
    </div>
  );
}

export function PerActionOverridesSection({
  builtins,
  profiles,
  defaultLabel,
  onProfileChange,
  onEdit,
  savedBuiltins,
}: {
  builtins: UtilityAgent[];
  profiles: AgentProfileOption[];
  defaultLabel: string;
  onProfileChange: (agent: UtilityAgent, value: string) => void;
  onEdit: (agent: UtilityAgent) => void;
  savedBuiltins: UtilityAgent[];
}) {
  const { t } = useTranslation();
  if (builtins.length === 0) return null;
  return (
    <SettingsCard
      isDirty={builtins.some((agent) =>
        isUtilityAgentDirty(
          agent,
          savedBuiltins.find((saved) => saved.id === agent.id),
        ),
      )}
      discoveryTargetId={SETTINGS_TARGETS.utilityActions}
      data-testid="utility-actions-card"
    >
      <SettingsCardHeader title={t("settings:utilityActionsTitle")} />
      <CardContent className="space-y-0">
        <p className="pb-3 text-sm text-muted-foreground">
          {t("settings:utilityActionsDescription")}
        </p>
        {builtins.map((agent) => (
          <BuiltinActionRow
            key={agent.id}
            agent={agent}
            profiles={profiles}
            defaultLabel={defaultLabel}
            onProfileChange={onProfileChange}
            onEdit={onEdit}
            isDirty={isUtilityAgentDirty(
              agent,
              savedBuiltins.find((saved) => saved.id === agent.id),
            )}
          />
        ))}
      </CardContent>
    </SettingsCard>
  );
}

export function CustomAgentRow({
  agent,
  profiles,
  onEdit,
  onDelete,
}: {
  agent: UtilityAgent;
  profiles: AgentProfileOption[];
  onEdit: (agent: UtilityAgent) => void;
  onDelete: (agent: UtilityAgent) => void;
}) {
  const { t } = useTranslation();
  const profile = profiles.find((item) => item.id === agent.agent_profile_id);
  return (
    <div className="flex flex-col gap-2 py-3 px-3 rounded hover:bg-muted/50 sm:flex-row sm:items-center sm:justify-between">
      <div className="min-w-0 flex-1">
        <div className="text-sm font-medium">{agent.name}</div>
        <p className="text-xs text-muted-foreground truncate">{agent.description}</p>
        <p className="text-xs text-muted-foreground truncate">
          {profile ? profile.label || profile.id : t("settings:utilityProfileNeedsRepair")}
        </p>
      </div>
      <div className="flex items-center gap-2">
        <Button
          variant="ghost"
          size="sm"
          onClick={() => onEdit(agent)}
          className="h-7 w-7 p-0 cursor-pointer text-muted-foreground hover:text-foreground"
        >
          <IconPencil className="h-3.5 w-3.5" />
        </Button>
        <Button
          variant="ghost"
          size="sm"
          onClick={() => onDelete(agent)}
          className="h-7 w-7 p-0 cursor-pointer text-muted-foreground hover:text-destructive"
        >
          <IconTrash className="h-3.5 w-3.5" />
        </Button>
      </div>
    </div>
  );
}

export function CustomAgentsSection({
  agents,
  profiles,
  onAdd,
  onEdit,
  onDelete,
}: {
  agents: UtilityAgent[];
  profiles: AgentProfileOption[];
  onAdd: () => void;
  onEdit: (agent: UtilityAgent) => void;
  onDelete: (agent: UtilityAgent) => void;
}) {
  const { t } = useTranslation();
  return (
    <SettingsCard
      discoveryTargetId={SETTINGS_TARGETS.utilityCustomAgents}
      data-testid="utility-custom-agents-card"
    >
      <SettingsCardHeader
        title={t("settings:utilityCustomAgentsTitle")}
        actions={
          <Button onClick={onAdd} size="sm" className={settingsActionClassName("cursor-pointer")}>
            <IconPlus className="h-4 w-4 mr-1" />
            {t("settings:utilityAddCustomAgent")}
          </Button>
        }
      />
      <CardContent className="space-y-4">
        <p className="text-sm text-muted-foreground">
          {t("settings:utilityCustomAgentsDescription")}
        </p>
        {agents.length === 0 ? (
          <p className="text-sm text-muted-foreground py-4">
            {t("settings:utilityCustomAgentsEmpty")}
          </p>
        ) : (
          <div className="space-y-2">
            {agents.map((agent) => (
              <CustomAgentRow
                key={agent.id}
                agent={agent}
                profiles={profiles}
                onEdit={onEdit}
                onDelete={onDelete}
              />
            ))}
          </div>
        )}
      </CardContent>
    </SettingsCard>
  );
}
