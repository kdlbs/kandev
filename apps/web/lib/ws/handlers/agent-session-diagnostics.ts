import { createDebugLogger, isDebug } from "@/lib/debug/log";
import type { KanbanState, WorkflowSnapshotData } from "@/lib/state/slices/kanban/types";
import type { AppState } from "@/lib/state/store";
import type { TaskSessionState } from "@/lib/types/http";

type KanbanTask = KanbanState["tasks"][number];

const lifecycleDebug = createDebugLogger("task-lifecycle:ws");

function findSnapshotTask(
  snapshots: Record<string, WorkflowSnapshotData>,
  workflowId: string,
  taskId: string,
  sessionId: string,
): KanbanTask | undefined {
  return snapshots[workflowId]?.tasks.find(
    (task) => task.id === taskId && task.primarySessionId === sessionId,
  );
}

function didPatchTaskPrimaryState(
  beforeTask: KanbanTask | undefined,
  afterTask: KanbanTask | undefined,
  newState: TaskSessionState,
): boolean {
  return (
    beforeTask?.primarySessionState !== newState && afterTask?.primarySessionState === newState
  );
}

export function buildKanbanPrimarySessionSyncLog(params: {
  beforeState: AppState;
  afterState: AppState;
  taskId: string;
  sessionId: string;
  newState: TaskSessionState;
}): {
  beforeTask: KanbanTask | undefined;
  patchedKanban: boolean;
  patchedSnapshotIds: string[];
} {
  const beforeTask =
    params.beforeState.kanban.tasks.find((task) => task.id === params.taskId) ??
    Object.values(params.beforeState.kanbanMulti.snapshots)
      .flatMap((snapshot) => snapshot.tasks)
      .find((task) => task.id === params.taskId);
  const beforeKanbanTask = params.beforeState.kanban.tasks.find(
    (task) => task.id === params.taskId && task.primarySessionId === params.sessionId,
  );
  const afterKanbanTask = params.afterState.kanban.tasks.find(
    (task) => task.id === params.taskId && task.primarySessionId === params.sessionId,
  );
  const patchedSnapshotIds = Object.keys(params.afterState.kanbanMulti.snapshots).filter(
    (workflowId) => {
      const beforeSnapshotTask = findSnapshotTask(
        params.beforeState.kanbanMulti.snapshots,
        workflowId,
        params.taskId,
        params.sessionId,
      );
      const afterSnapshotTask = findSnapshotTask(
        params.afterState.kanbanMulti.snapshots,
        workflowId,
        params.taskId,
        params.sessionId,
      );
      return didPatchTaskPrimaryState(beforeSnapshotTask, afterSnapshotTask, params.newState);
    },
  );
  return {
    beforeTask,
    patchedKanban: didPatchTaskPrimaryState(beforeKanbanTask, afterKanbanTask, params.newState),
    patchedSnapshotIds,
  };
}

export function logKanbanPrimarySessionSync(params: {
  taskId: string;
  sessionId: string;
  newState: TaskSessionState;
  beforeTask: KanbanTask | undefined;
  patchedKanban: boolean;
  patchedSnapshotIds: string[];
}): void {
  if (!isDebug()) return;
  lifecycleDebug("session.state_changed kanban sync", {
    task_id: params.taskId,
    sessionId: params.sessionId,
    newState: params.newState,
    beforeTaskPrimarySessionId: params.beforeTask?.primarySessionId ?? "-",
    beforeTaskPrimaryState: params.beforeTask?.primarySessionState ?? "-",
    patchedKanban: params.patchedKanban,
    patchedSnapshots: params.patchedSnapshotIds.join(",") || "-",
  });
}
