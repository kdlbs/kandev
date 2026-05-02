import type { StoreApi } from "zustand";
import type { AppState } from "@/lib/state/store";
import type { WsHandlers } from "@/lib/ws/handlers/types";
import type { KanbanState } from "@/lib/state/slices/kanban/types";
import { cleanupTaskStorage } from "@/lib/local-storage";
import { removeRecentTask } from "@/lib/recent-tasks";
import { useContextFilesStore } from "@/lib/state/context-files-store";
import { toKanbanTask, type TaskLike } from "@/lib/kanban/map-task";

type KanbanTask = KanbanState["tasks"][number];

function upsertTask(tasks: KanbanTask[], nextTask: KanbanTask): KanbanTask[] {
  const exists = tasks.some((task) => task.id === nextTask.id);
  return exists
    ? tasks.map((task) => (task.id === nextTask.id ? nextTask : task))
    : [...tasks, nextTask];
}

function upsertMultiTask(state: AppState, workflowId: string, task: KanbanTask): AppState {
  const snapshot = state.kanbanMulti.snapshots[workflowId];
  if (!snapshot) return state;
  return {
    ...state,
    kanbanMulti: {
      ...state.kanbanMulti,
      snapshots: {
        ...state.kanbanMulti.snapshots,
        [workflowId]: {
          ...snapshot,
          tasks: upsertTask(snapshot.tasks, task),
        },
      },
    },
  };
}

type TaskEventPayload = TaskLike & {
  workflow_id: string;
  is_ephemeral?: boolean;
  archived_at?: string | null;
};

/** Upsert a task in both single-kanban and multi-kanban snapshots. */
function upsertTaskInBothKanbans(
  state: AppState,
  wfId: string,
  payload: TaskEventPayload,
): AppState {
  // Skip ephemeral tasks - they should never be added to kanban
  if (payload.is_ephemeral) {
    return state;
  }

  const nextTask = toKanbanTask(payload);
  let next = state;

  if (state.kanban.workflowId === wfId) {
    next = { ...next, kanban: { ...next.kanban, tasks: upsertTask(next.kanban.tasks, nextTask) } };
  }

  if (state.kanbanMulti.snapshots[wfId]) {
    next = upsertMultiTask(next, wfId, nextTask);
  }

  return next;
}

/** Look up a task across both single-kanban and multi-kanban snapshots. */
function findTaskInState(state: AppState, taskId: string): KanbanTask | undefined {
  const fromKanban = state.kanban.tasks.find((t) => t.id === taskId);
  if (fromKanban) return fromKanban;
  for (const snapshot of Object.values(state.kanbanMulti.snapshots)) {
    const found = snapshot.tasks.find((t) => t.id === taskId);
    if (found) return found;
  }
  return undefined;
}

/** Remove a task from both single-kanban and multi-kanban snapshots. */
function removeTaskFromBothKanbans(state: AppState, wfId: string, taskId: string): AppState {
  let next = state;
  if (state.kanban.workflowId === wfId) {
    next = {
      ...next,
      kanban: { ...next.kanban, tasks: next.kanban.tasks.filter((t) => t.id !== taskId) },
    };
  }
  const snapshot = state.kanbanMulti.snapshots[wfId];
  if (snapshot) {
    next = {
      ...next,
      kanbanMulti: {
        ...next.kanbanMulti,
        snapshots: {
          ...next.kanbanMulti.snapshots,
          [wfId]: { ...snapshot, tasks: snapshot.tasks.filter((t) => t.id !== taskId) },
        },
      },
    };
  }
  return next;
}

export function registerTasksHandlers(store: StoreApi<AppState>): WsHandlers {
  return {
    "task.created": (message) => {
      // Skip ephemeral tasks (e.g., quick chat) - they shouldn't appear on the Kanban board
      if (message.payload.is_ephemeral) return;
      store.setState((state) =>
        upsertTaskInBothKanbans(state, message.payload.workflow_id, message.payload),
      );
    },
    "task.updated": (message) => {
      // Skip ephemeral tasks (e.g., quick chat) - they shouldn't appear on the Kanban board
      if (message.payload.is_ephemeral) return;

      // Capture the previous primary session id BEFORE the upsert so we can
      // detect a primary-session swap (e.g. workflow profile switch reusing a
      // different session) and follow focus to the new primary.
      const beforeState = store.getState();
      const taskId = message.payload.task_id;
      const previousPrimary = findTaskInState(beforeState, taskId)?.primarySessionId ?? null;

      store.setState((state) => {
        const wfId = message.payload.workflow_id;

        if (message.payload.archived_at) {
          return removeTaskFromBothKanbans(state, wfId, taskId);
        }

        return upsertTaskInBothKanbans(state, wfId, message.payload);
      });

      // Follow focus to the new primary when:
      //  - the user is currently viewing this task,
      //  - the user was sitting on the previous primary,
      //  - they did NOT explicitly pin that previous primary (a manual click
      //    sets pinnedSessionId; pinned sessions must be left alone), and
      //  - the primary actually changed.
      // This makes workflow profile switches transparent for unpinned users
      // without yanking pinned ones off their deliberate selection.
      const afterState = store.getState();
      const newPrimary = findTaskInState(afterState, taskId)?.primarySessionId ?? null;
      if (
        newPrimary &&
        newPrimary !== previousPrimary &&
        afterState.tasks.activeTaskId === taskId &&
        afterState.tasks.activeSessionId === previousPrimary &&
        afterState.tasks.pinnedSessionId !== previousPrimary
      ) {
        afterState.setActiveSessionAuto(taskId, newPrimary);
      }
    },
    "task.deleted": (message) => {
      const deletedId = message.payload.task_id;
      const wfId = message.payload.workflow_id;
      removeRecentTask(deletedId);

      const currentState = store.getState();
      const sessionIds = (currentState.taskSessionsByTask.itemsByTaskId[deletedId] ?? []).map(
        (s) => s.id,
      );
      const task = currentState.kanban.tasks.find((t) => t.id === deletedId);
      if (task?.primarySessionId && !sessionIds.includes(task.primarySessionId)) {
        sessionIds.push(task.primarySessionId);
      }
      const envIds = Array.from(
        new Set(
          sessionIds
            .map((sid) => currentState.environmentIdBySessionId[sid])
            .filter((eid): eid is string => Boolean(eid)),
        ),
      );
      cleanupTaskStorage(deletedId, sessionIds, envIds);
      for (const sid of sessionIds) {
        useContextFilesStore.getState().clearSession(sid);
      }

      store.setState((state) => {
        const isActive = state.tasks.activeTaskId === deletedId;
        let next = removeTaskFromBothKanbans(state, wfId, deletedId);
        if (isActive) {
          next = { ...next, tasks: { ...next.tasks, activeTaskId: null, activeSessionId: null } };
        }
        return next;
      });
    },
    "task.state_changed": (message) => {
      // Skip ephemeral tasks (e.g., quick chat) - they shouldn't appear on the Kanban board
      if (message.payload.is_ephemeral) return;

      store.setState((state) =>
        upsertTaskInBothKanbans(state, message.payload.workflow_id, message.payload),
      );
    },
  };
}
