import { describe, expect, it, vi } from "vitest";
import type { StoreApi } from "zustand";
import type { AppState } from "@/lib/state/store";
import { registerTasksHandlers } from "./tasks";

vi.mock("@/lib/recent-tasks", () => ({
  removeRecentTask: vi.fn(),
}));

type Listener = (state: AppState) => void;
const TASK_ID = "t1";

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
    setActiveSessionAuto: vi.fn(),
    removeTaskFromSidebarPrefs: vi.fn(),
    setTaskDeletedNotification: vi.fn(),
    setOfficeRefetchTrigger: vi.fn(),
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

function makeUpdatedMessage(payload: Record<string, unknown>) {
  return {
    id: "msg-1",
    type: "notification" as const,
    action: "task.updated" as const,
    payload,
  } as Parameters<NonNullable<ReturnType<typeof registerTasksHandlers>["task.updated"]>>[0];
}

// Unarchive is delivered as task.updated with archived_at back to null. The
// handler must re-add the task to the kanban caches (mirror of the archive
// removal path).
describe("task.updated unarchive restore", () => {
  it("re-adds the task to the active kanban when archived_at clears", () => {
    const store = makeStore();
    const handlers = registerTasksHandlers(store);

    handlers["task.updated"]!(
      makeUpdatedMessage({
        task_id: TASK_ID,
        workflow_id: "wf1",
        workflow_step_id: "step1",
        title: "Restored task",
        state: "TODO",
        is_ephemeral: false,
        archived_at: null,
      }),
    );

    const state = store.getState();
    expect(state.kanban.tasks.map((t) => t.id)).toContain(TASK_ID);
  });

  it("re-adds the task to a multi-kanban snapshot when archived_at clears", () => {
    const store = makeStore({
      kanban: { workflowId: "wf-other", steps: [], tasks: [] } as unknown as AppState["kanban"],
      kanbanMulti: {
        isLoading: false,
        snapshots: {
          wf1: { workflowId: "wf1", workflowName: "WF1", steps: [], tasks: [] },
        },
      } as unknown as AppState["kanbanMulti"],
    });
    const handlers = registerTasksHandlers(store);

    handlers["task.updated"]!(
      makeUpdatedMessage({
        task_id: TASK_ID,
        workflow_id: "wf1",
        workflow_step_id: "step1",
        title: "Restored task",
        state: "TODO",
        is_ephemeral: false,
        archived_at: null,
      }),
    );

    const state = store.getState();
    expect(state.kanbanMulti.snapshots.wf1.tasks.map((t) => t.id)).toContain(TASK_ID);
  });
});
