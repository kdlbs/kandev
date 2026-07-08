"use client";

import { useMemo } from "react";
import { Button } from "@kandev/ui/button";
import { CliProfileEditor } from "@/components/agent/cli-profile-editor";
import { useAppStoreApi } from "@/components/state-provider";
import { getCapabilityWarning } from "@/lib/capability-warning";
import type { AgentProfileOption } from "@/lib/state/slices/settings/types";
import { toAgentProfileOption } from "@/lib/state/slices/settings/types";
import type { Tier } from "@/lib/state/slices/office/types";
import { useAgentProfileOptions } from "@/components/task-create-dialog-options";

export type ProfileSetupChange = {
  agentProfileId?: string;
  tierProfileIds?: Partial<Record<Tier, string>>;
};

export function fillMissingTierProfileIds(
  current: Partial<Record<Tier, string>>,
  profileId: string,
): Partial<Record<Tier, string>> {
  return {
    frontier: current.frontier || profileId,
    balanced: current.balanced || profileId,
    economy: current.economy || profileId,
  };
}

export function sortProfiles(profiles: AgentProfileOption[]): AgentProfileOption[] {
  return [...profiles].sort((a, b) => {
    const aDisabled =
      a.cli_passthrough || !!getCapabilityWarning(a.capability_status, a.capability_error);
    const bDisabled =
      b.cli_passthrough || !!getCapabilityWarning(b.capability_status, b.capability_error);
    if (aDisabled === bDisabled) return 0;
    return aDisabled ? 1 : -1;
  });
}

function upsertProfileOption(
  profiles: AgentProfileOption[],
  option: AgentProfileOption,
): AgentProfileOption[] {
  return [...profiles.filter((p) => p.id !== option.id), option];
}

export function useSelectableProfileOptions(agentProfiles: AgentProfileOption[]) {
  const sortedProfiles = useMemo(() => sortProfiles(agentProfiles), [agentProfiles]);
  const baseOptions = useAgentProfileOptions(sortedProfiles);
  const profileOptions = useMemo(
    () =>
      baseOptions.map((opt, i) => ({
        ...opt,
        disabled:
          sortedProfiles[i]?.cli_passthrough ||
          !!getCapabilityWarning(
            sortedProfiles[i]?.capability_status,
            sortedProfiles[i]?.capability_error,
          ),
      })),
    [baseOptions, sortedProfiles],
  );
  return { sortedProfiles, profileOptions };
}

export function CreateProfilePanel({
  settingsAgents,
  wizardProfiles,
  canCancel,
  setAgentProfiles,
  onAgentProfilesChange,
  onProfileSaved,
  onClose,
}: {
  settingsAgents: { id: string; name: string }[];
  wizardProfiles: AgentProfileOption[];
  canCancel: boolean;
  setAgentProfiles: (profiles: AgentProfileOption[]) => void;
  onAgentProfilesChange?: (profiles: AgentProfileOption[]) => void;
  onProfileSaved: (profileId: string) => void;
  onClose: () => void;
}) {
  const store = useAppStoreApi();
  return (
    <div className="mt-2 rounded-md border bg-muted/30 p-3">
      <CliProfileEditor
        mode="create"
        defaultProfileName="default"
        showAdvanced
        allowCliPassthrough={false}
        onSaved={(saved) => {
          const agentForProfile = settingsAgents.find((a) => a.id === saved.agentId) ?? {
            id: saved.agentId ?? "",
            name: saved.agentId ?? "",
          };
          const option = toAgentProfileOption(agentForProfile, saved);
          const nextStoreProfiles = upsertProfileOption(
            store.getState().agentProfiles.items,
            option,
          );
          setAgentProfiles(nextStoreProfiles);
          onAgentProfilesChange?.(upsertProfileOption(wizardProfiles, option));
          onProfileSaved(saved.id);
          onClose();
        }}
        onCancel={canCancel ? onClose : undefined}
      />
    </div>
  );
}

export function CreateProfileButton({
  hasProfiles,
  onCreateClick,
}: {
  hasProfiles: boolean;
  onCreateClick: () => void;
}) {
  return (
    <Button
      type="button"
      variant="link"
      onClick={onCreateClick}
      className="h-auto p-0 cursor-pointer text-primary"
    >
      {hasProfiles ? "+ Create a new CLI profile" : "Create one inline"}
    </Button>
  );
}
