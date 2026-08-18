"use client";

import { useMemo } from "react";
import { Combobox, type ComboboxOption } from "@/components/combobox";
import { useTranslation } from "react-i18next";

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
  allLabel,
  searchPlaceholder,
  emptyMessage,
  testId,
  dropdownTestId,
  triggerClassName,
  className,
}: IntegrationRepositoryFilterProps) {
  const { t } = useTranslation();
  // Resolved here rather than as parameter defaults: a destructuring default is
  // evaluated before the component body runs, so `t` is not yet in scope there.
  const allRepositoriesLabel = allLabel ?? t("integrations:allRepositories");
  const searchPlaceholderText = searchPlaceholder ?? t("integrations:filterRepositories");
  const emptyMessageText = emptyMessage ?? t("integrations:noRepositoriesFound");
  const comboboxOptions = useMemo<ComboboxOption[]>(
    () => [
      {
        value: ALL_REPOSITORIES,
        label: allRepositoriesLabel,
        keywords: ["all", "repositories", "repos"],
      },
      ...options.map((option) => ({
        value: option.value,
        label: option.label,
        keywords: option.keywords ?? [option.label],
      })),
    ],
    [allRepositoriesLabel, options],
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
      placeholder={allRepositoriesLabel}
      searchPlaceholder={searchPlaceholderText}
      emptyMessage={emptyMessageText}
      triggerClassName={triggerClassName}
      className={className}
      testId={testId}
      dropdownTestId={dropdownTestId}
    />
  );
}
