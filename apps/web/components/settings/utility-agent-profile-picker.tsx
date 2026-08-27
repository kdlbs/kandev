"use client";

import { useFeature } from "@/hooks/domains/features/use-feature";
import type { AgentProfileOption } from "@/lib/state/slices/settings/types";
import { AgentProfilePicker } from "@/components/settings/agent-profile-picker";

export const utilityProfileEligibility = (
  profile: AgentProfileOption,
  dynamicRoutingEnabled = true,
  includeWorkspaceProfiles = false,
) =>
  profile.enabled !== false &&
  (dynamicRoutingEnabled || profile.kind !== "dynamic") &&
  (includeWorkspaceProfiles || (!profile.cli_passthrough && !profile.workspace_id));

type UtilityAgentProfilePickerProps = {
  profiles: AgentProfileOption[];
  value: string;
  onValueChange: (value: string) => void;
  testId: string;
  fallback?: { value: string; label: string };
  unavailableValue?: string;
  unavailableLabel?: string;
  triggerClassName?: string;
  includeWorkspaceProfiles?: boolean;
};

export function UtilityAgentProfilePicker({
  profiles,
  value,
  onValueChange,
  testId,
  fallback,
  unavailableValue,
  unavailableLabel,
  triggerClassName,
  includeWorkspaceProfiles = false,
}: UtilityAgentProfilePickerProps) {
  const dynamicRoutingEnabled = useFeature("dynamicAgentRouting");
  return (
    <AgentProfilePicker
      profiles={profiles}
      value={value}
      onValueChange={onValueChange}
      testId={testId}
      fallback={fallback}
      unavailableValue={unavailableValue}
      unavailableLabel={unavailableLabel}
      triggerClassName={triggerClassName}
      iconTestId="utility-profile-agent-icon"
      profileFilter={(profile) =>
        utilityProfileEligibility(profile, dynamicRoutingEnabled, includeWorkspaceProfiles)
      }
    />
  );
}
