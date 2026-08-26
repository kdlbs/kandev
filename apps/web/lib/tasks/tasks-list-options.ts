import type { Task, TaskState } from "@/lib/types/http";
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
  { value: "state" },
  { value: "workflow" },
  { value: "repository" },
  { value: "none" },
] as const;

export type TasksListSort = (typeof TASKS_LIST_SORT_OPTIONS)[number]["value"];
export type TasksListGroup = (typeof TASKS_LIST_GROUP_OPTIONS)[number]["value"];

export const DEFAULT_TASKS_LIST_SORT: TasksListSort = "updated_desc";
export const DEFAULT_TASKS_LIST_GROUP: TasksListGroup = "state";

// i18n-exempt: internal client-only option namespace, never rendered as copy.
export const TASK_LIST_FACET_PREFIX = "facet:";

export function isTaskListFacetOption(value: string): boolean {
  return value.startsWith(TASK_LIST_FACET_PREFIX);
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
  state: "tasks:groupByState",
  workflow: "tasks:groupByWorkflow",
  repository: "tasks:groupByRepository",
  none: "tasks:groupByNone",
};

export const TASK_STATE_ORDER: TaskState[] = [
  "CREATED",
  "SCHEDULING",
  "TODO",
  "IN_PROGRESS",
  "REVIEW",
  "BLOCKED",
  "WAITING_FOR_INPUT",
  "COMPLETED",
  "FAILED",
  "CANCELLED",
];

export function parseTasksListSort(value: string | null | undefined): TasksListSort {
  return TASKS_LIST_SORT_OPTIONS.some((option) => option.value === value)
    ? (value as TasksListSort)
    : DEFAULT_TASKS_LIST_SORT;
}

export function parseTasksListGroup(value: string | null | undefined): TasksListGroup {
  return TASKS_LIST_GROUP_OPTIONS.some((option) => option.value === value)
    ? (value as TasksListGroup)
    : DEFAULT_TASKS_LIST_GROUP;
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
  return (
    values
      .map((value) => value.label)
      .sort((a, b) => a.localeCompare(b, undefined, { sensitivity: "base" }))[0] ?? null
  );
}
