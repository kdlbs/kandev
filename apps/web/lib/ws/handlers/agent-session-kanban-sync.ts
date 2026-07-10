import type { StoreApi } from "zustand";
import { isDebug } from "@/lib/debug/log";
import type { KanbanState, WorkflowSnapshotData } from "@/lib/state/slices/kanban/types";
import type { AppState } from "@/lib/state/store";
import type { SessionId, TaskId, TaskSessionState } from "@/lib/types/http";
import {
  buildKanbanPrimarySessionSyncLog,
  logKanbanPrimarySessionSync,
} from "@/lib/ws/handlers/agent-session-diagnostics";

function snapshotKanbanStateForLog(state: AppState): AppState {
  return {
    ...state,
    kanban: { ...state.kanban },
    kanbanMulti: {
      ...state.kanbanMulti,
      snapshots: { ...state.kanbanMulti.snapshots },
    },
  };
}

function patchTaskPrimarySessionState(
  tasks: KanbanState["tasks"],
  taskId: string,
  sessionId: string,
  newState: TaskSessionState,
): KanbanState["tasks"] {
  let changed = false;
  const nextTasks = tasks.map((task) => {
    if (task.id !== taskId || task.primarySessionId !== sessionId) return task;
    const nextPendingAction =
      newState === "WAITING_FOR_INPUT" ? task.primarySessionPendingAction : undefined;
    if (
      task.primarySessionState === newState &&
      task.primarySessionPendingAction === nextPendingAction
    ) {
      return task;
    }
    changed = true;
    return {
      ...task,
      primarySessionState: newState,
      primarySessionPendingAction: nextPendingAction,
    };
  });
  return changed ? nextTasks : tasks;
}

function patchSnapshotPrimarySessionState(
  snapshot: WorkflowSnapshotData,
  taskId: string,
  sessionId: string,
  newState: TaskSessionState,
): WorkflowSnapshotData {
  const tasks = patchTaskPrimarySessionState(snapshot.tasks, taskId, sessionId, newState);
  return tasks === snapshot.tasks ? snapshot : { ...snapshot, tasks };
}

function patchWorkflowSnapshotPrimarySessionStates(
  snapshots: Record<string, WorkflowSnapshotData>,
  taskId: string,
  sessionId: string,
  newState: TaskSessionState,
): {
  nextSnapshots: Record<string, WorkflowSnapshotData>;
  snapshotsChanged: boolean;
} {
  let snapshotsChanged = false;
  const nextSnapshots = Object.fromEntries(
    Object.entries(snapshots).map(([workflowId, snapshot]) => {
      const nextSnapshot = patchSnapshotPrimarySessionState(snapshot, taskId, sessionId, newState);
      if (nextSnapshot !== snapshot) snapshotsChanged = true;
      return [workflowId, nextSnapshot];
    }),
  );
  return { nextSnapshots, snapshotsChanged };
}

export function syncKanbanPrimarySessionState(
  store: StoreApi<AppState>,
  taskId: TaskId,
  sessionId: SessionId,
  newState: TaskSessionState | undefined,
): void {
  if (!newState) return;

  const state = store.getState();
  if (!state.kanban?.tasks || !state.kanbanMulti?.snapshots) return;

  const debugMode = isDebug();
  const beforeState = debugMode ? snapshotKanbanStateForLog(state) : state;

  store.setState((currentState) => {
    if (!currentState.kanban?.tasks || !currentState.kanbanMulti?.snapshots) return currentState;
    const nextKanbanTasks = patchTaskPrimarySessionState(
      currentState.kanban.tasks,
      taskId,
      sessionId,
      newState,
    );
    const { nextSnapshots, snapshotsChanged } = patchWorkflowSnapshotPrimarySessionStates(
      currentState.kanbanMulti.snapshots,
      taskId,
      sessionId,
      newState,
    );

    if (nextKanbanTasks === currentState.kanban.tasks && !snapshotsChanged) return currentState;

    return {
      ...currentState,
      kanban:
        nextKanbanTasks === currentState.kanban.tasks
          ? currentState.kanban
          : { ...currentState.kanban, tasks: nextKanbanTasks },
      kanbanMulti: snapshotsChanged
        ? {
            ...currentState.kanbanMulti,
            snapshots: nextSnapshots,
          }
        : currentState.kanbanMulti,
    };
  });

  if (!debugMode) return;
  const syncLog = buildKanbanPrimarySessionSyncLog({
    beforeState,
    afterState: store.getState(),
    taskId,
    sessionId,
    newState,
  });
  logKanbanPrimarySessionSync({
    taskId,
    sessionId,
    newState,
    beforeTask: syncLog.beforeTask,
    patchedKanban: syncLog.patchedKanban,
    patchedSnapshotIds: syncLog.patchedSnapshotIds,
  });
}
