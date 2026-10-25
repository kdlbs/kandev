import { linkToTaskOverview, linkToTasks, linkToThreads, normalizePathname } from "@/lib/links";
import { parseTaskListingView, type TaskListingView } from "./view-preference";

/** Which top-level page the user is looking at when they pick a view. */
export type TaskListingPage = "kanban" | "tasks" | "threads";

type TaskListingNavigationInput = {
  view: string;
  currentPage: TaskListingPage;
  workspaceId?: string;
  workflowId?: string | null;
  /** False on phones, where Pipeline has no layout and falls back to Kanban. */
  allowPipeline?: boolean;
};

export type TaskListingNavigation = {
  view: TaskListingView;
  /** Null when the current page already renders the chosen view. */
  href: string | null;
};

const THREADS_PATH = "/threads";
const TASKS_PATH = "/tasks";

/** The page each view renders on, so the same switch drives every surface. */
const VIEW_PAGE: Record<TaskListingView, TaskListingPage> = {
  kanban: "kanban",
  pipeline: "kanban",
  list: "tasks",
  threads: "threads",
};

function hrefFor(
  view: TaskListingView,
  workspaceId: string | undefined,
  workflowId: string | null | undefined,
): string {
  if (view === "list") return linkToTasks(workspaceId);
  if (view === "threads") return linkToThreads(workspaceId);
  return linkToTaskOverview({ workspaceId, workflowId: workflowId ?? undefined });
}

/**
 * Resolves a view-toggle pick into the view to remember and the page to
 * navigate to. Shared by the desktop topbar and the phone menu sheet so the two
 * cannot drift apart on where a view lives.
 *
 * Returns null when the pick is not a view this surface can honour, and the
 * caller should do nothing at all.
 */
export function resolveTaskListingNavigation({
  view,
  currentPage,
  workspaceId,
  workflowId,
  allowPipeline = true,
}: TaskListingNavigationInput): TaskListingNavigation | null {
  const parsed = parseTaskListingView(JSON.stringify(view));
  if (!parsed) return null;
  if (parsed === "pipeline" && !allowPipeline) return null;
  return {
    view: parsed,
    href: VIEW_PAGE[parsed] === currentPage ? null : hrefFor(parsed, workspaceId, workflowId),
  };
}

/**
 * Where a workspace or workflow change should rewrite the URL to, given the
 * page it happened on.
 *
 * The board's scope handlers `pushState` directly rather than routing, so a
 * fixed task-overview href would leave the deck or the list rendered under a
 * Home URL: the two disagree until a reload or a back/forward silently swaps
 * in the board. Routed views keep their own path instead.
 */
export function listingHistoryHref(
  pathname: string,
  scope: { workspaceId?: string; workflowId?: string },
): string {
  const normalized = normalizePathname(pathname);
  if (normalized === THREADS_PATH) return linkToThreads(scope.workspaceId);
  if (normalized === TASKS_PATH) return linkToTasks(scope.workspaceId);
  return linkToTaskOverview(scope);
}
