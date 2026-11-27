import type { ReactNode } from "react";
import { Button } from "@kandev/ui/button";
import { Input } from "@kandev/ui/input";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
import { controlSizingClassName } from "@kandev/ui/control-sizing";
import { IconSearch, IconX } from "@tabler/icons-react";
import { useTranslation } from "react-i18next";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import type { TasksListArchiveMode } from "@/lib/tasks/tasks-list-query";
import { TasksListArchiveModes } from "./tasks-list-archive-modes";
import { TasksListFiltersDrawer } from "./tasks-list-filters-drawer";

export type TasksListFilterProps = {
  searchQuery: string;
  onSearchChange: (value: string) => void;
  workflowId: string | null;
  onWorkflowChange: (value: string | null) => void;
  repositoryId: string | null;
  onRepositoryChange: (value: string | null) => void;
  workflows: ReadonlyArray<{ id: string; name: string }>;
  repositories: ReadonlyArray<{ id: string; name: string }>;
  onClearFilters: () => void;
};

export function TasksListFilters(
  props: TasksListFilterProps & {
    archiveMode: TasksListArchiveMode;
    onArchiveModeChange: (mode: TasksListArchiveMode) => void;
    displayOptions: ReactNode;
    total: number;
    isLoading: boolean;
  },
) {
  const { t } = useTranslation();
  const { isMobile } = useResponsiveBreakpoint();
  const count = Number(props.workflowId !== null) + Number(props.repositoryId !== null);
  const fields = <TasksListScopeFields {...props} />;
  return (
    <div className="space-y-3" data-testid="tasks-list-filters">
      <div className="flex flex-col gap-3 md:flex-row md:items-center">
        <TasksListArchiveModes value={props.archiveMode} onChange={props.onArchiveModeChange} />
        <div className="flex min-w-0 flex-1 items-center gap-2">
          <TaskListSearch value={props.searchQuery} onChange={props.onSearchChange} />
          {isMobile ? (
            <TasksListFiltersDrawer count={count}>{fields}</TasksListFiltersDrawer>
          ) : (
            <span className="shrink-0 text-xs tabular-nums text-muted-foreground">
              {!props.isLoading && t("tasks:filteredCount", { count: props.total })}
            </span>
          )}
        </div>
      </div>
      {!isMobile && (
        <div className="flex flex-wrap items-center justify-between gap-3">
          {fields}
          {props.displayOptions}
        </div>
      )}
      <TaskListAppliedScope {...props} />
    </div>
  );
}

function TaskListSearch({ value, onChange }: { value: string; onChange: (value: string) => void }) {
  const { t } = useTranslation();
  return (
    <div className="relative min-w-0 flex-1">
      <IconSearch
        className="pointer-events-none absolute left-2.5 top-1/2 size-4 -translate-y-1/2 text-muted-foreground"
        aria-hidden="true"
      />
      <Input
        value={value}
        onChange={(event) => onChange(event.target.value)}
        placeholder={t("kanban:searchTasksPlaceholder")}
        aria-label={t("kanban:searchTasksPlaceholder")}
        data-testid="tasks-list-search"
        className={controlSizingClassName("standard", "pl-8 pr-11")}
      />
      {value && (
        <Button
          variant="ghost"
          size="icon"
          onClick={() => onChange("")}
          aria-label={t("tasks:clearSearch")}
          className="absolute right-0 top-0 size-11 cursor-pointer md:size-7 [@media(pointer:coarse)]:size-11"
        >
          <IconX className="size-4" aria-hidden="true" />
        </Button>
      )}
    </div>
  );
}

function TasksListScopeFields(props: TasksListFilterProps) {
  const { t } = useTranslation();
  const hasFilters = Boolean(
    props.searchQuery || props.workflowId !== null || props.repositoryId !== null,
  );
  return (
    <div className="flex flex-col gap-3 md:flex-row md:flex-wrap md:items-end">
      <TaskListScopeSelect
        label={t("kanban:workflow")}
        value={props.workflowId}
        onChange={props.onWorkflowChange}
        options={props.workflows}
        allLabel={t("kanban:allWorkflowsTitleCase")}
        unavailableLabel={t("tasks:workflowUnavailable")}
        testId="tasks-list-workflow-filter"
      />
      <TaskListScopeSelect
        label={t("kanban:repository")}
        value={props.repositoryId}
        onChange={props.onRepositoryChange}
        options={props.repositories}
        allLabel={t("kanban:allRepositories")}
        unavailableLabel={t("tasks:repositoryUnavailable")}
        testId="tasks-list-repository-filter"
      />
      {hasFilters && (
        <Button
          variant="ghost"
          onClick={props.onClearFilters}
          className={controlSizingClassName(
            "standard",
            "cursor-pointer justify-start md:justify-center",
          )}
          data-testid="tasks-list-clear-filters"
        >
          {t("tasks:clearFilters")}
        </Button>
      )}
    </div>
  );
}

function TaskListScopeSelect({
  label,
  value,
  onChange,
  options,
  allLabel,
  unavailableLabel,
  testId,
}: {
  label: string;
  value: string | null;
  onChange: (value: string | null) => void;
  options: ReadonlyArray<{ id: string; name: string }>;
  allLabel: string;
  unavailableLabel: string;
  testId: string;
}) {
  return (
    <div className="min-w-0 space-y-1 md:w-44">
      <label className="text-xs text-muted-foreground" id={`${testId}-label`}>
        {label}
      </label>
      <Select
        value={value ?? "all"}
        onValueChange={(next) => onChange(next === "all" ? null : next)}
      >
        <SelectTrigger
          aria-labelledby={`${testId}-label`}
          data-testid={testId}
          className={controlSizingClassName("standard", "w-full cursor-pointer")}
        >
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          <SelectItem value="all">{allLabel}</SelectItem>
          {value && !options.some((item) => item.id === value) && (
            <SelectItem value={value}>{unavailableLabel}</SelectItem>
          )}
          {options.map((item) => (
            <SelectItem key={item.id} value={item.id}>
              {item.name}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
    </div>
  );
}

function TaskListAppliedScope(props: TasksListFilterProps) {
  const { t } = useTranslation();
  return (
    <div className="flex flex-wrap gap-2 empty:hidden" data-testid="tasks-list-applied-filters">
      {props.workflowId !== null && (
        <Button
          variant="outline"
          onClick={() => props.onWorkflowChange(null)}
          aria-label={t("tasks:removeWorkflowFilter")}
          className={controlSizingClassName("standard", "max-w-full cursor-pointer gap-2 text-xs")}
        >
          <span className="truncate">
            {t("tasks:appliedWorkflow", {
              name:
                props.workflows.find((w) => w.id === props.workflowId)?.name ??
                t("tasks:workflowUnavailable"),
            })}
          </span>
          <IconX className="size-3 shrink-0" aria-hidden="true" />
        </Button>
      )}
      {props.repositoryId !== null && (
        <Button
          variant="outline"
          onClick={() => props.onRepositoryChange(null)}
          aria-label={t("tasks:removeRepositoryFilter")}
          className={controlSizingClassName("standard", "max-w-full cursor-pointer gap-2 text-xs")}
        >
          <span className="truncate">
            {t("tasks:appliedRepository", {
              name:
                props.repositories.find((r) => r.id === props.repositoryId)?.name ??
                t("tasks:repositoryUnavailable"),
            })}
          </span>
          <IconX className="size-3 shrink-0" aria-hidden="true" />
        </Button>
      )}
    </div>
  );
}
