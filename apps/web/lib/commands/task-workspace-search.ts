import { isTaskDetailPath } from "@/lib/links";
import type { AppState } from "@/lib/state/store";

export function isTaskWorkspaceSearchAvailable(state: AppState, pathname: string): boolean {
  const { activeTaskId, activeSessionId } = state.tasks;
  if (!activeTaskId || !activeSessionId || !isTaskDetailPath(pathname, activeTaskId)) return false;
  return state.taskSessions.items[activeSessionId]?.task_id === activeTaskId;
}
