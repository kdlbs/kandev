import { useMemo } from "react";
import { useAppStore } from "@/components/state-provider";
import {
  buildTaskFromKanban,
  resolveLatestTaskProjection,
} from "@/components/task/task-page-content-helpers";
import type { AppState } from "@/lib/state/store";

function ownedSession(state: AppState, taskId: string, sessionId?: string | null) {
  if (!sessionId) return null;
  return state.taskSessions.items[sessionId]?.task_id === taskId ? sessionId : null;
}

/** Presentation reads current domain state without replaying a previous route snapshot. */
export function useTaskRouteProjection(taskId: string, requestedSessionId?: string) {
  const projection = useAppStore((state) => {
    if (state.auth.mode !== "disabled" && !state.auth.authenticated) return null;
    const task = resolveLatestTaskProjection(
      taskId,
      state.kanban.tasks,
      state.kanbanMulti.snapshots,
    );
    if (!task?.workspaceId || task.workspaceId !== state.workspaces.activeId || task.isArchived) {
      return null;
    }
    if (requestedSessionId && !ownedSession(state, taskId, requestedSessionId)) return null;
    return task;
  });
  const sessionId = useAppStore((state) => {
    if (!projection) return null;
    if (requestedSessionId) return ownedSession(state, taskId, requestedSessionId);
    return (
      ownedSession(state, taskId, state.tasks.activeSessionId) ??
      ownedSession(state, taskId, projection.primarySessionId)
    );
  });
  const task = useMemo(() => (projection ? buildTaskFromKanban(projection) : null), [projection]);
  return task ? { task, sessionId } : null;
}
