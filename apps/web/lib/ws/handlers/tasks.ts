import type { StoreApi } from "zustand";
import type { AppState } from "@/lib/state/store";
import type { WsHandlers } from "@/lib/ws/handlers/types";
import type { KanbanState } from "@/lib/state/slices/kanban/types";
import { cleanupTaskStorage } from "@/lib/local-storage";
import { useContextFilesStore } from "@/lib/state/context-files-store";

type KanbanTask = KanbanState["tasks"][number];

// eslint-disable-next-line @typescript-eslint/no-explicit-any
function buildTaskFromPayload(payload: any, existing?: KanbanTask): KanbanTask {
  const task = {
    id: payload.task_id,
    workflowStepId: payload.workflow_step_id,
    title: payload.title,
    description: payload.description,
    position: payload.position ?? 0,
    state: payload.state,
    repositoryId: payload.repository_id ?? existing?.repositoryId,
    primarySessionId: payload.primary_session_id ?? existing?.primarySessionId,
    sessionCount: payload.session_count ?? existing?.sessionCount,
    reviewStatus: payload.review_status ?? existing?.reviewStatus,
    updatedAt: payload.updated_at ?? existing?.updatedAt,
  };
  return task;
}

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

/** Upsert a task in both single-kanban and multi-kanban snapshots. */
// eslint-disable-next-line @typescript-eslint/no-explicit-any
function upsertTaskInBothKanbans(state: AppState, wfId: string, payload: any): AppState {
  let next = state;

  if (state.kanban.workflowId === wfId) {
    const existing = state.kanban.tasks.find((t) => t.id === payload.task_id);
    const nextTask = buildTaskFromPayload(payload, existing);
    next = { ...next, kanban: { ...next.kanban, tasks: upsertTask(next.kanban.tasks, nextTask) } };
  }

  const snapshot = state.kanbanMulti.snapshots[wfId];
  if (snapshot) {
    const existing = snapshot.tasks.find((t) => t.id === payload.task_id);
    const nextTask = buildTaskFromPayload(payload, existing);
    next = upsertMultiTask(next, wfId, nextTask);
  }

  return next;
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
      store.setState((state) =>
        upsertTaskInBothKanbans(state, message.payload.workflow_id, message.payload),
      );
    },
    "task.updated": (message) => {
      store.setState((state) => {
        const wfId = message.payload.workflow_id;
        const taskId = message.payload.task_id;

        if (message.payload.archived_at) {
          return removeTaskFromBothKanbans(state, wfId, taskId);
        }

        return upsertTaskInBothKanbans(state, wfId, message.payload);
      });
    },
    "task.deleted": (message) => {
      const deletedId = message.payload.task_id;
      const wfId = message.payload.workflow_id;

      const currentState = store.getState();
      const sessionIds = (currentState.taskSessionsByTask.itemsByTaskId[deletedId] ?? []).map(
        (s) => s.id,
      );
      const task = currentState.kanban.tasks.find((t) => t.id === deletedId);
      if (task?.primarySessionId && !sessionIds.includes(task.primarySessionId)) {
        sessionIds.push(task.primarySessionId);
      }
      cleanupTaskStorage(deletedId, sessionIds);
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
      store.setState((state) =>
        upsertTaskInBothKanbans(state, message.payload.workflow_id, message.payload),
      );
    },
  };
}
