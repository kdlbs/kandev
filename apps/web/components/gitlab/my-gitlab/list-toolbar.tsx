"use client";

import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
import { Input } from "@kandev/ui/input";
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
  showMilestoneFilter: boolean;
  milestone: string;
  committedMilestone: string;
  onMilestoneChange: (value: string) => void;
  onCommitMilestone: () => void;
};

function ProjectFilterSelect({
  projectFilter,
  onProjectFilterChange,
  projectOptions,
}: Pick<ListToolbarProps, "projectFilter" | "onProjectFilterChange" | "projectOptions">) {
  const { t } = useTranslation();
  const selectValue = projectFilter || ALL_PROJECTS;
  return (
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
  );
}

function MilestoneFilterInput({
  milestone,
  committedMilestone,
  onMilestoneChange,
  onCommitMilestone,
}: Pick<
  ListToolbarProps,
  "milestone" | "committedMilestone" | "onMilestoneChange" | "onCommitMilestone"
>) {
  const { t } = useTranslation();
  return (
    <Input
      value={milestone}
      onChange={(event) => onMilestoneChange(event.target.value)}
      onKeyDown={(event) => {
        if (event.key === "Enter" && !event.nativeEvent.isComposing && event.keyCode !== 229) {
          event.preventDefault();
          onCommitMilestone();
        }
      }}
      onBlur={() => {
        if (milestone !== committedMilestone) onCommitMilestone();
      }}
      placeholder={t("gitlab:eGSprint42")}
      aria-label={t("gitlab:milestoneFilterLabel")}
      className="h-11 w-full md:h-8 md:w-[180px]"
      data-testid="gitlab-milestone-filter"
    />
  );
}

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
  showMilestoneFilter,
  milestone,
  committedMilestone,
  onMilestoneChange,
  onCommitMilestone,
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
        <>
          <ProjectFilterSelect
            projectFilter={projectFilter}
            onProjectFilterChange={onProjectFilterChange}
            projectOptions={projectOptions}
          />
          {showMilestoneFilter ? (
            <MilestoneFilterInput
              milestone={milestone}
              committedMilestone={committedMilestone}
              onMilestoneChange={onMilestoneChange}
              onCommitMilestone={onCommitMilestone}
            />
          ) : null}
        </>
      }
      queryPlaceholder={t("gitlab:customQueryPressEnterEG")}
      titleTestId="gitlab-list-toolbar-title"
      queryTestId="gitlab-list-toolbar-custom-query"
      refreshTestId="gitlab-list-toolbar-refresh"
    />
  );
}
