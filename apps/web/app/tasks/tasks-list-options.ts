import type { Task, TaskState } from "@/lib/types/http";

export const TASKS_LIST_SORT_OPTIONS = [
  { value: "updated_desc", label: "Updated newest" },
  { value: "updated_asc", label: "Updated oldest" },
  { value: "created_desc", label: "Created newest" },
  { value: "created_asc", label: "Created oldest" },
  { value: "title_asc", label: "Title A-Z" },
  { value: "title_desc", label: "Title Z-A" },
] as const;

export const TASKS_LIST_GROUP_OPTIONS = [
  { value: "state", label: "State" },
  { value: "workflow", label: "Workflow" },
  { value: "repository", label: "Repository" },
  { value: "none", label: "None" },
] as const;

export type TasksListSort = (typeof TASKS_LIST_SORT_OPTIONS)[number]["value"];
export type TasksListGroup = (typeof TASKS_LIST_GROUP_OPTIONS)[number]["value"];

export const DEFAULT_TASKS_LIST_SORT: TasksListSort = "updated_desc";
export const DEFAULT_TASKS_LIST_GROUP: TasksListGroup = "state";

export const TASK_STATE_ORDER: TaskState[] = [
  "TODO",
  "IN_PROGRESS",
  "REVIEW",
  "WAITING_FOR_INPUT",
  "BLOCKED",
  "FAILED",
  "COMPLETED",
  "CANCELLED",
  "SCHEDULING",
  "CREATED",
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
