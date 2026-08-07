"use client";

import { Button } from "@kandev/ui/button";
import { Checkbox } from "@kandev/ui/checkbox";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@kandev/ui/dropdown-menu";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
import { IconAdjustmentsHorizontal } from "@tabler/icons-react";
import { useKanbanDisplaySettings } from "@/hooks/use-kanban-display-settings";
import {
  pluginTaskFilterRegistrationKey,
  type PluginTaskFilterRegistration,
} from "@/lib/plugins/registry";
import type { Repository } from "@/lib/types/http";
import type { WorkflowsState } from "@/lib/state/slices";
import { useMemo, type ComponentProps } from "react";
import { useTranslation } from "react-i18next";
import { getRepositoryPlaceholderKey } from "@/lib/kanban/repository-placeholder";

type KanbanDisplayDropdownProps = {
  triggerSize?: ComponentProps<typeof Button>["size"];
  currentPage?: "kanban" | "tasks";
  /** Plugin-registered task filters (`registerTaskFilter`) rendered as extra sections. */
  pluginFilters?: PluginTaskFilterRegistration[];
  pluginFilterSelections?: Record<string, string[]>;
  onPluginFilterChange?: (filterId: string, values: string[]) => void;
};

function WorkflowSection({
  activeWorkflowId,
  workflows,
  onWorkflowChange,
}: {
  activeWorkflowId: string | null;
  workflows: WorkflowsState["items"];
  onWorkflowChange: (id: string | null) => void;
}) {
  const { t } = useTranslation();
  return (
    <div className="space-y-1.5">
      <DropdownMenuLabel className="px-0 text-foreground">{t("kanban:workflow")}</DropdownMenuLabel>
      <Select
        value={activeWorkflowId ?? "all"}
        onValueChange={(value) => onWorkflowChange(value === "all" ? null : value)}
      >
        <SelectTrigger data-testid="display-workflow-filter" className="w-full border-border">
          <SelectValue placeholder={t("kanban:selectWorkflow")} />
        </SelectTrigger>
        <SelectContent>
          <SelectItem value="all">{t("kanban:allWorkflowsTitleCase")}</SelectItem>
          {workflows.map((workflow: WorkflowsState["items"][number]) => (
            <SelectItem key={workflow.id} value={workflow.id}>
              {workflow.name}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
    </div>
  );
}

function RepositorySection({
  repositoryValue,
  repositories,
  repositoriesLoading,
  onRepositoryChange,
}: {
  repositoryValue: string;
  repositories: Repository[];
  repositoriesLoading: boolean;
  onRepositoryChange: (value: string | "all") => void;
}) {
  const { t } = useTranslation();
  return (
    <div className="space-y-1.5">
      <DropdownMenuLabel className="px-0 text-foreground">
        {t("kanban:repository")}
      </DropdownMenuLabel>
      <Select
        value={repositoryValue}
        onValueChange={(value) => onRepositoryChange(value as string | "all")}
        disabled={repositories.length === 0}
      >
        <SelectTrigger data-testid="display-repository-filter" className="w-full border-border">
          <SelectValue
            placeholder={t(
              getRepositoryPlaceholderKey(repositoriesLoading, repositories.length === 0),
            )}
          />
        </SelectTrigger>
        <SelectContent>
          <SelectItem value="all">{t("kanban:allRepositories")}</SelectItem>
          {repositories.map((repo: Repository) => (
            <SelectItem key={repo.id} value={repo.id}>
              {repo.name}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
    </div>
  );
}

function PluginFilterSection({
  filter,
  filterKey,
  selected,
  onChange,
}: {
  filter: PluginTaskFilterRegistration;
  filterKey: string;
  selected: string[];
  onChange: (values: string[]) => void;
}) {
  const options = useMemo(() => filter.getOptions(), [filter]);
  const toggleOption = (value: string, checked: boolean) => {
    onChange(checked ? [...selected, value] : selected.filter((v) => v !== value));
  };

  return (
    <div className="space-y-1.5" data-testid={`display-plugin-filter-${filterKey}`}>
      <DropdownMenuLabel className="px-0 text-foreground">{filter.label}</DropdownMenuLabel>
      <div className="space-y-1">
        {options.map((option) => (
          <label key={option.value} className="flex items-center gap-2 cursor-pointer">
            <Checkbox
              data-testid={`display-plugin-filter-${filterKey}-option-${option.value}`}
              checked={selected.includes(option.value)}
              onCheckedChange={(checked) => toggleOption(option.value, checked === true)}
            />
            {option.color && (
              <span
                className="h-2 w-2 rounded-full shrink-0"
                style={{ backgroundColor: option.color }}
              />
            )}
            <span className="text-sm text-foreground">{option.label}</span>
          </label>
        ))}
      </div>
    </div>
  );
}

function PreviewPanelSection({
  enablePreviewOnClick,
  onTogglePreviewOnClick,
}: {
  enablePreviewOnClick: boolean | undefined;
  onTogglePreviewOnClick: ((value: boolean) => void) | undefined;
}) {
  const { t } = useTranslation();
  return (
    <div className="space-y-1.5">
      <DropdownMenuLabel className="px-0 text-foreground">
        {t("kanban:previewPanel")}
      </DropdownMenuLabel>
      <label className="flex items-center gap-2 cursor-pointer">
        <Checkbox
          checked={enablePreviewOnClick ?? false}
          onCheckedChange={(checked) => {
            onTogglePreviewOnClick?.(!!checked);
          }}
        />
        <span className="text-sm text-foreground">{t("kanban:openPreviewOnClick")}</span>
      </label>
      <p className="text-xs text-muted-foreground pl-6">
        {t("kanban:whenEnabledClickingATaskOpens")}
      </p>
    </div>
  );
}

function TasksListSection({
  tasksListShowDetails,
  onToggleTasksListShowDetails,
}: {
  tasksListShowDetails: boolean;
  onToggleTasksListShowDetails: (value: boolean) => void;
}) {
  const { t } = useTranslation();
  return (
    <>
      <DropdownMenuSeparator />
      <div className="space-y-1.5">
        <DropdownMenuLabel className="px-0 text-foreground">
          {t("kanban:listRows")}
        </DropdownMenuLabel>
        <label className="flex items-center gap-2 cursor-pointer">
          <Checkbox
            checked={tasksListShowDetails}
            onCheckedChange={(checked) => onToggleTasksListShowDetails(checked === true)}
          />
          <span className="text-sm text-foreground">{t("kanban:showTaskDetails")}</span>
        </label>
        <p className="pl-6 text-xs text-muted-foreground">
          {t("kanban:addRepositoryPullRequestSessionParent")}
        </p>
      </div>
    </>
  );
}

export function KanbanDisplayDropdown({
  triggerSize = "icon",
  currentPage = "kanban",
  pluginFilters,
  pluginFilterSelections,
  onPluginFilterChange,
}: KanbanDisplayDropdownProps) {
  const {
    workflows,
    activeWorkflowId,
    repositories,
    repositoriesLoading,
    allRepositoriesSelected,
    selectedRepositoryId,
    enablePreviewOnClick,
    tasksListShowDetails,
    onWorkflowChange,
    onRepositoryChange,
    onTogglePreviewOnClick,
    onToggleTasksListShowDetails,
  } = useKanbanDisplaySettings();

  const repositoryValue = allRepositoriesSelected ? "all" : (selectedRepositoryId ?? "all");

  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button
          variant="outline"
          size={triggerSize}
          data-testid="display-button"
          className="cursor-pointer"
        >
          <IconAdjustmentsHorizontal className="h-4 w-4" />
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" className="w-[280px] p-3">
        <div className="space-y-3">
          <WorkflowSection
            activeWorkflowId={activeWorkflowId}
            workflows={workflows}
            onWorkflowChange={onWorkflowChange}
          />
          <DropdownMenuSeparator />
          <RepositorySection
            repositoryValue={repositoryValue}
            repositories={repositories}
            repositoriesLoading={repositoriesLoading}
            onRepositoryChange={onRepositoryChange}
          />
          {pluginFilters?.map((filter) => {
            const filterKey = pluginTaskFilterRegistrationKey(filter);
            return (
              <div key={filterKey} className="contents">
                <DropdownMenuSeparator />
                <PluginFilterSection
                  filter={filter}
                  filterKey={filterKey}
                  selected={pluginFilterSelections?.[filterKey] ?? []}
                  onChange={(values) => onPluginFilterChange?.(filterKey, values)}
                />
              </div>
            );
          })}
          <DropdownMenuSeparator />
          <PreviewPanelSection
            enablePreviewOnClick={enablePreviewOnClick}
            onTogglePreviewOnClick={onTogglePreviewOnClick}
          />
          {currentPage === "tasks" && (
            <TasksListSection
              tasksListShowDetails={tasksListShowDetails}
              onToggleTasksListShowDetails={onToggleTasksListShowDetails}
            />
          )}
        </div>
      </DropdownMenuContent>
    </DropdownMenu>
  );
}
