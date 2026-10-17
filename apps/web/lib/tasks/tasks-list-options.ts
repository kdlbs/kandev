import type { Task } from "@/lib/types/http";
import type { TaskListFacetValue } from "@/lib/plugins/types";

// Values only. These carried a parallel `label: "Updated newest"` alongside the
// key maps below, described as a "fallback/debug value" — but the mobile menu
// (`mobile-menu-task-list-options.tsx`) rendered `option.label` directly, so the
// phone sort and group pickers shipped untranslated while the desktop ones
// resolved through the keys. One source of display copy, and no way to render
// the wrong one.
export const TASKS_LIST_SORT_OPTIONS = [
  { value: "updated_desc" },
  { value: "updated_asc" },
  { value: "created_desc" },
  { value: "created_asc" },
  { value: "title_asc" },
  { value: "title_desc" },
] as const;

export const TASKS_LIST_GROUP_OPTIONS = [
  { value: "workflow_step" },
  { value: "workflow" },
  { value: "repository" },
  { value: "none" },
] as const;

export type TasksListSort = (typeof TASKS_LIST_SORT_OPTIONS)[number]["value"];
export type TasksListGroup = (typeof TASKS_LIST_GROUP_OPTIONS)[number]["value"];
export type TaskListFacetOption = `facet:${string}:${string}`;
export type TasksListSortPreference = TasksListSort | TaskListFacetOption;
export type TasksListGroupPreference = TasksListGroup | TaskListFacetOption;

export const DEFAULT_TASKS_LIST_SORT: TasksListSort = "updated_desc";
export const DEFAULT_TASKS_LIST_GROUP: TasksListGroup = "workflow_step";

// i18n-exempt: internal client-only option namespace, never rendered as copy.
export const TASK_LIST_FACET_PREFIX = "facet:";

const PLUGIN_ID_PATTERN = /^[a-z0-9][a-z0-9._-]*$/;
const FACET_ID_PATTERN = /^[a-z0-9]+(?:-[a-z0-9]+)*$/;

export function encodeTaskListFacetOption(facet: {
  pluginId: string;
  id: string;
}): TaskListFacetOption {
  return `${TASK_LIST_FACET_PREFIX}${facet.pluginId}:${facet.id}`;
}

export function parseTaskListFacetOption(value: string): { pluginId: string; id: string } | null {
  const trimmed = value.trim();
  if (!trimmed.startsWith(TASK_LIST_FACET_PREFIX)) return null;
  const segments = trimmed.slice(TASK_LIST_FACET_PREFIX.length).split(":");
  const [pluginId, id] = segments;
  return segments.length === 2 && PLUGIN_ID_PATTERN.test(pluginId) && FACET_ID_PATTERN.test(id)
    ? { pluginId, id }
    : null;
}

export function isTaskListFacetOption(value: string): value is TaskListFacetOption {
  return parseTaskListFacetOption(value) !== null;
}

// The only source of display copy for these options; resolved at render against
// the `tasks:` catalog.
export const SORT_OPTION_LABEL_KEYS: Record<TasksListSort, string> = {
  updated_desc: "tasks:sortUpdatedNewest",
  updated_asc: "tasks:sortUpdatedOldest",
  created_desc: "tasks:sortCreatedNewest",
  created_asc: "tasks:sortCreatedOldest",
  title_asc: "tasks:sortTitleAZ",
  title_desc: "tasks:sortTitleZA",
};

export const GROUP_OPTION_LABEL_KEYS: Record<TasksListGroup, string> = {
  workflow_step: "tasks:groupByWorkflowStep",
  workflow: "tasks:groupByWorkflow",
  repository: "tasks:groupByRepository",
  none: "tasks:groupByNone",
};

export function parseTasksListSort(value: string | null | undefined): TasksListSortPreference {
  const trimmed = value?.trim() ?? "";
  if (TASKS_LIST_SORT_OPTIONS.some((option) => option.value === trimmed))
    return trimmed as TasksListSort;
  return isTaskListFacetOption(trimmed) ? trimmed : DEFAULT_TASKS_LIST_SORT;
}

export function parseTasksListGroup(value: string | null | undefined): TasksListGroupPreference {
  const trimmed = value?.trim() ?? "";
  if (TASKS_LIST_GROUP_OPTIONS.some((option) => option.value === trimmed))
    return trimmed as TasksListGroup;
  return isTaskListFacetOption(trimmed) ? trimmed : DEFAULT_TASKS_LIST_GROUP;
}

export function resolveTasksListApiSort(value: string): TasksListSort {
  const requested = parseTasksListSort(value);
  return isTaskListFacetOption(requested) ? DEFAULT_TASKS_LIST_SORT : requested;
}

export function sortTasksForList(tasks: Task[], sort: TasksListSort): Task[] {
  return [...tasks].sort((a, b) => compareTasksForList(a, b, sort));
}

export function compareTasksForList(a: Task, b: Task, sort: TasksListSort): number {
  const titleCompare = a.title.localeCompare(b.title, undefined, { sensitivity: "base" });
  switch (sort) {
    case "updated_asc":
      return compareDate(a.updated_at, b.updated_at) || titleCompare;
    case "created_desc":
      return compareDate(b.created_at, a.created_at) || titleCompare;
    case "created_asc":
      return compareDate(a.created_at, b.created_at) || titleCompare;
    case "title_asc":
      return titleCompare || compareDate(b.updated_at, a.updated_at);
    case "title_desc":
      return -titleCompare || compareDate(b.updated_at, a.updated_at);
    case "updated_desc":
    default:
      return compareDate(b.updated_at, a.updated_at) || titleCompare;
  }
}

function compareDate(a: string, b: string): number {
  return Date.parse(a) - Date.parse(b);
}

export function sortTasksByFacet(
  tasks: Task[],
  facetKey: string,
  values: Record<string, readonly TaskListFacetValue[]>,
): Task[] {
  return tasks
    .map((task, index) => ({
      task,
      index,
      label: firstFacetLabel(values[`${facetKey}:${task.id}`] ?? []),
    }))
    .sort((a, b) => {
      if (!a.label && !b.label) return a.index - b.index;
      if (!a.label) return 1;
      if (!b.label) return -1;
      return (
        a.label.localeCompare(b.label, undefined, { sensitivity: "base" }) || a.index - b.index
      );
    })
    .map(({ task }) => task);
}

export function firstFacetLabel(values: readonly TaskListFacetValue[]): string | null {
  let first: string | null = null;
  for (const { label } of values) {
    if (
      label &&
      (first === null || label.localeCompare(first, undefined, { sensitivity: "base" }) < 0)
    ) {
      first = label;
    }
  }
  return first;
}
