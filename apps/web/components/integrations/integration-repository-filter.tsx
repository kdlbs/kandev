"use client";

import { useMemo } from "react";
import { Combobox, type ComboboxOption } from "@/components/combobox";

const ALL_REPOSITORIES = "__all_repositories__";

export type IntegrationRepositoryFilterOption = {
  value: string;
  label: string;
  keywords?: string[];
};

export type IntegrationRepositoryFilterProps = {
  value: string;
  onValueChange: (value: string) => void;
  options: IntegrationRepositoryFilterOption[];
  ariaLabel: string;
  allLabel?: string;
  searchPlaceholder?: string;
  emptyMessage?: string;
  testId?: string;
  dropdownTestId?: string;
  triggerClassName?: string;
  className?: string;
};

export function IntegrationRepositoryFilter({
  value,
  onValueChange,
  options,
  ariaLabel,
  allLabel = "All repositories",
  searchPlaceholder = "Filter repositories...",
  emptyMessage = "No repositories found.",
  testId,
  dropdownTestId,
  triggerClassName,
  className,
}: IntegrationRepositoryFilterProps) {
  const comboboxOptions = useMemo<ComboboxOption[]>(
    () => [
      {
        value: ALL_REPOSITORIES,
        label: allLabel,
        keywords: ["all", "repositories", "repos"],
      },
      ...options.map((option) => ({
        value: option.value,
        label: option.label,
        keywords: option.keywords ?? [option.label],
      })),
    ],
    [allLabel, options],
  );

  return (
    <Combobox
      value={value || ALL_REPOSITORIES}
      onValueChange={(next) => {
        // Combobox reports an active-option reselect as an empty value.
        if (!next) return;
        onValueChange(next === ALL_REPOSITORIES ? "" : next);
      }}
      options={comboboxOptions}
      ariaLabel={ariaLabel}
      placeholder={allLabel}
      searchPlaceholder={searchPlaceholder}
      emptyMessage={emptyMessage}
      triggerClassName={triggerClassName}
      className={className}
      testId={testId}
      dropdownTestId={dropdownTestId}
    />
  );
}
