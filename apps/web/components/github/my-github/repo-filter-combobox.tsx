"use client";

import { useMemo } from "react";
import { Combobox, type ComboboxOption } from "@/components/combobox";
import { useTranslation } from "react-i18next";
import { t } from "@/lib/i18n";

const ALL_REPOS = "__all__";

type RepoFilterComboboxProps = {
  repoFilter: string;
  onRepoFilterChange: (value: string) => void;
  repoOptions: string[];
  ariaLabel: string;
  testId: string;
  dropdownTestId: string;
  triggerClassName?: string;
  className?: string;
};

function buildRepoFilterOptions(repoOptions: string[]): ComboboxOption[] {
  return [
    { value: ALL_REPOS, label: t("github:allRepos"), keywords: ["all", "repositories", "repos"] },
    ...repoOptions.map((repo) => ({ value: repo, label: repo, keywords: [repo] })),
  ];
}

export function RepoFilterCombobox({
  repoFilter,
  onRepoFilterChange,
  repoOptions,
  ariaLabel,
  testId,
  dropdownTestId,
  triggerClassName,
  className,
}: RepoFilterComboboxProps) {
  const { t } = useTranslation();
  const options = useMemo(() => buildRepoFilterOptions(repoOptions), [repoOptions]);

  return (
    <Combobox
      value={repoFilter || ALL_REPOS}
      onValueChange={(value) => {
        // Combobox signals reselecting the active option with an empty value.
        if (!value) return;
        onRepoFilterChange(value === ALL_REPOS ? "" : value);
      }}
      options={options}
      ariaLabel={ariaLabel}
      placeholder={t("github:allRepos")}
      searchPlaceholder={t("github:filterRepositories")}
      emptyMessage={t("github:noRepositoriesFound")}
      triggerClassName={triggerClassName}
      className={className}
      testId={testId}
      dropdownTestId={dropdownTestId}
    />
  );
}
