"use client";

import { useAppStore } from "@/components/state-provider";
import { findTaskInSnapshots } from "@/lib/kanban/find-task";
import { isThreadTaskEligible, resolveTaskPendingAction } from "@/lib/threads/active-threads";

/**
 * Whether this session can be opened in the Threads deck.
 *
 * Reads the already-loaded task sessions rather than fetching. A settled
 * sibling can be selected when its task still has a live primary column, but
 * an inactive task still hides the affordance because Threads has no column.
 */
export function useIsDeckThread(taskId: string | null, sessionId: string | null): boolean {
  return useAppStore((state) => {
    if (!taskId || !sessionId) return false;
    const session = state.taskSessionsByTask.itemsByTaskId[taskId]?.find(
      (candidate) => candidate.id === sessionId,
    );
    if (!session) return false;
    const taskSessions = state.taskSessionsByTask.itemsByTaskId[taskId] ?? [];
    const task = findTaskInSnapshots(taskId, state.kanbanMulti.snapshots, state.kanban.tasks);
    const primarySession =
      taskSessions.find((candidate) => candidate.is_primary) ??
      (session.is_primary ? session : null);
    return isThreadTaskEligible({
      taskState: task?.state ?? null,
      reviewStatus: task?.reviewStatus ?? null,
      taskPendingAction: task ? resolveTaskPendingAction(task) : null,
      primarySession: primarySession
        ? {
            state: primarySession.state,
            pendingAction: primarySession.pending_action,
          }
        : null,
    });
  });
}
