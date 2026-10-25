import { describe, it, expect, vi } from "vitest";
import type { StoreApi } from "zustand";
import type { AppState } from "@/lib/state/store";
import { registerTasksHandlers } from "./tasks";

// AC-001.6: a task.updated payload that omits workspace_orphaned entirely
// (a lightweight partial delta) must preserve the previously cached value
// rather than silently clearing it. Mirrors the interrupted/autoStartFailed
// preserveOmittedField coverage this handler already relies on.

const WORKFLOW_ID = "wf1";
const TASK_ID = "t1";
const STEP_ID = "step1";

function makeStore(initial: Partial<AppState> = {}) {
  let state = {
    kanban: { workflowId: WORKFLOW_ID, steps: [], tasks: [] },
    kanbanMulti: { snapshots: {}, isLoading: false },
    tasks: {
      activeTaskId: null,
      activeSessionId: null,
      pinnedSessionId: null,
      lastSessionByTaskId: {},
    },
    taskSessionsByTask: { itemsByTaskId: {}, loadedByTaskId: {}, loadingByTaskId: {} },
    environmentIdBySessionId: {},
    setActiveSession: vi.fn(),
    setActiveSessionAuto: vi.fn(),
    removeTaskFromSidebarPrefs: vi.fn(),
    setTaskDeletedNotification: vi.fn(),
    ...initial,
  } as unknown as AppState;

  return {
    getState: () => state,
    setState: (updater: AppState | ((s: AppState) => AppState)) => {
      const next =
        typeof updater === "function" ? (updater as (s: AppState) => AppState)(state) : updater;
      state = { ...state, ...next };
    },
    subscribe: () => () => {},
    destroy: vi.fn(),
    getInitialState: vi.fn(),
  } as unknown as StoreApi<AppState> & { getState: () => AppState };
}

function makeMessage(payload: Record<string, unknown>) {
  return {
    id: "msg-1",
    type: "notification" as const,
    action: "task.updated" as const,
    payload,
  } as Parameters<NonNullable<ReturnType<typeof registerTasksHandlers>["task.updated"]>>[0];
}

function basePayload(overrides: Record<string, unknown> = {}) {
  return {
    task_id: TASK_ID,
    workflow_id: WORKFLOW_ID,
    workflow_step_id: STEP_ID,
    title: "Test",
    description: "",
    state: "TODO",
    is_ephemeral: false,
    ...overrides,
  };
}

describe("task.updated handler — workspaceOrphaned preservation", () => {
  it("preserves a cached true value when the payload omits workspace_orphaned", () => {
    const store = makeStore({
      kanban: {
        workflowId: WORKFLOW_ID,
        steps: [],
        tasks: [
          {
            id: TASK_ID,
            workflowId: WORKFLOW_ID,
            workflowStepId: STEP_ID,
            title: "Test",
            position: 0,
            workspaceOrphaned: true,
          },
        ],
      },
    });

    registerTasksHandlers(store)["task.updated"]!(makeMessage(basePayload()));

    const task = store.getState().kanban.tasks.find((t) => t.id === TASK_ID);
    expect(task?.workspaceOrphaned).toBe(true);
  });

  it("clears the cached value when the payload carries an explicit false", () => {
    const store = makeStore({
      kanban: {
        workflowId: WORKFLOW_ID,
        steps: [],
        tasks: [
          {
            id: TASK_ID,
            workflowId: WORKFLOW_ID,
            workflowStepId: STEP_ID,
            title: "Test",
            position: 0,
            workspaceOrphaned: true,
          },
        ],
      },
    });

    registerTasksHandlers(store)["task.updated"]!(
      makeMessage(basePayload({ workspace_orphaned: false })),
    );

    const task = store.getState().kanban.tasks.find((t) => t.id === TASK_ID);
    expect(task?.workspaceOrphaned).toBe(false);
  });

  it("sets the value from an explicit true on the payload", () => {
    const store = makeStore({
      kanban: {
        workflowId: WORKFLOW_ID,
        steps: [],
        tasks: [
          {
            id: TASK_ID,
            workflowId: WORKFLOW_ID,
            workflowStepId: STEP_ID,
            title: "Test",
            position: 0,
            workspaceOrphaned: false,
          },
        ],
      },
    });

    registerTasksHandlers(store)["task.updated"]!(
      makeMessage(basePayload({ workspace_orphaned: true })),
    );

    const task = store.getState().kanban.tasks.find((t) => t.id === TASK_ID);
    expect(task?.workspaceOrphaned).toBe(true);
  });
});
