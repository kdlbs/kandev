"use client";

import { useMemo } from "react";
import { useTranslation } from "react-i18next";
import { Combobox, type ComboboxOption } from "@/components/combobox";
import { AgentLogo } from "@/components/agent-logo";
import { cn } from "@/lib/utils";
import type { AgentProfileOption } from "@/lib/state/slices/settings/types";

export type AgentProfilePickerProps = {
  profiles: AgentProfileOption[];
  value: string;
  onValueChange: (value: string) => void;
  testId: string;
  fallback?: { value: string; label: string };
  unavailableValue?: string;
  unavailableLabel?: string;
  triggerClassName?: string;
  placeholder?: string;
  searchPlaceholder?: string;
  emptyMessage?: string;
  ariaLabel?: string;
  profileFilter?: (profile: AgentProfileOption) => boolean;
  iconTestId?: string;
};

function ProfileLabel({
  profile,
  label,
  iconTestId,
}: {
  profile: AgentProfileOption;
  label: string;
  iconTestId: string;
}) {
  return (
    <span className="flex min-w-0 items-center gap-2">
      <span data-testid={iconTestId} className="shrink-0">
        <AgentLogo agentName={profile.agent_name} size={16} className="shrink-0" />
      </span>
      <span className="truncate">{label}</span>
    </span>
  );
}

export function AgentProfilePicker({
  profiles,
  value,
  onValueChange,
  testId,
  fallback,
  unavailableValue,
  unavailableLabel,
  triggerClassName,
  placeholder,
  searchPlaceholder,
  emptyMessage,
  ariaLabel,
  profileFilter,
  iconTestId = "agent-profile-picker-agent-icon",
}: AgentProfilePickerProps) {
  const { t } = useTranslation();
  const selectableProfiles = useMemo(
    () => profiles.filter((profile) => profileFilter?.(profile) ?? true),
    [profileFilter, profiles],
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
        renderLabel: () => (
          <ProfileLabel
            profile={profile}
            label={profile.label || profile.id}
            iconTestId={iconTestId}
          />
        ),
      })),
    );
    return result;
  }, [
    fallback,
    hasUnavailableValue,
    iconTestId,
    selectableProfiles,
    t,
    unavailableId,
    unavailableLabel,
  ]);

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
      placeholder={placeholder ?? t("settings:utilitySelectProfile")}
      searchPlaceholder={searchPlaceholder ?? t("settings:utilitySearchProfiles")}
      emptyMessage={emptyMessage ?? t("settings:utilityNoProfilesFound")}
      ariaLabel={ariaLabel ?? t("settings:utilityAgentProfile")}
      testId={testId}
      dropdownTestId={`${testId}-dropdown`}
      triggerClassName={cn(
        "border-border dark:bg-input/30 hover:bg-input/50 hover:text-foreground aria-expanded:bg-muted aria-expanded:text-foreground font-normal",
        triggerClassName,
      )}
      className="max-h-[min(60vh,24rem)]"
    />
  );
}
