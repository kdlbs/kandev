"use client";

import { useMemo, useState } from "react";
import { AgentLogo } from "@/components/agent-logo";
import {
  ContextMenuItem,
  ContextMenuSub,
  ContextMenuSubContent,
  ContextMenuSubTrigger,
} from "@kandev/ui/context-menu";
import {
  DropdownMenuItem,
  DropdownMenuSub,
  DropdownMenuSubContent,
  DropdownMenuSubTrigger,
} from "@kandev/ui/dropdown-menu";
import { useAppStore } from "@/components/state-provider";
import { isHealthyAgentProfile } from "@/hooks/domains/settings/use-healthy-agent-profiles";
import { useRemoteAuthSpecs } from "@/hooks/domains/settings/use-remote-auth-specs";
import { useTaskExecutorProfile } from "@/hooks/domains/session/use-task-executor-profile";
import { useFeature } from "@/hooks/domains/features/use-feature";
import {
  isAgentConfiguredOnExecutor,
  shouldFilterHandoffByHostHealth,
} from "@/lib/agent-executor-compat";
import type { AgentProfileOption } from "@/lib/state/slices";
import { isSelectableAgentProfile } from "@/lib/state/slices/settings/types";
import { useTranslation } from "react-i18next";

export type HandoffProfile = {
  id: string;
  label: string;
  agentName?: string;
  disabled: boolean;
};

function profileDisplayLabel(profile: AgentProfileOption): { label: string; agentName: string } {
  const parts = profile.label.split(" \u2022 ");
  const agentLabel =
    parts.length > 1 ? parts.slice(1).join(" \u2022 ") : (parts[0] ?? profile.label);
  return {
    label: agentLabel,
    agentName: profile.agent_name,
  };
}

export function useHandoffProfiles(taskId: string, enabled = true): HandoffProfile[] {
  const agentProfiles = useAppStore((s) => s.agentProfiles.items);
  const executorProfile = useTaskExecutorProfile(taskId, enabled);
  const dynamicRoutingEnabled = useFeature("dynamicAgentRouting");
  const { specs: authSpecs, loaded: authLoaded } = useRemoteAuthSpecs();

  return useMemo(() => {
    const filterByHostHealth = shouldFilterHandoffByHostHealth(executorProfile);
    return agentProfiles
      .filter((profile) => isSelectableAgentProfile(profile, dynamicRoutingEnabled))
      .filter((profile) => !filterByHostHealth || isHealthyAgentProfile(profile))
      .map((profile) => {
        const { label, agentName } = profileDisplayLabel(profile);
        let disabled = false;
        if (executorProfile && authLoaded) {
          disabled = !isAgentConfiguredOnExecutor(profile, executorProfile, authSpecs);
        }
        return { id: profile.id, label, agentName, disabled };
      });
  }, [agentProfiles, dynamicRoutingEnabled, executorProfile, authSpecs, authLoaded]);
}

/**
 * True when at least one selectable (enabled) agent profile is configured,
 * regardless of its agent's capability health. Distinguishes "no profiles
 * configured" from "profiles exist but every agent is currently unhealthy" so
 * the Handoff empty state can point at the actual remedy (agent install/auth)
 * instead of profile creation.
 */
export function useHasSelectableAgentProfiles(): boolean {
  const agentProfiles = useAppStore((s) => s.agentProfiles.items);
  const dynamicRoutingEnabled = useFeature("dynamicAgentRouting");
  return useMemo(
    () => agentProfiles.some((profile) => isSelectableAgentProfile(profile, dynamicRoutingEnabled)),
    [agentProfiles, dynamicRoutingEnabled],
  );
}

function HandoffProfileList({
  profiles,
  hasSelectableProfiles,
  onSelectProfile,
  Item,
}: {
  profiles: HandoffProfile[];
  hasSelectableProfiles: boolean;
  onSelectProfile: (profileId: string) => void;
  Item: typeof ContextMenuItem | typeof DropdownMenuItem;
}) {
  const { t } = useTranslation();
  if (profiles.length === 0) {
    const emptyMessageKey = hasSelectableProfiles
      ? "task:noAgentProfilesReadyForHandoff"
      : "task:noAgentProfilesConfigured";
    return (
      <Item disabled className="text-xs text-muted-foreground">
        {t(emptyMessageKey)}
      </Item>
    );
  }
  return profiles.map((profile) => (
    <Item
      key={profile.id}
      className="cursor-pointer"
      disabled={profile.disabled}
      title={profile.disabled ? t("task:notConfiguredForThisExecutor") : undefined}
      data-testid={`handoff-profile-${profile.id}`}
      onSelect={() => onSelectProfile(profile.id)}
    >
      <span className="inline-flex items-center gap-1.5">
        {profile.agentName && (
          <AgentLogo agentName={profile.agentName} size={14} className="shrink-0" />
        )}
        {profile.label}
      </span>
    </Item>
  ));
}

type HandoffMenuProps = {
  taskId: string;
  disabled?: boolean;
  onSelectProfile: (profileId: string) => void;
};

export function HandoffContextMenuSub({ taskId, disabled, onSelectProfile }: HandoffMenuProps) {
  const { t } = useTranslation();
  const [submenuOpen, setSubmenuOpen] = useState(false);
  const profiles = useHandoffProfiles(taskId, submenuOpen);
  const hasSelectableProfiles = useHasSelectableAgentProfiles();
  const submenuDisabled = disabled || (profiles.length > 0 && profiles.every((p) => p.disabled));

  return (
    <ContextMenuSub open={submenuOpen} onOpenChange={setSubmenuOpen}>
      <ContextMenuSubTrigger
        className="cursor-pointer"
        disabled={submenuDisabled}
        data-testid="session-handoff-submenu"
      >
        {t("task:handoff")}
      </ContextMenuSubTrigger>
      <ContextMenuSubContent className="w-48">
        <HandoffProfileList
          profiles={profiles}
          hasSelectableProfiles={hasSelectableProfiles}
          onSelectProfile={onSelectProfile}
          Item={ContextMenuItem}
        />
      </ContextMenuSubContent>
    </ContextMenuSub>
  );
}

export function HandoffDropdownMenuSub({ taskId, disabled, onSelectProfile }: HandoffMenuProps) {
  const { t } = useTranslation();
  const [submenuOpen, setSubmenuOpen] = useState(false);
  const profiles = useHandoffProfiles(taskId, submenuOpen);
  const hasSelectableProfiles = useHasSelectableAgentProfiles();
  const submenuDisabled = disabled || (profiles.length > 0 && profiles.every((p) => p.disabled));

  return (
    <DropdownMenuSub open={submenuOpen} onOpenChange={setSubmenuOpen}>
      <DropdownMenuSubTrigger
        className="cursor-pointer"
        disabled={submenuDisabled}
        data-testid="session-handoff-submenu"
      >
        {t("task:handoff")}
      </DropdownMenuSubTrigger>
      <DropdownMenuSubContent className="w-48">
        <HandoffProfileList
          profiles={profiles}
          hasSelectableProfiles={hasSelectableProfiles}
          onSelectProfile={onSelectProfile}
          Item={DropdownMenuItem}
        />
      </DropdownMenuSubContent>
    </DropdownMenuSub>
  );
}
