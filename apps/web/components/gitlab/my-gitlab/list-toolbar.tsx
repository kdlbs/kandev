"use client";

import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
import { IntegrationListToolbar } from "@/components/integrations/integration-list-toolbar";

const ALL_PROJECTS = "__all__";

type ListToolbarProps = {
  title: string;
  count: number;
  loading: boolean;
  lastFetchedAt: Date | null;
  customQuery: string;
  committedQuery: string;
  onCustomQueryChange: (value: string) => void;
  onCommitCustomQuery: () => void;
  projectFilter: string;
  onProjectFilterChange: (value: string) => void;
  projectOptions: string[];
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
  projectFilter,
  onProjectFilterChange,
  projectOptions,
  onRefresh,
}: ListToolbarProps) {
  const selectValue = projectFilter || ALL_PROJECTS;
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
        <Select
          value={selectValue}
          onValueChange={(value) => onProjectFilterChange(value === ALL_PROJECTS ? "" : value)}
        >
          <SelectTrigger
            className="h-8 w-full cursor-pointer md:w-[220px]"
            data-testid="gitlab-project-filter-trigger"
          >
            <SelectValue placeholder="All projects" />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value={ALL_PROJECTS} className="cursor-pointer">
              All projects
            </SelectItem>
            {projectOptions.map((key) => (
              <SelectItem key={key} value={key} className="cursor-pointer">
                {key}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      }
      queryPlaceholder='Custom query — press Enter. e.g. "labels=bug&state=opened"'
      titleTestId="gitlab-list-toolbar-title"
      queryTestId="gitlab-list-toolbar-custom-query"
      refreshTestId="gitlab-list-toolbar-refresh"
    />
  );
}
