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
import { useRemoteAuthSpecs } from "@/hooks/domains/settings/use-remote-auth-specs";
import { useSettingsData } from "@/hooks/domains/settings/use-settings-data";
import { useTaskExecutorProfile } from "@/hooks/domains/session/use-task-executor-profile";
import { isAgentConfiguredOnExecutor } from "@/lib/agent-executor-compat";
import type { AgentProfileOption } from "@/lib/state/slices";

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
  const { agentProfiles } = useSettingsData(enabled);
  const executorProfile = useTaskExecutorProfile(taskId, enabled);
  const { specs: authSpecs, loaded: authLoaded } = useRemoteAuthSpecs();

  return useMemo(() => {
    return agentProfiles.map((profile) => {
      const { label, agentName } = profileDisplayLabel(profile);
      let disabled = false;
      if (executorProfile && authLoaded) {
        disabled = !isAgentConfiguredOnExecutor(profile, executorProfile, authSpecs);
      }
      return { id: profile.id, label, agentName, disabled };
    });
  }, [agentProfiles, executorProfile, authSpecs, authLoaded]);
}

function HandoffProfileList({
  profiles,
  onSelectProfile,
  Item,
}: {
  profiles: HandoffProfile[];
  onSelectProfile: (profileId: string) => void;
  Item: typeof ContextMenuItem | typeof DropdownMenuItem;
}) {
  if (profiles.length === 0) {
    return (
      <Item disabled className="text-xs text-muted-foreground">
        No agent profiles configured
      </Item>
    );
  }
  return profiles.map((profile) => (
    <Item
      key={profile.id}
      className="cursor-pointer"
      disabled={profile.disabled}
      title={profile.disabled ? "Not configured for this executor" : undefined}
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
  const [submenuOpen, setSubmenuOpen] = useState(false);
  const profiles = useHandoffProfiles(taskId, submenuOpen);
  const submenuDisabled = disabled || (profiles.length > 0 && profiles.every((p) => p.disabled));

  return (
    <ContextMenuSub open={submenuOpen} onOpenChange={setSubmenuOpen}>
      <ContextMenuSubTrigger
        className="cursor-pointer"
        disabled={submenuDisabled}
        data-testid="session-handoff-submenu"
      >
        Handoff
      </ContextMenuSubTrigger>
      <ContextMenuSubContent className="w-48">
        <HandoffProfileList
          profiles={profiles}
          onSelectProfile={onSelectProfile}
          Item={ContextMenuItem}
        />
      </ContextMenuSubContent>
    </ContextMenuSub>
  );
}

export function HandoffDropdownMenuSub({ taskId, disabled, onSelectProfile }: HandoffMenuProps) {
  const [submenuOpen, setSubmenuOpen] = useState(false);
  const profiles = useHandoffProfiles(taskId, submenuOpen);
  const submenuDisabled = disabled || (profiles.length > 0 && profiles.every((p) => p.disabled));

  return (
    <DropdownMenuSub open={submenuOpen} onOpenChange={setSubmenuOpen}>
      <DropdownMenuSubTrigger
        className="cursor-pointer"
        disabled={submenuDisabled}
        data-testid="session-handoff-submenu"
      >
        Handoff
      </DropdownMenuSubTrigger>
      <DropdownMenuSubContent className="w-48">
        <HandoffProfileList
          profiles={profiles}
          onSelectProfile={onSelectProfile}
          Item={DropdownMenuItem}
        />
      </DropdownMenuSubContent>
    </DropdownMenuSub>
  );
}
