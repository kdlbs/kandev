import { beforeEach, describe, expect, it, vi } from "vitest";
import type { StoreApi } from "zustand";
import type { AppState } from "@/lib/state/store";
import { registerTasksHandlers } from "./tasks";

type Listener = (state: AppState) => void;

function makeStore() {
  let state = {
    kanban: {
      workflowId: "wf1",
      steps: [],
      tasks: [{ id: "t1", primarySessionId: "sess-1", workflowId: "wf1" }],
    },
    kanbanMulti: { snapshots: {}, isLoading: false },
    tasks: {
      activeTaskId: null,
      activeSessionId: null,
      pinnedSessionId: null,
      lastSessionByTaskId: {},
    },
    taskSessionsByTask: { itemsByTaskId: {}, loadedByTaskId: {}, loadingByTaskId: {} },
    environmentIdBySessionId: {},
    walkthroughs: {
      byTaskId: {
        t1: {
          id: "wt-1",
          task_id: "t1",
          title: "Walkthrough",
          steps: [],
          created_by: "agent",
          created_at: "2026-01-01T00:00:00Z",
          updated_at: "2026-01-01T00:00:00Z",
        },
        t2: null,
      },
      activeStepByTaskId: { t1: 2, t2: 1 },
      lastSeenUpdatedAtByTaskId: { t1: "old", t2: "keep" },
    },
    removeTaskFromSidebarPrefs: vi.fn(),
    setTaskDeletedNotification: vi.fn(),
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

function makeDeletedMessage() {
  return {
    id: "msg-1",
    type: "notification" as const,
    action: "task.deleted" as const,
    payload: { task_id: "t1", workflow_id: "wf1" },
  } as Parameters<NonNullable<ReturnType<typeof registerTasksHandlers>["task.deleted"]>>[0];
}

describe("task.deleted walkthrough cleanup", () => {
  beforeEach(() => {
    window.localStorage.clear();
    window.sessionStorage.clear();
  });

  it("clears deleted task walkthrough state without touching other tasks", () => {
    const store = makeStore();
    const handlers = registerTasksHandlers(store);

    handlers["task.deleted"]!(makeDeletedMessage());

    const state = store.getState();
    expect(state.walkthroughs.byTaskId).not.toHaveProperty("t1");
    expect(state.walkthroughs.activeStepByTaskId).not.toHaveProperty("t1");
    expect(state.walkthroughs.lastSeenUpdatedAtByTaskId).not.toHaveProperty("t1");
    expect(state.walkthroughs.byTaskId).toHaveProperty("t2", null);
    expect(state.walkthroughs.lastSeenUpdatedAtByTaskId).toHaveProperty("t2", "keep");
  });
});
