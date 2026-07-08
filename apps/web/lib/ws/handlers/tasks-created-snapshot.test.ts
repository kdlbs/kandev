import { describe, expect, it, vi } from "vitest";
import type { StoreApi } from "zustand";
import type { AppState } from "@/lib/state/store";
import { registerTasksHandlers } from "./tasks";

type Listener = (state: AppState) => void;

function makeStore(initial: Partial<AppState> = {}) {
  let state = {
    kanban: { workflowId: "wf1", steps: [], tasks: [] },
    kanbanMulti: { snapshots: {}, isLoading: false },
    workflows: {
      activeId: "wf1",
      items: [
        { id: "wf1", workspaceId: "ws1", name: "Active Flow" },
        { id: "wf2", workspaceId: "ws1", name: "Sibling Flow" },
      ],
    },
    tasks: {
      activeTaskId: null,
      activeSessionId: null,
      pinnedSessionId: null,
      lastSessionByTaskId: {},
    },
    taskSessionsByTask: { itemsByTaskId: {}, loadedByTaskId: {}, loadingByTaskId: {} },
    environmentIdBySessionId: {},
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

function makeCreatedMessage(payload: Record<string, unknown>) {
  return {
    id: "msg-1",
    type: "notification" as const,
    action: "task.created" as const,
    payload,
  } as Parameters<NonNullable<ReturnType<typeof registerTasksHandlers>["task.created"]>>[0];
}

describe("task.created sidebar snapshots", () => {
  it("keeps a created subtask visible when its workflow snapshot has not loaded yet", () => {
    const store = makeStore();
    const handlers = registerTasksHandlers(store);

    handlers["task.created"]!(
      makeCreatedMessage({
        task_id: "child-before-snapshot",
        workflow_id: "wf2",
        workflow_step_id: "step2",
        title: "Child before snapshot",
        state: "IN_PROGRESS",
        parent_id: "parent-task",
        is_ephemeral: false,
      }),
    );

    const snapshot = store.getState().kanbanMulti.snapshots.wf2;
    expect(snapshot.workflowName).toBe("Sibling Flow");
    expect(snapshot.steps).toEqual([]);
    expect(snapshot.tasks.map((task) => task.id)).toEqual(["child-before-snapshot"]);
    expect(snapshot.tasks[0].parentTaskId).toBe("parent-task");
  });

  it("keeps the task even when workflow metadata has not loaded yet", () => {
    const store = makeStore({
      workflows: { activeId: null, items: [] },
    } as Partial<AppState>);
    const handlers = registerTasksHandlers(store);

    handlers["task.created"]!(
      makeCreatedMessage({
        task_id: "child-before-workflows",
        workflow_id: "wf-late",
        workflow_step_id: "step-late",
        title: "Child before workflows",
        state: "IN_PROGRESS",
        is_ephemeral: false,
      }),
    );

    const snapshot = store.getState().kanbanMulti.snapshots["wf-late"];
    expect(snapshot.workflowName).toBe("wf-late");
    expect(snapshot.tasks.map((task) => task.id)).toEqual(["child-before-workflows"]);
  });
});
