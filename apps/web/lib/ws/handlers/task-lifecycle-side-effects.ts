import type { AppState } from "@/lib/state/store";
import { softNavigate } from "@/lib/routing/client-router";
import { isTaskDetailPath, linkToTaskOverview, normalizePathname } from "@/lib/links";

export function clearRemovedTaskSelection(state: AppState, taskId: string): AppState {
  let next = state;
  if (next.tasks.activeTaskId === taskId) {
    next = {
      ...next,
      tasks: {
        ...next.tasks,
        activeTaskId: null,
        activeSessionId: null,
        pinnedSessionId: null,
      },
    };
  }
  if (next.tasks.lastSessionByTaskId[taskId]) {
    const { [taskId]: _, ...rest } = next.tasks.lastSessionByTaskId;
    next = { ...next, tasks: { ...next.tasks, lastSessionByTaskId: rest } };
  }
  return next;
}

export function clearDeletedTaskWalkthrough(state: AppState, taskId: string): AppState {
  if (!state.walkthroughs?.byTaskId) return state;
  if (!(taskId in state.walkthroughs.byTaskId)) return state;
  const { [taskId]: _removedWalkthrough, ...byTaskId } = state.walkthroughs.byTaskId;
  const { [taskId]: _removedStep, ...activeStepByTaskId } = state.walkthroughs.activeStepByTaskId;
  const { [taskId]: _removedLastSeen, ...lastSeenUpdatedAtByTaskId } =
    state.walkthroughs.lastSeenUpdatedAtByTaskId;
  return {
    ...state,
    walkthroughs: {
      ...state.walkthroughs,
      byTaskId,
      activeStepByTaskId,
      lastSeenUpdatedAtByTaskId,
    },
  };
}

export function removedTaskRedirectHref(pathname: string, taskId: string): string | null {
  if (isTaskDetailPath(pathname, taskId)) return linkToTaskOverview();
  const normalized = normalizePathname(pathname);
  return normalized === `/office/tasks/${taskId}` ? "/office/tasks" : null;
}

/**
 * Soft-redirect away from a removed task's page. Only fires when the user is
 * currently parked on that task's route, so a background removal of some other
 * task never yanks the user elsewhere.
 */
export function redirectAwayFromRemovedTask(taskId: string): void {
  if (typeof window === "undefined") return;
  const href = removedTaskRedirectHref(window.location.pathname, taskId);
  if (!href) return;
  softNavigate(href, "replace");
}
