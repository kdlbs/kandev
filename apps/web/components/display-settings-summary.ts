import type { TFunction } from "i18next";
import type { PluginTaskFilterRegistration } from "@/lib/plugins/registry";
import { pluginTaskFilterRegistrationKey } from "@/lib/plugins/registry";
import type { Repository, TaskPriority } from "@/lib/types/http";
import type { WorkflowsState } from "@/lib/state/slices";
import { KANBAN_SORT_LABEL_KEYS, type KanbanSort } from "@/lib/kanban/kanban-sort";
import { getRepositoryPlaceholderKey } from "@/lib/kanban/repository-placeholder";
import { TASK_PRIORITY_LABEL_KEYS, isTaskPriority } from "@/lib/tasks/task-priority";

const UNAVAILABLE_SUMMARY_KEY = "common:unavailable";

export type DisplayFilterSummaryOptions = {
  t: TFunction;
  activeWorkflowId: string | null;
  workflows: WorkflowsState["items"];
  showWorkflow: boolean;
  repositoryValue: string;
  repositories: Array<Pick<Repository, "id" | "name">>;
  repositoriesLoading: boolean;
  showRepository: boolean;
  priorityFilterTokens: TaskPriority[];
  showPriority: boolean;
  pluginFilters?: PluginTaskFilterRegistration[];
  pluginFilterSelections?: Record<string, string[]>;
};

function workflowSummary({
  t,
  activeWorkflowId,
  workflows,
}: Pick<DisplayFilterSummaryOptions, "t" | "activeWorkflowId" | "workflows">): string {
  if (!activeWorkflowId) return t("kanban:allWorkflowsTitleCase");
  return (
    workflows.find((workflow) => workflow.id === activeWorkflowId)?.name ??
    t(UNAVAILABLE_SUMMARY_KEY)
  );
}

function repositorySummary({
  t,
  repositoryValue,
  repositories,
  repositoriesLoading,
}: Pick<
  DisplayFilterSummaryOptions,
  "t" | "repositoryValue" | "repositories" | "repositoriesLoading"
>): string {
  if (repositoriesLoading && repositories.length === 0) return t("common:loading");
  if (repositoryValue === "all") {
    return repositories.length === 0
      ? t(getRepositoryPlaceholderKey(repositoriesLoading, true))
      : t("kanban:allRepositories");
  }
  return (
    repositories.find((repository) => repository.id === repositoryValue)?.name ??
    t(UNAVAILABLE_SUMMARY_KEY)
  );
}

function prioritySummary(t: TFunction, priorityFilterTokens: TaskPriority[]): string {
  if (priorityFilterTokens.length === 0) return t("kanban:allPriorities");
  return priorityFilterTokens
    .map((token) =>
      isTaskPriority(token) ? t(TASK_PRIORITY_LABEL_KEYS[token]) : t(UNAVAILABLE_SUMMARY_KEY),
    )
    .join(", ");
}

function pluginFilterSummaries(
  t: TFunction,
  pluginFilters: PluginTaskFilterRegistration[] | undefined,
  pluginFilterSelections: Record<string, string[]> | undefined,
): string[] {
  return (pluginFilters ?? []).flatMap((filter) => {
    const filterKey = pluginTaskFilterRegistrationKey(filter);
    const selected = pluginFilterSelections?.[filterKey] ?? [];
    return selected.length > 0
      ? [t("kanban:selectedFilterCount", { label: filter.label, count: selected.length })]
      : [];
  });
}

/** Builds the display group summary without owning or changing preference state. */
export function buildDisplayFilterSummary({
  t,
  activeWorkflowId,
  workflows,
  showWorkflow,
  repositoryValue,
  repositories,
  repositoriesLoading,
  showRepository,
  priorityFilterTokens,
  showPriority,
  pluginFilters,
  pluginFilterSelections,
}: DisplayFilterSummaryOptions): string {
  const parts: string[] = [];
  if (showWorkflow) parts.push(workflowSummary({ t, activeWorkflowId, workflows }));
  if (showRepository) {
    parts.push(repositorySummary({ t, repositoryValue, repositories, repositoriesLoading }));
  }
  if (showPriority) parts.push(prioritySummary(t, priorityFilterTokens));
  parts.push(...pluginFilterSummaries(t, pluginFilters, pluginFilterSelections));
  return parts.join(", ");
}

export function buildDisplaySortSummary(t: TFunction, sort: KanbanSort): string {
  const labelKey = KANBAN_SORT_LABEL_KEYS[sort];
  return labelKey ? t(labelKey) : t(UNAVAILABLE_SUMMARY_KEY);
}

export function buildDisplayBooleanSummary(t: TFunction, value: boolean | undefined): string {
  return t(value ? "kanban:on" : "kanban:off");
}
