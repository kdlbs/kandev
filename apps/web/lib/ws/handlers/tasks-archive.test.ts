import { beforeEach, describe, expect, it, vi } from "vitest";
import type { StoreApi } from "zustand";
import { removeRecentTask } from "@/lib/recent-tasks";
import type { AppState } from "@/lib/state/store";
import { registerTasksHandlers } from "./tasks";

vi.mock("@/lib/recent-tasks", () => ({
  removeRecentTask: vi.fn(),
}));

type Listener = (state: AppState) => void;
type KanbanTask = { id: string; title: string; workflowId: string; workflowStepId: string };

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

function taskPayload(id: string, workflowId = "wf1") {
  return {
    task_id: id,
    workflow_id: workflowId,
    workflow_step_id: "step1",
    title: "Test",
    state: "IN_PROGRESS",
    is_ephemeral: false,
  };
}

describe("task.updated archive cleanup", () => {
  beforeEach(() => {
    vi.mocked(removeRecentTask).mockClear();
  });

  it("removes archived tasks from the active kanban cache even when workflow focus changed", () => {
    const staleTask: KanbanTask = {
      id: "t1",
      title: "Test",
      workflowId: "wf1",
      workflowStepId: "step1",
    };
    const store = makeStore({
      kanban: {
        workflowId: "wf-active",
        steps: [],
        tasks: [staleTask],
      } as unknown as AppState["kanban"],
      kanbanMulti: {
        isLoading: false,
        snapshots: {
          wf1: { workflowId: "wf1", workflowName: "WF1", steps: [], tasks: [staleTask] },
        },
      } as unknown as AppState["kanbanMulti"],
    });

    const handlers = registerTasksHandlers(store);
    handlers["task.updated"]!(
      makeUpdatedMessage({
        ...taskPayload("t1", "wf1"),
        archived_at: "2026-06-30T12:00:00Z",
      }),
    );

    const state = store.getState();
    expect(state.kanban.tasks).toEqual([]);
    expect(state.kanbanMulti.snapshots.wf1.tasks).toEqual([]);
  });

  it("clears active task state, recent history, and sidebar prefs for archived task events", () => {
    const store = makeStore({
      kanban: {
        workflowId: "wf1",
        steps: [],
        tasks: [{ id: "t1", primarySessionId: "sess-old", workflowId: "wf1" }],
      } as unknown as AppState["kanban"],
      tasks: {
        activeTaskId: "t1",
        activeSessionId: "sess-old",
        pinnedSessionId: null,
        lastSessionByTaskId: { t1: "sess-old", t2: "sess-other" },
      },
      environmentIdBySessionId: {},
    } as unknown as Partial<AppState>);

    const handlers = registerTasksHandlers(store);
    handlers["task.updated"]!(
      makeUpdatedMessage({
        ...taskPayload("t1"),
        archived_at: "2026-06-30T12:00:00Z",
      }),
    );

    const state = store.getState();
    expect(state.tasks.activeTaskId).toBeNull();
    expect(state.tasks.activeSessionId).toBeNull();
    expect(state.tasks.lastSessionByTaskId).not.toHaveProperty("t1");
    expect(state.tasks.lastSessionByTaskId).toHaveProperty("t2", "sess-other");
    expect(removeRecentTask).toHaveBeenCalledWith("t1");
    expect(state.removeTaskFromSidebarPrefs).toHaveBeenCalledWith("t1");
  });
});
