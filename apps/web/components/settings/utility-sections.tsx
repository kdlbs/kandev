"use client";

import { useTranslation } from "react-i18next";
import { IconPencil, IconPlus, IconTrash } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { CardAction, CardContent, CardHeader, CardTitle } from "@kandev/ui/card";
import { Label } from "@kandev/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
import type { UtilityAgent } from "@/lib/api/domains/utility-api";
import type { AgentProfileOption } from "@/lib/state/slices/settings/types";
import { SettingsCard } from "@/components/settings/settings-card";
import { STANDALONE_SETTINGS_TARGETS } from "@/lib/settings-discovery/catalog/standalone";
import { isUtilityAgentDirty } from "@/components/settings/utility-dirty";

export const USE_DEFAULT = "__USE_DEFAULT__";
const UNCONFIGURED = "__UNCONFIGURED__";

function profileLabel(profile: AgentProfileOption): string {
  return profile.label || profile.id;
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
  const selected = profiles.find((profile) => profile.id === profileId);
  return (
    <SettingsCard
      isDirty={isDirty}
      discoveryTargetId={STANDALONE_SETTINGS_TARGETS.utilityDefaultModel}
      data-testid="utility-default-model-card"
    >
      <CardHeader>
        <CardTitle className="text-base">
          <h3>{t("settings:utilityDefaultModelTitle")}</h3>
        </CardTitle>
      </CardHeader>
      <CardContent className="space-y-3">
        <p className="text-sm text-muted-foreground">
          {t("settings:utilityDefaultModelDescription")}
        </p>
        <div className="space-y-2">
          <Label className="text-xs text-muted-foreground">
            {t("settings:utilityAgentProfile")}
          </Label>
          <Select
            value={profileId || USE_DEFAULT}
            onValueChange={(value) => onProfileChange(value === USE_DEFAULT ? "" : value)}
          >
            <SelectTrigger className="w-full cursor-pointer" data-settings-dirty={isDirty}>
              <SelectValue placeholder={t("settings:utilitySelectProfile")} />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value={USE_DEFAULT}>{t("settings:utilityNoDefaultProfile")}</SelectItem>
              {profileId && !selected && (
                <SelectItem value={profileId}>
                  {t("settings:utilityUnavailableProfile", { name: profileId })}
                </SelectItem>
              )}
              {profiles
                .filter(
                  (profile) =>
                    profile.enabled !== false && !profile.cli_passthrough && !profile.workspace_id,
                )
                .map((profile) => (
                  <SelectItem key={profile.id} value={profile.id}>
                    {profileLabel(profile)}
                  </SelectItem>
                ))}
            </SelectContent>
          </Select>
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
  let currentValue = USE_DEFAULT;
  if (agent.profile_binding_state === "unconfigured") {
    currentValue = UNCONFIGURED;
  } else if (agent.profile_binding_state !== "inherit" && agent.agent_profile_id) {
    currentValue = agent.agent_profile_id;
  }
  return (
    <div
      className="flex flex-col gap-2 py-2 px-2 rounded hover:bg-muted/50 md:flex-row md:items-center md:gap-4"
      data-testid={`utility-action-row-${agent.id}`}
      data-settings-dirty={isDirty}
    >
      <div className="min-w-0 md:flex-1">
        <div className="text-sm font-medium truncate">{agent.name}</div>
        <p className="text-xs text-muted-foreground truncate">{agent.description}</p>
      </div>
      <div className="flex items-center gap-2">
        <Select value={currentValue} onValueChange={(value) => onProfileChange(agent, value)}>
          <SelectTrigger
            className="min-w-0 flex-1 cursor-pointer md:w-[280px] md:flex-none"
            data-settings-dirty={isDirty}
          >
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value={USE_DEFAULT}>{defaultLabel}</SelectItem>
            {currentValue === UNCONFIGURED && (
              <SelectItem value={UNCONFIGURED} disabled>
                {t("settings:utilityProfileNeedsRepair")}
              </SelectItem>
            )}
            {profiles
              .filter(
                (profile) =>
                  profile.enabled !== false && !profile.cli_passthrough && !profile.workspace_id,
              )
              .map((profile) => (
                <SelectItem key={profile.id} value={profile.id}>
                  {profileLabel(profile)}
                </SelectItem>
              ))}
          </SelectContent>
        </Select>
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
      discoveryTargetId={STANDALONE_SETTINGS_TARGETS.utilityActions}
      data-testid="utility-actions-card"
    >
      <CardHeader>
        <CardTitle className="text-base">
          <h3>{t("settings:utilityActionsTitle")}</h3>
        </CardTitle>
      </CardHeader>
      <CardContent className="space-y-0">
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
          {profile ? profileLabel(profile) : t("settings:utilityProfileNeedsRepair")}
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
      discoveryTargetId={STANDALONE_SETTINGS_TARGETS.utilityCustomAgents}
      data-testid="utility-custom-agents-card"
    >
      <CardHeader>
        <CardTitle className="text-base">
          <h3>{t("settings:utilityCustomAgentsTitle")}</h3>
        </CardTitle>
        <CardAction>
          <Button onClick={onAdd} size="sm" className="cursor-pointer">
            <IconPlus className="h-4 w-4 mr-1" />
            {t("settings:utilityAddCustomAgent")}
          </Button>
        </CardAction>
      </CardHeader>
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
