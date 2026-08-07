"use client";

import { useTranslation } from "react-i18next";
import { IntegrationListToolbar } from "@/components/integrations/integration-list-toolbar";
import { RepoFilterCombobox } from "./repo-filter-combobox";

type ListToolbarProps = {
  title: string;
  count: number;
  loading: boolean;
  lastFetchedAt: Date | null;
  customQuery: string;
  committedQuery: string;
  onCustomQueryChange: (value: string) => void;
  onCommitCustomQuery: () => void;
  repoFilter: string;
  onRepoFilterChange: (value: string) => void;
  repoOptions: string[];
  onRefresh: () => void;
};

export function ListToolbar({
  title,
  count,
  loading,
  lastFetchedAt,
  customQuery,
  committedQuery,
  onCustomQueryChange,
  onCommitCustomQuery,
  repoFilter,
  onRepoFilterChange,
  repoOptions,
  onRefresh,
}: ListToolbarProps) {
  const { t } = useTranslation();
  return (
    <IntegrationListToolbar
      title={title}
      count={count}
      loading={loading}
      lastFetchedAt={lastFetchedAt}
      customQuery={customQuery}
      committedQuery={committedQuery}
      onCustomQueryChange={onCustomQueryChange}
      onCommitCustomQuery={onCommitCustomQuery}
      onRefresh={onRefresh}
      filter={
        <RepoFilterCombobox
          repoFilter={repoFilter}
          onRepoFilterChange={onRepoFilterChange}
          repoOptions={repoOptions}
          ariaLabel={t("github:filterGithubResultsByRepository")}
          triggerClassName="h-8 w-full border border-input bg-background px-2 py-1.5 text-xs/relaxed hover:bg-secondary/50 md:w-[220px]"
          className="md:min-w-[360px]"
          testId="github-repo-filter-trigger"
          dropdownTestId="github-repo-filter-dropdown"
        />
      }
      queryPlaceholder={t("github:customQueryPressEnterEG")}
      titleTestId="github-list-toolbar-title"
    />
  );
}
