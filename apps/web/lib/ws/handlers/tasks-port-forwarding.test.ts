import { describe, expect, it, vi } from "vitest";
import type { StoreApi } from "zustand";
import type { AppState } from "@/lib/state/store";
import { registerTasksHandlers } from "./tasks";

function makeStore() {
  const task = {
    id: "task-1",
    workflowStepId: "step-1",
    title: "Task",
    position: 0,
    metadata: { port_forwarding_enabled: false, unrelated: "preserve" },
  };
  let state = {
    kanban: { workflowId: "wf-1", steps: [], tasks: [task] },
    kanbanMulti: {
      isLoading: false,
      snapshots: {
        "wf-1": {
          workflowId: "wf-1",
          workflowName: "Workflow",
          steps: [],
          tasks: [task],
        },
      },
    },
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
  } as unknown as AppState;

  return {
    getState: () => state,
    setState: (updater: AppState | ((s: AppState) => AppState)) => {
      const next =
        typeof updater === "function" ? (updater as (s: AppState) => AppState)(state) : updater;
      state = { ...state, ...next };
    },
    subscribe: vi.fn(() => vi.fn()),
    destroy: vi.fn(),
    getInitialState: vi.fn(),
  } as unknown as StoreApi<AppState> & { getState: () => AppState };
}

function taskUpdated(payload: Record<string, unknown>) {
  return {
    id: "message-1",
    type: "notification" as const,
    action: "task.updated" as const,
    payload,
  } as Parameters<NonNullable<ReturnType<typeof registerTasksHandlers>["task.updated"]>>[0];
}

function taskPayload(metadata: Record<string, unknown> | null | undefined) {
  const payload = {
    task_id: "task-1",
    workflow_id: "wf-1",
    workflow_step_id: "step-1",
    title: "Task",
    position: 0,
    state: "TODO",
    is_ephemeral: false,
  };
  return metadata === undefined ? payload : { ...payload, metadata };
}

describe("task.updated port-forwarding metadata", () => {
  it("updates the preference in both kanban projections", () => {
    const store = makeStore();
    registerTasksHandlers(store)["task.updated"]!(
      taskUpdated(taskPayload({ port_forwarding_enabled: true, unrelated: "preserve" })),
    );

    expect(store.getState().kanban.tasks[0]?.metadata).toEqual({
      port_forwarding_enabled: true,
      unrelated: "preserve",
    });
    expect(store.getState().kanbanMulti.snapshots["wf-1"]?.tasks[0]?.metadata).toEqual({
      port_forwarding_enabled: true,
      unrelated: "preserve",
    });
  });

  it("preserves metadata when a partial task update omits it", () => {
    const store = makeStore();
    registerTasksHandlers(store)["task.updated"]!(taskUpdated(taskPayload(undefined)));

    expect(store.getState().kanban.tasks[0]?.metadata).toEqual({
      port_forwarding_enabled: false,
      unrelated: "preserve",
    });
  });

  it("allows an explicit null metadata value to clear stale state", () => {
    const store = makeStore();
    registerTasksHandlers(store)["task.updated"]!(taskUpdated(taskPayload(null)));

    expect(store.getState().kanban.tasks[0]?.metadata).toBeNull();
  });
});
