import { vi } from "vitest";
import type { StoreApi } from "zustand";
import type { AppState } from "@/lib/state/store";
import { registerTasksHandlers } from "./tasks";

export const SESS_OTHER = "sess-other";
export const SESS_DRIFTED = "sess-drifted";
export const SESS_PINNED = "sess-pinned";
export const REVIEW_TITLE = "Review PR #11259";

type Listener = (state: AppState) => void;

export function makeStore(initial: Partial<AppState> = {}) {
  let state = {
    kanban: { workflowId: "wf1", steps: [], tasks: [] },
    kanbanMulti: { snapshots: {}, isLoading: false },
    tasks: {
      activeTaskId: null,
      activeSessionId: null,
      pinnedSessionId: null,
      lastSessionByTaskId: {},
    },
    taskSessionsByTask: { itemsByTaskId: {}, loadedByTaskId: {}, loadingByTaskId: {} },
    environmentIdBySessionId: {},
    setActiveSession: vi.fn((taskId: string, sessionId: string | null) => {
      state = {
        ...state,
        tasks: {
          activeTaskId: taskId,
          activeSessionId: sessionId,
          pinnedSessionId: sessionId,
          lastSessionByTaskId: sessionId
            ? { ...state.tasks.lastSessionByTaskId, [taskId]: sessionId }
            : state.tasks.lastSessionByTaskId,
        },
      };
    }),
    setActiveSessionAuto: vi.fn((taskId: string, sessionId: string | null) => {
      state = {
        ...state,
        tasks: {
          ...state.tasks,
          activeTaskId: taskId,
          activeSessionId: sessionId,
        },
      };
    }),
    removeTaskFromSidebarPrefs: vi.fn(),
    setTaskDeletedNotification: vi.fn(),
    ...initial,
  } as unknown as AppState;

  const listeners = new Set<Listener>();
  return {
    getState: () => state,
    setState: (updater: AppState | ((current: AppState) => AppState)) => {
      const next =
        typeof updater === "function"
          ? (updater as (current: AppState) => AppState)(state)
          : updater;
      state = { ...state, ...next };
      for (const listener of listeners) listener(state);
    },
    subscribe: (listener: Listener) => {
      listeners.add(listener);
      return () => listeners.delete(listener);
    },
    destroy: vi.fn(),
    getInitialState: vi.fn(),
  } as unknown as StoreApi<AppState> & { getState: () => AppState };
}

export function makeTask(id: string, primarySessionId: string | null, workflowId = "wf1") {
  return {
    task_id: id,
    workflow_id: workflowId,
    workflow_step_id: "step1",
    title: "Test",
    description: "",
    state: "IN_PROGRESS",
    primary_session_id: primarySessionId,
    is_ephemeral: false,
  } as Record<string, unknown>;
}

export function makeMessage(payload: Record<string, unknown>) {
  return {
    id: "msg-1",
    type: "notification" as const,
    action: "task.updated" as const,
    payload,
  } as Parameters<NonNullable<ReturnType<typeof registerTasksHandlers>["task.updated"]>>[0];
}

export function makeDeletedMessage(payload: Record<string, unknown>) {
  return {
    id: "msg-1",
    type: "notification" as const,
    action: "task.deleted" as const,
    payload,
  } as Parameters<NonNullable<ReturnType<typeof registerTasksHandlers>["task.deleted"]>>[0];
}

export function makeActiveStore() {
  return makeStore({
    kanban: { workflowId: "wf1", steps: [], tasks: [{ id: "t1", workflowId: "wf1" }] },
    tasks: {
      activeTaskId: "t1",
      activeSessionId: null,
      pinnedSessionId: null,
      lastSessionByTaskId: {},
    },
    environmentIdBySessionId: {},
  } as unknown as Partial<AppState>);
}

export function makeInactiveStore() {
  return makeStore({
    kanban: { workflowId: "wf1", steps: [], tasks: [{ id: "t1", workflowId: "wf1" }] },
    tasks: {
      activeTaskId: null,
      activeSessionId: null,
      pinnedSessionId: null,
      lastSessionByTaskId: {},
    },
    environmentIdBySessionId: {},
  } as unknown as Partial<AppState>);
}
