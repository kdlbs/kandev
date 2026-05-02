import type { TaskSessionState, TaskState } from "@/lib/types/http";

export const RECENT_TASKS_STORAGE_KEY = "kandev.recentTasks.v1";
export const RECENT_TASKS_CHANGED_EVENT = "kandev:recent-tasks-changed";
export const MAX_RECENT_TASKS = 12;

export type RecentTaskEntry = {
  taskId: string;
  title: string;
  visitedAt: string;
  taskState?: TaskState | null;
  sessionState?: TaskSessionState | null;
  repositoryPath?: string | null;
  workflowId?: string | null;
  workflowName?: string | null;
  workflowStepTitle?: string | null;
  workspaceId?: string | null;
};

type RecentTasksChangedDetail = {
  entries: RecentTaskEntry[];
};

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

function optionalString(value: unknown): string | null {
  return typeof value === "string" && value.length > 0 ? value : null;
}

function normalizeEntry(value: unknown): RecentTaskEntry | null {
  if (!isRecord(value)) return null;
  const taskId = optionalString(value.taskId);
  if (!taskId) return null;
  const title = optionalString(value.title) ?? "Untitled task";
  const visitedAt = optionalString(value.visitedAt) ?? new Date(0).toISOString();

  return {
    taskId,
    title,
    visitedAt,
    taskState: optionalString(value.taskState) as TaskState | null,
    sessionState: optionalString(value.sessionState) as TaskSessionState | null,
    repositoryPath: optionalString(value.repositoryPath),
    workflowId: optionalString(value.workflowId),
    workflowName: optionalString(value.workflowName),
    workflowStepTitle: optionalString(value.workflowStepTitle),
    workspaceId: optionalString(value.workspaceId),
  };
}

function normalizeEntries(value: unknown): RecentTaskEntry[] {
  if (!Array.isArray(value)) return [];
  return value
    .map(normalizeEntry)
    .filter((entry): entry is RecentTaskEntry => entry !== null)
    .slice(0, MAX_RECENT_TASKS);
}

function emitRecentTasksChanged(entries: RecentTaskEntry[]): void {
  if (typeof window === "undefined") return;
  window.dispatchEvent(
    new CustomEvent<RecentTasksChangedDetail>(RECENT_TASKS_CHANGED_EVENT, {
      detail: { entries },
    }),
  );
}

export function getRecentTasks(): RecentTaskEntry[] {
  if (typeof window === "undefined") return [];
  try {
    const raw = window.localStorage.getItem(RECENT_TASKS_STORAGE_KEY);
    if (!raw) return [];
    return normalizeEntries(JSON.parse(raw));
  } catch {
    return [];
  }
}

export function setRecentTasks(entries: RecentTaskEntry[]): RecentTaskEntry[] {
  const normalized = normalizeEntries(entries);
  if (typeof window === "undefined") return normalized;
  try {
    window.localStorage.setItem(RECENT_TASKS_STORAGE_KEY, JSON.stringify(normalized));
    emitRecentTasksChanged(normalized);
  } catch {
    // Ignore write failures: private browsing, blocked storage, or quota issues.
  }
  return normalized;
}

export function upsertRecentTask(entry: RecentTaskEntry): RecentTaskEntry[] {
  const current = getRecentTasks().filter((item) => item.taskId !== entry.taskId);
  return setRecentTasks([entry, ...current].slice(0, MAX_RECENT_TASKS));
}

export function removeRecentTask(taskId: string): RecentTaskEntry[] {
  return setRecentTasks(getRecentTasks().filter((entry) => entry.taskId !== taskId));
}
