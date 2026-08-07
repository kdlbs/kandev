"use client";

import { RepoFilterCombobox } from "./repo-filter-combobox";
import { IntegrationListToolbar } from "@/components/integrations/integration-list-toolbar";

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
          ariaLabel="Filter GitHub results by repository"
          triggerClassName="w-full md:w-[220px] h-8 border border-input bg-background hover:bg-secondary/50 px-2 py-1.5 text-xs/relaxed"
          className="md:min-w-[360px]"
          testId="github-repo-filter-trigger"
          dropdownTestId="github-repo-filter-dropdown"
        />
      }
      queryPlaceholder='Custom query — press Enter. e.g. "is:open review-requested:@me"'
      titleTestId="github-list-toolbar-title"
    />
  );
}
