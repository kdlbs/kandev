"use client";

import { useTranslation } from "react-i18next";
import { IntegrationRepositoryFilter } from "@/components/integrations/integration-repository-filter";

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
  return (
    <IntegrationRepositoryFilter
      value={repoFilter}
      onValueChange={onRepoFilterChange}
      options={repoOptions.map((repo) => ({ value: repo, label: repo, keywords: [repo] }))}
      ariaLabel={ariaLabel}
      allLabel={t("github:allRepos")}
      searchPlaceholder={t("github:filterRepositories")}
      emptyMessage={t("github:noRepositoriesFound")}
      triggerClassName={triggerClassName}
      className={className}
      testId={testId}
      dropdownTestId={dropdownTestId}
    />
  );
}
