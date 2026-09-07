export const TASK_LISTING_VIEW_STORAGE_KEY = "kandev.taskListing.view.v1";
export const TASK_LISTING_VIEW_CHANGE_EVENT = "kandev.taskListing.view.change";

export type TaskListingView = "kanban" | "pipeline" | "list" | "threads";

/**
 * Views that live on their own route instead of inside the Home board. Home
 * hands navigation over to them when they are the remembered preference.
 */
const ROUTED_TASK_LISTING_VIEWS: TaskListingView[] = ["list", "threads"];

const TASK_LISTING_VIEWS: TaskListingView[] = ["kanban", "pipeline", "list", "threads"];

const DEFAULT_TASK_LISTING_VIEW: TaskListingView = "kanban";
let transientTaskListingView: TaskListingView | null = null;

export function parseTaskListingView(raw: string | null): TaskListingView | null {
  if (!raw) return null;
  try {
    const value: unknown = JSON.parse(raw);
    return TASK_LISTING_VIEWS.find((view) => view === value) ?? null;
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

/**
 * The routed view Home should hand off to, or null when Home renders the
 * preference itself. An explicitly opened task or session always wins: the URL
 * the user followed is a stronger signal than the remembered listing.
 */
export function resolveHomeTaskListingRedirect(
  preferredView: TaskListingView,
  initialTaskId: string | undefined,
  initialSessionId: string | undefined,
): TaskListingView | null {
  if (initialTaskId || initialSessionId) return null;
  return ROUTED_TASK_LISTING_VIEWS.includes(preferredView) ? preferredView : null;
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
