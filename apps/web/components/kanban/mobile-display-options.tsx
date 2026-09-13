"use client";

import {
  useEffect,
  useMemo,
  useState,
  type Dispatch,
  type ReactNode,
  type SetStateAction,
} from "react";
import { Checkbox } from "@kandev/ui/checkbox";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
import type { PluginTaskFilterRegistration } from "@/lib/plugins/registry";
import { pluginTaskFilterRegistrationKey } from "@/lib/plugins/registry";
import type { Repository, TaskPriority } from "@/lib/types/http";
import type { WorkflowsState } from "@/lib/state/slices";
import {
  KANBAN_SORT_OPTIONS,
  KANBAN_SORT_LABEL_KEYS,
  type KanbanSort,
} from "@/lib/kanban/kanban-sort";
import { TASK_PRIORITY_TOKENS, TASK_PRIORITY_LABEL_KEYS } from "@/lib/tasks/task-priority";
import { useTranslation } from "react-i18next";
import { getRepositoryPlaceholderKey } from "@/lib/kanban/repository-placeholder";
import { ColumnsMenu, type ColumnsMenuStep } from "./columns-menu";
import {
  MobileTasksListOptions,
  type TasksListDisplayOptions,
} from "./mobile-menu-task-list-options";
import { DisplaySettingsDisclosure } from "@/components/display-settings-disclosure";
import {
  buildDisplayBooleanSummary,
  buildDisplayFilterSummary,
  buildDisplaySortSummary,
} from "@/components/display-settings-summary";
import {
  mobileDisplayControlClass,
  mobileFieldClass,
  mobileFieldLabelClass,
  mobileSectionTitleClass,
} from "./mobile-menu-styles";

export type MobileDisplayOptionsProps = {
  activeWorkflowId: string | null;
  workflows: WorkflowsState["items"];
  onWorkflowChange: (id: string | null) => void;
  repositoryValue: string;
  repositories: Repository[];
  repositoriesLoading: boolean;
  onRepositoryChange: (value: string | "all") => void;
  enablePreviewOnClick: boolean | undefined;
  onTogglePreviewOnClick: ((checked: boolean) => void) | undefined;
  tasksListShowDetails: boolean;
  onToggleTasksListShowDetails: (checked: boolean) => void;
  showTaskDetails: boolean;
  showWorkflow: boolean;
  showRepository: boolean;
  showPreviewPanel: boolean;
  tasksListOptions?: TasksListDisplayOptions;
  /**
   * Column visibility for the workflow the phone board is focused on. Null off
   * the phone kanban, where the lane header owns the control instead.
   */
  columnsSection: MobileColumnsSection | null;
  /** Board sort and priority filter are board-only, like `columnsSection`. */
  showBoardControls: boolean;
  boardSort: KanbanSort;
  onBoardSortChange: (sort: KanbanSort) => void;
  priorityFilterTokens: TaskPriority[];
  onPriorityFilterChange: (token: TaskPriority) => void;
  pluginFilters: PluginTaskFilterRegistration[];
  pluginFilterSelections: Record<string, string[]>;
  onPluginFilterChange: (filterId: string, values: string[]) => void;
};

export type MobileColumnsSection = {
  workflowId: string;
  workflowName: string;
  steps: ColumnsMenuStep[];
  hiddenStepIds: string[];
  onToggle: (workflowId: string, stepId: string) => void;
  autoHideEmpty: boolean;
  onToggleAutoHide: (workflowId: string) => void;
};

function MobileDisplaySelects({ settings }: { settings: MobileDisplayOptionsProps }) {
  const { t } = useTranslation();
  return (
    <>
      {settings.showWorkflow && (
        <div className={mobileFieldClass}>
          <label className={mobileFieldLabelClass}>{t("kanban:workflow")}</label>
          <Select
            value={settings.activeWorkflowId ?? "all"}
            onValueChange={(value) => settings.onWorkflowChange(value === "all" ? null : value)}
          >
            <SelectTrigger
              data-testid="mobile-display-workflow-filter"
              className={mobileDisplayControlClass}
            >
              <SelectValue placeholder={t("kanban:allWorkflows")} />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="all">{t("kanban:allWorkflows")}</SelectItem>
              {settings.workflows.map((workflow: WorkflowsState["items"][number]) => (
                <SelectItem key={workflow.id} value={workflow.id}>
                  {workflow.name}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
      )}

      {settings.showRepository && (
        <div className={mobileFieldClass}>
          <label className={mobileFieldLabelClass}>{t("kanban:repository")}</label>
          <Select
            value={settings.repositoryValue}
            onValueChange={(value) => settings.onRepositoryChange(value as string | "all")}
            disabled={settings.repositories.length === 0}
          >
            <SelectTrigger
              data-testid="mobile-display-repository-filter"
              className={mobileDisplayControlClass}
            >
              <SelectValue
                placeholder={t(
                  getRepositoryPlaceholderKey(
                    settings.repositoriesLoading,
                    settings.repositories.length === 0,
                  ),
                )}
              />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="all">{t("kanban:allRepositories")}</SelectItem>
              {settings.repositories.map((repo: Repository) => (
                <SelectItem key={repo.id} value={repo.id}>
                  {repo.name}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
      )}
    </>
  );
}

function MobileBoardSortSelect({ settings }: { settings: MobileDisplayOptionsProps }) {
  const { t } = useTranslation();
  return (
    <div className={mobileFieldClass}>
      <label className={mobileFieldLabelClass}>{t("kanban:boardSort")}</label>
      <Select
        value={settings.boardSort}
        onValueChange={(value) => settings.onBoardSortChange(value as KanbanSort)}
      >
        <SelectTrigger
          data-testid="mobile-board-sort"
          aria-label={t("kanban:boardSort")}
          className={mobileDisplayControlClass}
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

function MobilePriorityFilterGroup({ settings }: { settings: MobileDisplayOptionsProps }) {
  const { t } = useTranslation();
  return (
    <div className={mobileFieldClass}>
      <label className={mobileFieldLabelClass}>{t("kanban:priorityFilter")}</label>
      <div className="space-y-1">
        {TASK_PRIORITY_TOKENS.map((token) => (
          <label
            key={token}
            className="flex min-h-11 cursor-pointer items-center gap-3 rounded-md px-0 text-sm font-medium"
          >
            <Checkbox
              data-testid={`mobile-priority-filter-option-${token}`}
              checked={settings.priorityFilterTokens.includes(token)}
              onCheckedChange={() => settings.onPriorityFilterChange(token)}
            />
            <span>{t(TASK_PRIORITY_LABEL_KEYS[token])}</span>
          </label>
        ))}
      </div>
    </div>
  );
}

function MobilePluginFilterSection({
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
    onChange(checked ? [...selected, value] : selected.filter((current) => current !== value));
  };

  return (
    <div className={mobileFieldClass} data-testid={`mobile-display-plugin-filter-${filterKey}`}>
      <label className={mobileFieldLabelClass}>{filter.label}</label>
      <div className="space-y-1">
        {options.map((option) => (
          <label
            key={option.value}
            className="flex min-h-11 cursor-pointer items-center gap-3 rounded-md px-0 text-sm font-medium"
          >
            <Checkbox
              data-testid={`mobile-display-plugin-filter-${filterKey}-option-${option.value}`}
              checked={selected.includes(option.value)}
              onCheckedChange={(checked) => toggleOption(option.value, checked === true)}
            />
            {option.color && (
              <span
                className="h-2 w-2 shrink-0 rounded-full"
                style={{ backgroundColor: option.color }}
              />
            )}
            <span>{option.label}</span>
          </label>
        ))}
      </div>
    </div>
  );
}

type MobileDisplaySettingsGroup = "filters" | "sort" | "preview" | "list-rows";
type MobileDisplaySettingsExpansion = Record<MobileDisplaySettingsGroup, boolean>;

const COLLAPSED_MOBILE_DISPLAY_SETTINGS: MobileDisplaySettingsExpansion = {
  filters: false,
  sort: false,
  preview: false,
  "list-rows": false,
};

function MobileDisplayGroup({
  group,
  title,
  summary,
  expanded,
  onExpandedChange,
  children,
}: {
  group: MobileDisplaySettingsGroup;
  title: string;
  summary: string;
  expanded: boolean;
  onExpandedChange: (expanded: boolean) => void;
  children: ReactNode;
}) {
  return (
    <DisplaySettingsDisclosure
      testId={`mobile-display-settings-${group}`}
      title={title}
      summary={summary}
      expanded={expanded}
      onExpandedChange={onExpandedChange}
      touchTargets
      className="px-0"
      contentClassName="space-y-3 pb-1"
    >
      {children}
    </DisplaySettingsDisclosure>
  );
}

type MobileDisplayGroupProps = {
  settings: MobileDisplayOptionsProps;
  expanded: boolean;
  onExpandedChange: (expanded: boolean) => void;
};

function MobileFiltersGroup({ settings, expanded, onExpandedChange }: MobileDisplayGroupProps) {
  const { t } = useTranslation();
  return (
    <MobileDisplayGroup
      group="filters"
      title={t("kanban:filters")}
      summary={buildDisplayFilterSummary({
        t,
        activeWorkflowId: settings.activeWorkflowId,
        workflows: settings.workflows,
        showWorkflow: settings.showWorkflow,
        repositoryValue: settings.repositoryValue,
        repositories: settings.repositories,
        repositoriesLoading: settings.repositoriesLoading,
        showRepository: settings.showRepository,
        priorityFilterTokens: settings.priorityFilterTokens,
        showPriority: settings.showBoardControls,
        pluginFilters: settings.pluginFilters,
        pluginFilterSelections: settings.pluginFilterSelections,
      })}
      expanded={expanded}
      onExpandedChange={onExpandedChange}
    >
      <MobileDisplaySelects settings={settings} />
      {settings.showBoardControls && <MobilePriorityFilterGroup settings={settings} />}
      {settings.pluginFilters.map((filter) => {
        const filterKey = pluginTaskFilterRegistrationKey(filter);
        return (
          <MobilePluginFilterSection
            key={filterKey}
            filter={filter}
            filterKey={filterKey}
            selected={settings.pluginFilterSelections[filterKey] ?? []}
            onChange={(values) => settings.onPluginFilterChange(filterKey, values)}
          />
        );
      })}
    </MobileDisplayGroup>
  );
}

function MobileSortGroup({ settings, expanded, onExpandedChange }: MobileDisplayGroupProps) {
  const { t } = useTranslation();
  return (
    <MobileDisplayGroup
      group="sort"
      title={t("kanban:sort")}
      summary={buildDisplaySortSummary(t, settings.boardSort)}
      expanded={expanded}
      onExpandedChange={onExpandedChange}
    >
      <MobileBoardSortSelect settings={settings} />
    </MobileDisplayGroup>
  );
}

function MobilePreviewGroup({ settings, expanded, onExpandedChange }: MobileDisplayGroupProps) {
  const { t } = useTranslation();
  return (
    <MobileDisplayGroup
      group="preview"
      title={t("kanban:previewPanel")}
      summary={buildDisplayBooleanSummary(t, settings.enablePreviewOnClick)}
      expanded={expanded}
      onExpandedChange={onExpandedChange}
    >
      <div className={mobileFieldClass}>
        <label className={mobileFieldLabelClass}>{t("kanban:previewPanel")}</label>
        <label className="flex min-h-11 cursor-pointer items-center gap-3 rounded-md px-0 text-sm font-medium">
          <Checkbox
            data-testid="mobile-display-preview-toggle"
            checked={settings.enablePreviewOnClick ?? false}
            onCheckedChange={(checked) => settings.onTogglePreviewOnClick?.(!!checked)}
          />
          <span className="text-sm">{t("kanban:openPreviewOnClick")}</span>
        </label>
      </div>
    </MobileDisplayGroup>
  );
}

function MobileListRowsGroup({ settings, expanded, onExpandedChange }: MobileDisplayGroupProps) {
  const { t } = useTranslation();
  return (
    <MobileDisplayGroup
      group="list-rows"
      title={t("kanban:listRows")}
      summary={buildDisplayBooleanSummary(t, settings.tasksListShowDetails)}
      expanded={expanded}
      onExpandedChange={onExpandedChange}
    >
      <div className={mobileFieldClass}>
        <label className={mobileFieldLabelClass}>{t("kanban:listRows")}</label>
        <label className="flex min-h-11 cursor-pointer items-center gap-3 rounded-md px-0 text-sm font-medium">
          <Checkbox
            data-testid="mobile-display-task-details-toggle"
            checked={settings.tasksListShowDetails}
            onCheckedChange={(checked) => settings.onToggleTasksListShowDetails(checked === true)}
          />
          <span>{t("kanban:showTaskDetails")}</span>
        </label>
        <p className="pl-6 text-xs text-muted-foreground">
          {t("kanban:addRepositoryPullRequestSessionParent")}
        </p>
      </div>
    </MobileDisplayGroup>
  );
}

function updateMobileDisplayGroup(
  setExpandedGroups: Dispatch<SetStateAction<MobileDisplaySettingsExpansion>>,
  group: MobileDisplaySettingsGroup,
  expanded: boolean,
) {
  setExpandedGroups((current) => ({ ...current, [group]: expanded }));
}

export function MobileDisplayOptions({
  open,
  ...settings
}: MobileDisplayOptionsProps & { open: boolean }) {
  const { t } = useTranslation();
  const [expandedGroups, setExpandedGroups] = useState(COLLAPSED_MOBILE_DISPLAY_SETTINGS);
  useEffect(() => {
    if (!open) setExpandedGroups(COLLAPSED_MOBILE_DISPLAY_SETTINGS);
  }, [open]);

  const showFilters =
    settings.showWorkflow ||
    settings.showRepository ||
    settings.showBoardControls ||
    settings.pluginFilters.length > 0;

  return (
    <div className="space-y-4">
      <label className={mobileSectionTitleClass}>{t("kanban:displayOptions")}</label>
      {showFilters && (
        <MobileFiltersGroup
          settings={settings}
          expanded={expandedGroups.filters}
          onExpandedChange={(expanded) =>
            updateMobileDisplayGroup(setExpandedGroups, "filters", expanded)
          }
        />
      )}
      {settings.showBoardControls && (
        <MobileSortGroup
          settings={settings}
          expanded={expandedGroups.sort}
          onExpandedChange={(expanded) =>
            updateMobileDisplayGroup(setExpandedGroups, "sort", expanded)
          }
        />
      )}
      {settings.showPreviewPanel && (
        <MobilePreviewGroup
          settings={settings}
          expanded={expandedGroups.preview}
          onExpandedChange={(expanded) =>
            updateMobileDisplayGroup(setExpandedGroups, "preview", expanded)
          }
        />
      )}
      {settings.showTaskDetails && (
        <MobileListRowsGroup
          settings={settings}
          expanded={expandedGroups["list-rows"]}
          onExpandedChange={(expanded) =>
            updateMobileDisplayGroup(setExpandedGroups, "list-rows", expanded)
          }
        />
      )}
      {settings.columnsSection && (
        <div className={mobileFieldClass}>
          <label className={mobileFieldLabelClass}>{t("kanban:columns")}</label>
          <ColumnsMenu {...settings.columnsSection} touchTargets />
        </div>
      )}
      {settings.tasksListOptions && <MobileTasksListOptions options={settings.tasksListOptions} />}
    </div>
  );
}
