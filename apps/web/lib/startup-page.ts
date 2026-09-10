import { findMostRecentTaskForWorkspace, type RecentTaskEntry } from "./recent-tasks";
import type { StartupPage } from "./types/http-user-settings";
import { linkToTasks, linkToThreads } from "./links";
import {
  resolveHomeTaskListingRedirect,
  type TaskListingView,
} from "./task-listing/view-preference";

type StartupTaskResolutionInput = {
  startupPage: StartupPage;
  workspaceId: string | undefined;
  recentTasks: RecentTaskEntry[];
  hasExplicitDestination: boolean;
};

export function isExplicitHomeDestination(
  searchParams: URLSearchParams,
  initialTaskId?: string,
  initialSessionId?: string,
): boolean {
  return Boolean(
    initialTaskId ||
    initialSessionId ||
    searchParams.get("taskId") ||
    searchParams.get("sessionId") ||
    searchParams.get("workflowId") ||
    searchParams.get("home") === "overview",
  );
}

export function resolveStartupTaskId({
  startupPage,
  workspaceId,
  recentTasks,
  hasExplicitDestination,
}: StartupTaskResolutionInput): string | null {
  if (startupPage !== "last_task" || hasExplicitDestination) return null;
  return findMostRecentTaskForWorkspace(recentTasks, workspaceId)?.taskId ?? null;
}

export function resolveStartupListingRedirect({
  startupPage,
  preferredView,
  workspaceId,
  searchParams,
  initialTaskId,
  initialSessionId,
}: {
  startupPage: StartupPage;
  preferredView: TaskListingView;
  workspaceId?: string;
  searchParams: URLSearchParams;
  initialTaskId?: string;
  initialSessionId?: string;
}): string | null {
  if (
    initialTaskId ||
    initialSessionId ||
    searchParams.get("taskId") ||
    searchParams.get("sessionId") ||
    searchParams.get("workflowId")
  )
    return null;
  // Explicit overview skips the fixed default but still restores the remembered listing.
  if (startupPage === "threads" && !isExplicitHomeDestination(searchParams)) {
    return workspaceId ? linkToThreads(workspaceId) : null;
  }
  const routedView = resolveHomeTaskListingRedirect(preferredView, initialTaskId, initialSessionId);
  if (routedView === "threads") return linkToThreads(workspaceId);
  return routedView === "list" ? linkToTasks(workspaceId) : null;
}
