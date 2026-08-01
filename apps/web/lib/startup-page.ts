import { findMostRecentTaskForWorkspace, type RecentTaskEntry } from "./recent-tasks";
import type { StartupPage } from "./types/http-user-settings";

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
