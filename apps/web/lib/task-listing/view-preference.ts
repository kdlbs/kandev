export const TASK_LISTING_VIEW_STORAGE_KEY = "kandev.taskListing.view.v1";
export const TASK_LISTING_VIEW_CHANGE_EVENT = "kandev.taskListing.view.change";

export type TaskListingView = "kanban" | "pipeline" | "list";

const DEFAULT_TASK_LISTING_VIEW: TaskListingView = "kanban";
let transientTaskListingView: TaskListingView | null = null;

export function parseTaskListingView(raw: string | null): TaskListingView | null {
  if (!raw) return null;
  try {
    const value: unknown = JSON.parse(raw);
    return value === "kanban" || value === "pipeline" || value === "list" ? value : null;
  } catch {
    return null;
  }
}

export function resolveTaskListingView(
  storedView: TaskListingView | null,
  legacyKanbanViewMode: string | null | undefined,
): TaskListingView {
  if (storedView) return storedView;
  if (hasStoredTaskListingView()) return DEFAULT_TASK_LISTING_VIEW;
  return legacyKanbanViewMode === "graph2" ? "pipeline" : DEFAULT_TASK_LISTING_VIEW;
}

export function getEffectiveTaskListingView(
  preferredView: TaskListingView,
  isMobile: boolean,
): TaskListingView {
  return isMobile && preferredView === "pipeline" ? "kanban" : preferredView;
}

export function shouldRestoreHomeTaskListingView(
  preferredView: TaskListingView,
  initialTaskId: string | undefined,
  initialSessionId: string | undefined,
): boolean {
  return preferredView === "list" && !initialTaskId && !initialSessionId;
}

export function getStoredTaskListingView(): TaskListingView | null {
  if (typeof window === "undefined") return null;
  if (transientTaskListingView) return transientTaskListingView;
  try {
    return parseTaskListingView(window.localStorage.getItem(TASK_LISTING_VIEW_STORAGE_KEY));
  } catch {
    return null;
  }
}

function hasStoredTaskListingView(): boolean {
  if (typeof window === "undefined") return false;
  try {
    return window.localStorage.getItem(TASK_LISTING_VIEW_STORAGE_KEY) !== null;
  } catch {
    return false;
  }
}

export function setStoredTaskListingView(view: TaskListingView): void {
  if (typeof window === "undefined") return;
  try {
    window.localStorage.setItem(TASK_LISTING_VIEW_STORAGE_KEY, JSON.stringify(view));
    transientTaskListingView = null;
  } catch {
    // Storage is unavailable, but preserve the selection until this document closes.
    transientTaskListingView = view;
  }
  window.dispatchEvent(new CustomEvent(TASK_LISTING_VIEW_CHANGE_EVENT));
}
