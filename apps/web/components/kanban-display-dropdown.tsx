"use client";

import { Button } from "@kandev/ui/button";
import { Checkbox } from "@kandev/ui/checkbox";
import { Popover, PopoverContent, PopoverTrigger } from "@kandev/ui/popover";
import { DropdownMenuLabel, DropdownMenuSeparator } from "@kandev/ui/dropdown-menu";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
import { IconAdjustmentsHorizontal } from "@tabler/icons-react";
import { useKanbanDisplaySettings } from "@/hooks/use-kanban-display-settings";
import {
  pluginTaskFilterRegistrationKey,
  type PluginTaskFilterRegistration,
} from "@/lib/plugins/registry";
import type { Repository, TaskPriority } from "@/lib/types/http";
import type { WorkflowsState } from "@/lib/state/slices";
import { useMemo, useRef, useState, type ComponentProps, type ReactNode } from "react";
import { useTranslation } from "react-i18next";
import { getRepositoryPlaceholderKey } from "@/lib/kanban/repository-placeholder";
import type { TaskListingPage } from "@/lib/task-listing/view-navigation";
import {
  KANBAN_SORT_OPTIONS,
  KANBAN_SORT_LABEL_KEYS,
  type KanbanSort,
} from "@/lib/kanban/kanban-sort";
import { TASK_PRIORITY_TOKENS, TASK_PRIORITY_LABEL_KEYS } from "@/lib/tasks/task-priority";
import { DisplaySettingsDisclosure } from "@/components/display-settings-disclosure";
import {
  buildDisplayBooleanSummary,
  buildDisplayFilterSummary,
  buildDisplaySortSummary,
} from "@/components/display-settings-summary";

type KanbanDisplayDropdownProps = {
  triggerSize?: ComponentProps<typeof Button>["size"];
  currentPage?: TaskListingPage;
  /** Plugin-registered task filters (`registerTaskFilter`) rendered as extra sections. */
  pluginFilters?: PluginTaskFilterRegistration[];
  pluginFilterSelections?: Record<string, string[]>;
  onPluginFilterChange?: (filterId: string, values: string[]) => void;
};

type DisplaySettingsGroup = "filters" | "sort" | "preview" | "list-rows";
type DisplaySettingsExpansion = Record<DisplaySettingsGroup, boolean>;

const COLLAPSED_DISPLAY_SETTINGS: DisplaySettingsExpansion = {
  filters: false,
  sort: false,
  preview: false,
  "list-rows": false,
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

function BoardSortSection({
  boardSort,
  onBoardSortChange,
}: {
  boardSort: KanbanSort;
  onBoardSortChange: (sort: KanbanSort) => void;
}) {
  const { t } = useTranslation();
  return (
    <div className="space-y-1.5">
      <DropdownMenuLabel className="px-0 text-foreground">
        {t("kanban:boardSort")}
      </DropdownMenuLabel>
      <Select value={boardSort} onValueChange={(value) => onBoardSortChange(value as KanbanSort)}>
        <SelectTrigger
          data-testid="display-board-sort"
          aria-label={t("kanban:boardSort")}
          className="w-full border-border"
        >
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          {KANBAN_SORT_OPTIONS.map((option) => (
            <SelectItem key={option.value} value={option.value}>
              {t(KANBAN_SORT_LABEL_KEYS[option.value])}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
    </div>
  );
}

function PriorityFilterSection({
  priorityFilterTokens,
  onPriorityFilterChange,
}: {
  priorityFilterTokens: TaskPriority[];
  onPriorityFilterChange: (token: TaskPriority) => void;
}) {
  const { t } = useTranslation();
  return (
    <div className="space-y-1.5">
      <DropdownMenuLabel className="px-0 text-foreground">
        {t("kanban:priorityFilter")}
      </DropdownMenuLabel>
      <div className="space-y-1">
        {TASK_PRIORITY_TOKENS.map((token) => (
          <label key={token} className="flex items-center gap-2 cursor-pointer">
            <Checkbox
              data-testid={`display-priority-filter-option-${token}`}
              checked={priorityFilterTokens.includes(token)}
              onCheckedChange={() => onPriorityFilterChange(token)}
            />
            <span className="text-sm text-foreground">{t(TASK_PRIORITY_LABEL_KEYS[token])}</span>
          </label>
        ))}
      </div>
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
    <div className="space-y-1.5">
      <DropdownMenuLabel className="px-0 text-foreground">{t("kanban:listRows")}</DropdownMenuLabel>
      <label className="flex items-center gap-2 cursor-pointer">
        <Checkbox
          data-testid="display-task-details-toggle"
          checked={tasksListShowDetails}
          onCheckedChange={(checked) => onToggleTasksListShowDetails(checked === true)}
        />
        <span className="text-sm text-foreground">{t("kanban:showTaskDetails")}</span>
      </label>
      <p className="pl-6 text-xs text-muted-foreground">
        {t("kanban:addRepositoryPullRequestSessionParent")}
      </p>
    </div>
  );
}

function DisplaySettingsGroup({
  group,
  title,
  summary,
  expanded,
  onExpandedChange,
  children,
}: {
  group: DisplaySettingsGroup;
  title: string;
  summary: string;
  expanded: boolean;
  onExpandedChange: (expanded: boolean) => void;
  children: ReactNode;
}) {
  return (
    <DisplaySettingsDisclosure
      testId={`display-settings-${group}`}
      title={title}
      summary={summary}
      expanded={expanded}
      onExpandedChange={onExpandedChange}
      className="px-0"
      contentClassName="space-y-3 pb-1"
    >
      {children}
    </DisplaySettingsDisclosure>
  );
}

type DropdownSectionsProps = KanbanDisplayDropdownProps &
  ReturnType<typeof useKanbanDisplaySettings> & {
    repositoryValue: string;
    expandedGroups: DisplaySettingsExpansion;
    onExpandedChange: (group: DisplaySettingsGroup, expanded: boolean) => void;
  };

function DesktopFiltersGroup({
  settings,
  expanded,
  onExpandedChange,
}: {
  settings: DropdownSectionsProps;
  expanded: boolean;
  onExpandedChange: (expanded: boolean) => void;
}) {
  const { t } = useTranslation();
  const showRepository = settings.currentPage !== "threads";
  const showPriority = settings.currentPage === "kanban";
  const visiblePluginFilters =
    settings.currentPage === "threads" ? [] : (settings.pluginFilters ?? []);
  return (
    <DisplaySettingsGroup
      group="filters"
      title={t("kanban:filters")}
      summary={buildDisplayFilterSummary({
        t,
        activeWorkflowId: settings.activeWorkflowId,
        workflows: settings.workflows,
        showWorkflow: true,
        repositoryValue: settings.repositoryValue,
        repositories: settings.repositories,
        repositoriesLoading: settings.repositoriesLoading,
        showRepository,
        priorityFilterTokens: settings.priorityFilterTokens,
        showPriority,
        pluginFilters: visiblePluginFilters,
        pluginFilterSelections: settings.pluginFilterSelections,
      })}
      expanded={expanded}
      onExpandedChange={onExpandedChange}
    >
      <WorkflowSection
        activeWorkflowId={settings.activeWorkflowId}
        workflows={settings.workflows}
        onWorkflowChange={settings.onWorkflowChange}
      />
      {showRepository && (
        <>
          <DropdownMenuSeparator />
          <RepositorySection
            repositoryValue={settings.repositoryValue}
            repositories={settings.repositories}
            repositoriesLoading={settings.repositoriesLoading}
            onRepositoryChange={settings.onRepositoryChange}
          />
          {showPriority && (
            <>
              <DropdownMenuSeparator />
              <PriorityFilterSection
                priorityFilterTokens={settings.priorityFilterTokens}
                onPriorityFilterChange={settings.onPriorityFilterChange}
              />
            </>
          )}
          {visiblePluginFilters.map((filter) => {
            const filterKey = pluginTaskFilterRegistrationKey(filter);
            return (
              <div key={filterKey} className="contents">
                <DropdownMenuSeparator />
                <PluginFilterSection
                  filter={filter}
                  filterKey={filterKey}
                  selected={settings.pluginFilterSelections?.[filterKey] ?? []}
                  onChange={(values) => settings.onPluginFilterChange?.(filterKey, values)}
                />
              </div>
            );
          })}
        </>
      )}
    </DisplaySettingsGroup>
  );
}

function DropdownSections(props: DropdownSectionsProps) {
  const { t } = useTranslation();
  const showRepository = props.currentPage !== "threads";
  const showPriority = props.currentPage === "kanban";
  return (
    <div className="space-y-3">
      <DesktopFiltersGroup
        settings={props}
        expanded={props.expandedGroups.filters}
        onExpandedChange={(expanded) => props.onExpandedChange("filters", expanded)}
      />
      {showPriority && (
        <DisplaySettingsGroup
          group="sort"
          title={t("kanban:sort")}
          summary={buildDisplaySortSummary(t, props.boardSort)}
          expanded={props.expandedGroups.sort}
          onExpandedChange={(expanded) => props.onExpandedChange("sort", expanded)}
        >
          <BoardSortSection
            boardSort={props.boardSort}
            onBoardSortChange={props.onBoardSortChange}
          />
        </DisplaySettingsGroup>
      )}
      {showRepository && (
        <DisplaySettingsGroup
          group="preview"
          title={t("kanban:previewPanel")}
          summary={buildDisplayBooleanSummary(t, props.enablePreviewOnClick)}
          expanded={props.expandedGroups.preview}
          onExpandedChange={(expanded) => props.onExpandedChange("preview", expanded)}
        >
          <PreviewPanelSection
            enablePreviewOnClick={props.enablePreviewOnClick}
            onTogglePreviewOnClick={props.onTogglePreviewOnClick}
          />
        </DisplaySettingsGroup>
      )}
      {props.currentPage === "tasks" && (
        <DisplaySettingsGroup
          group="list-rows"
          title={t("kanban:listRows")}
          summary={buildDisplayBooleanSummary(t, props.tasksListShowDetails)}
          expanded={props.expandedGroups["list-rows"]}
          onExpandedChange={(expanded) => props.onExpandedChange("list-rows", expanded)}
        >
          <TasksListSection
            tasksListShowDetails={props.tasksListShowDetails}
            onToggleTasksListShowDetails={props.onToggleTasksListShowDetails}
          />
        </DisplaySettingsGroup>
      )}
    </div>
  );
}

export function KanbanDisplayDropdown({
  triggerSize = "icon",
  currentPage = "kanban",
  pluginFilters,
  pluginFilterSelections,
  onPluginFilterChange,
}: KanbanDisplayDropdownProps) {
  const displaySettings = useKanbanDisplaySettings();
  const { allRepositoriesSelected, selectedRepositoryId } = displaySettings;
  const repositoryValue = allRepositoriesSelected ? "all" : (selectedRepositoryId ?? "all");
  const triggerRef = useRef<HTMLButtonElement>(null);
  const [open, setOpen] = useState(false);
  const [expandedGroups, setExpandedGroups] = useState(COLLAPSED_DISPLAY_SETTINGS);
  const handleOpenChange = (nextOpen: boolean) => {
    setOpen(nextOpen);
    if (!nextOpen) setExpandedGroups(COLLAPSED_DISPLAY_SETTINGS);
  };

  return (
    <Popover open={open} onOpenChange={handleOpenChange}>
      <PopoverTrigger asChild>
        <Button
          ref={triggerRef}
          variant="outline"
          size={triggerSize}
          data-testid="display-button"
          className="cursor-pointer"
        >
          <IconAdjustmentsHorizontal className="h-4 w-4" />
        </Button>
      </PopoverTrigger>
      <PopoverContent
        align="end"
        data-testid="display-settings-content"
        className="w-[280px] max-h-(--radix-popover-content-available-height) overflow-y-auto p-3"
        onPointerDownOutside={(e) => {
          const target = e.target as HTMLElement;
          if (
            triggerRef.current?.contains(target) ||
            target.closest("[data-radix-select-content]")
          ) {
            e.preventDefault();
          }
        }}
      >
        <DropdownSections
          {...displaySettings}
          currentPage={currentPage}
          pluginFilters={pluginFilters}
          pluginFilterSelections={pluginFilterSelections}
          onPluginFilterChange={onPluginFilterChange}
          repositoryValue={repositoryValue}
          expandedGroups={expandedGroups}
          onExpandedChange={(group, expanded) =>
            setExpandedGroups((current) => ({ ...current, [group]: expanded }))
          }
        />
      </PopoverContent>
    </Popover>
  );
}
