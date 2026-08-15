"use client";

import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
import { useTranslation } from "react-i18next";
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
  const { t } = useTranslation();
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
            <SelectValue placeholder={t("gitlab:allProjects")} />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value={ALL_PROJECTS} className="cursor-pointer">
              {t("gitlab:allProjects")}
            </SelectItem>
            {projectOptions.map((key) => (
              <SelectItem key={key} value={key} className="cursor-pointer">
                {key}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      }
      queryPlaceholder={t("gitlab:customQueryPressEnterEG")}
      titleTestId="gitlab-list-toolbar-title"
      queryTestId="gitlab-list-toolbar-custom-query"
      refreshTestId="gitlab-list-toolbar-refresh"
    />
  );
}
