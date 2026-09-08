import { describe, expect, it, vi } from "vitest";
import type { StoreApi } from "zustand";
import type { AppState } from "@/lib/state/store";
import { registerTasksHandlers } from "./tasks";

type Listener = (state: AppState) => void;

/**
 * Minimal in-memory store for the tasks WS handler tests.
 * The handler reads kanban tasks, kanbanMulti snapshots, and tasks.activeTaskId/activeSessionId,
 * and calls setActiveSession; everything else can stay default.
 */
function makeStore(initial: Partial<AppState> = {}) {
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
          ...state.tasks,
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
    setState: (updater: AppState | ((s: AppState) => AppState)) => {
      const next =
        typeof updater === "function" ? (updater as (s: AppState) => AppState)(state) : updater;
      state = { ...state, ...next };
      for (const l of listeners) l(state);
    },
    subscribe: (l: Listener) => {
      listeners.add(l);
      return () => listeners.delete(l);
    },
    destroy: vi.fn(),
    getInitialState: vi.fn(),
  } as unknown as StoreApi<AppState> & { getState: () => AppState };
}

function makeTask(id: string, primarySessionId: string | null, workflowId = "wf1") {
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

function makeMessage(payload: Record<string, unknown>) {
  return {
    id: "msg-1",
    type: "notification" as const,
    action: "task.updated" as const,
    payload,
  } as Parameters<NonNullable<ReturnType<typeof registerTasksHandlers>["task.updated"]>>[0];
}

// Shared setup for the primary-session focus-follow tests: a single task t1
// whose kanban primary, plus the active/pinned session ids, are the only knobs
// that vary between cases.

function makeAssigneeStore(assigneeUserId?: string) {
  const existingTask = {
    id: "t1",
    workflowStepId: "step1",
    title: "Old title",
    position: 0,
    primarySessionId: "session-1",
    assigneeUserId,
  };
  return makeStore({
    kanban: {
      workflowId: "wf1",
      steps: [],
      tasks: [existingTask],
    } as unknown as AppState["kanban"],
    kanbanMulti: {
      isLoading: false,
      snapshots: {
        wf1: { workflowId: "wf1", workflowName: "WF1", steps: [], tasks: [existingTask] },
      },
    } as unknown as AppState["kanbanMulti"],
  });
}

describe("human assignee preservation", () => {
  // Observed live before this guard: taking a task over showed the new owner,
  // then the next unrelated task.updated blanked the top bar back to
  // "Unassigned" while the database still held the assignment.
  it("preserves the assignee when a partial update omits assignee_user_id", () => {
    const store = makeAssigneeStore("user-7");

    registerTasksHandlers(store)["task.updated"]!(
      makeMessage({ ...makeTask("t1", "session-1"), title: "Retitled task" }),
    );

    const state = store.getState();
    expect(state.kanban.tasks[0]).toMatchObject({ assigneeUserId: "user-7" });
    expect(state.kanbanMulti.snapshots.wf1.tasks[0]).toMatchObject({ assigneeUserId: "user-7" });
  });

  it("adopts an explicit reassignment", () => {
    const store = makeAssigneeStore("user-7");

    registerTasksHandlers(store)["task.updated"]!(
      makeMessage({ ...makeTask("t1", "session-1"), assignee_user_id: "user-9" }),
    );

    expect(store.getState().kanban.tasks[0]).toMatchObject({ assigneeUserId: "user-9" });
  });

  it("clears the assignee when the event says it is empty", () => {
    const store = makeAssigneeStore("user-7");

    registerTasksHandlers(store)["task.updated"]!(
      makeMessage({ ...makeTask("t1", "session-1"), assignee_user_id: "" }),
    );

    expect(store.getState().kanban.tasks[0]).toMatchObject({ assigneeUserId: undefined });
  });
});
