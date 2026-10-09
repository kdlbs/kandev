import { useMemo } from "react";
import { useShallow } from "zustand/react/shallow";
import { useAppStore } from "@/components/state-provider";
import {
  buildTaskFromKanban,
  resolveLatestTaskProjection,
} from "@/components/task/task-page-content-helpers";
import type { AppState } from "@/lib/state/store";

export function getOwnedTaskSessionId(state: AppState, taskId: string, sessionId?: string | null) {
  if (!sessionId) return null;
  return state.taskSessions.items[sessionId]?.task_id === taskId ? sessionId : null;
}

/** Presentation reads current domain state without replaying a previous route snapshot. */
export function useTaskRouteProjection(taskId: string, requestedSessionId?: string) {
  const [projection, sessionId] = useAppStore(
    useShallow((state) => {
      if (state.auth.mode !== "disabled" && !state.auth.authenticated) return [null, null] as const;
      const task = resolveLatestTaskProjection(
        taskId,
        state.kanban.tasks,
        state.kanbanMulti.snapshots,
      );
      if (!task?.workspaceId || task.workspaceId !== state.workspaces.activeId || task.isArchived) {
        return [null, null] as const;
      }
      const requested = getOwnedTaskSessionId(state, taskId, requestedSessionId);
      if (requestedSessionId && !requested) return [null, null] as const;
      const selected =
        requested ??
        getOwnedTaskSessionId(state, taskId, state.tasks.activeSessionId) ??
        getOwnedTaskSessionId(state, taskId, task.primarySessionId);
      return [task, selected] as const;
    }),
  );
  const task = useMemo(() => (projection ? buildTaskFromKanban(projection) : null), [projection]);
  return task ? { task, sessionId } : null;
}
