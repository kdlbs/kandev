"use client";

import { useMemo } from "react";
import { useTranslation } from "react-i18next";
import { Combobox, type ComboboxOption } from "@/components/combobox";
import { AgentLogo } from "@/components/agent-logo";
import type { AgentProfileOption } from "@/lib/state/slices/settings/types";

export const utilityProfileEligibility = (profile: AgentProfileOption) =>
  profile.enabled !== false && !profile.cli_passthrough && !profile.workspace_id;

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

function ProfileLabel({ profile, label }: { profile: AgentProfileOption; label: string }) {
  return (
    <span className="flex min-w-0 items-center gap-2">
      <span data-testid="utility-profile-agent-icon" className="shrink-0">
        <AgentLogo agentName={profile.agent_name} size={16} className="shrink-0" />
      </span>
      <span className="truncate">{label}</span>
    </span>
  );
}

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
  const { t } = useTranslation();
  const selectableProfiles = useMemo(
    () =>
      profiles.filter((profile) => includeWorkspaceProfiles || utilityProfileEligibility(profile)),
    [includeWorkspaceProfiles, profiles],
  );
  const selectedProfile = selectableProfiles.find((profile) => profile.id === value);
  const unavailableId = unavailableValue ?? value;
  const hasUnavailableValue = Boolean(
    unavailableId && unavailableId !== fallback?.value && !selectedProfile,
  );

  const options = useMemo<ComboboxOption[]>(() => {
    const result: ComboboxOption[] = [];
    if (fallback) result.push({ value: fallback.value, label: fallback.label });
    if (hasUnavailableValue) {
      result.push({
        value: unavailableId,
        label: unavailableLabel ?? t("settings:utilityUnavailableProfile", { name: unavailableId }),
        disabled: true,
        keywords: [unavailableId],
      });
    }
    result.push(
      ...selectableProfiles.map((profile) => ({
        value: profile.id,
        label: profile.label || profile.id,
        keywords: [profile.label, profile.agent_name, profile.id],
        renderLabel: () => <ProfileLabel profile={profile} label={profile.label || profile.id} />,
      })),
    );
    return result;
  }, [fallback, hasUnavailableValue, selectableProfiles, t, unavailableId, unavailableLabel]);

  const resolvedValue = value || fallback?.value || "";
  return (
    <Combobox
      options={options}
      value={resolvedValue}
      onValueChange={(nextValue) => {
        if (nextValue === fallback?.value) {
          onValueChange("");
          return;
        }
        if (!nextValue && value && value !== fallback?.value) {
          onValueChange(value);
          return;
        }
        onValueChange(nextValue);
      }}
      placeholder={t("settings:utilitySelectProfile")}
      searchPlaceholder={t("settings:utilitySearchProfiles")}
      emptyMessage={t("settings:utilityNoProfilesFound")}
      ariaLabel={t("settings:utilityAgentProfile")}
      testId={testId}
      dropdownTestId={`${testId}-dropdown`}
      triggerClassName={triggerClassName}
      className="max-h-[min(60vh,24rem)]"
    />
  );
}
