import { beforeEach, describe, expect, it, vi } from "vitest";
import type { StoreApi } from "zustand";
import { removeRecentTask } from "@/lib/recent-tasks";
import type { AppState } from "@/lib/state/store";
import { registerTasksHandlers } from "./tasks";

vi.mock("@/lib/recent-tasks", () => ({
  removeRecentTask: vi.fn(),
}));

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
    tasks: { activeTaskId: null, activeSessionId: null, pinnedSessionId: null },
    taskSessionsByTask: { itemsByTaskId: {}, loadedByTaskId: {}, loadingByTaskId: {} },
    environmentIdBySessionId: {},
    setActiveSession: vi.fn((taskId: string, sessionId: string | null) => {
      state = {
        ...state,
        tasks: {
          activeTaskId: taskId,
          activeSessionId: sessionId,
          pinnedSessionId: sessionId,
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

function makeDeletedMessage(payload: Record<string, unknown>) {
  return {
    id: "msg-1",
    type: "notification" as const,
    action: "task.deleted" as const,
    payload,
  } as Parameters<NonNullable<ReturnType<typeof registerTasksHandlers>["task.deleted"]>>[0];
}

describe("task.updated primary-session focus follow", () => {
  let store: ReturnType<typeof makeStore>;
  let setActiveSessionAuto: ReturnType<typeof vi.fn>;

  beforeEach(() => {
    setActiveSessionAuto = vi.fn();
    vi.mocked(removeRecentTask).mockClear();
  });

  it("follows focus to the new primary when the user is on the previous primary", () => {
    store = makeStore({
      kanban: {
        workflowId: "wf1",
        steps: [],
        tasks: [{ id: "t1", primarySessionId: "sess-old", workflowId: "wf1" }],
      } as unknown as AppState["kanban"],
      tasks: { activeTaskId: "t1", activeSessionId: "sess-old", pinnedSessionId: null },
      setActiveSessionAuto,
    });

    const handlers = registerTasksHandlers(store);
    handlers["task.updated"]!(makeMessage(makeTask("t1", "sess-new")));

    expect(setActiveSessionAuto).toHaveBeenCalledTimes(1);
    expect(setActiveSessionAuto).toHaveBeenCalledWith("t1", "sess-new");
  });

  it("does NOT follow focus when the user is on a different session than the previous primary", () => {
    // User manually selected sess-other; primary swapping shouldn't yank them away.
    store = makeStore({
      kanban: {
        workflowId: "wf1",
        steps: [],
        tasks: [{ id: "t1", primarySessionId: "sess-old", workflowId: "wf1" }],
      } as unknown as AppState["kanban"],
      tasks: { activeTaskId: "t1", activeSessionId: "sess-other", pinnedSessionId: "sess-other" },
      setActiveSessionAuto,
    });

    const handlers = registerTasksHandlers(store);
    handlers["task.updated"]!(makeMessage(makeTask("t1", "sess-new")));

    expect(setActiveSessionAuto).not.toHaveBeenCalled();
  });

  it("does NOT follow focus when the user is viewing a different task", () => {
    store = makeStore({
      kanban: {
        workflowId: "wf1",
        steps: [],
        tasks: [{ id: "t1", primarySessionId: "sess-old", workflowId: "wf1" }],
      } as unknown as AppState["kanban"],
      tasks: { activeTaskId: "t2", activeSessionId: "sess-old", pinnedSessionId: null },
      setActiveSessionAuto,
    });

    const handlers = registerTasksHandlers(store);
    handlers["task.updated"]!(makeMessage(makeTask("t1", "sess-new")));

    expect(setActiveSessionAuto).not.toHaveBeenCalled();
  });

  it("does NOT call setActiveSessionAuto when the primary did not change", () => {
    store = makeStore({
      kanban: {
        workflowId: "wf1",
        steps: [],
        tasks: [{ id: "t1", primarySessionId: "sess-old", workflowId: "wf1" }],
      } as unknown as AppState["kanban"],
      tasks: { activeTaskId: "t1", activeSessionId: "sess-old", pinnedSessionId: null },
      setActiveSessionAuto,
    });

    const handlers = registerTasksHandlers(store);
    handlers["task.updated"]!(makeMessage(makeTask("t1", "sess-old")));

    expect(setActiveSessionAuto).not.toHaveBeenCalled();
  });

  // Regression: even when the user happens to be sitting on the previous
  // primary, an explicit pin on it must override primary-follow-focus —
  // otherwise a workflow profile switch silently yanks them off the session
  // they deliberately clicked into.
  it("does NOT follow focus when the user has pinned the previous primary", () => {
    store = makeStore({
      kanban: {
        workflowId: "wf1",
        steps: [],
        tasks: [{ id: "t1", primarySessionId: "sess-old", workflowId: "wf1" }],
      } as unknown as AppState["kanban"],
      tasks: { activeTaskId: "t1", activeSessionId: "sess-old", pinnedSessionId: "sess-old" },
      setActiveSessionAuto,
    });

    const handlers = registerTasksHandlers(store);
    handlers["task.updated"]!(makeMessage(makeTask("t1", "sess-new")));

    expect(setActiveSessionAuto).not.toHaveBeenCalled();
  });
});

describe("task.updated cross-workflow placement", () => {
  it("removes the task from its old workflow snapshot before upserting into the new one", () => {
    const task = { id: "t1", title: "Test", workflowId: "wf1", workflowStepId: "step1" };
    const store = makeStore({
      kanban: {
        workflowId: "wf1",
        steps: [],
        tasks: [task],
      } as unknown as AppState["kanban"],
      kanbanMulti: {
        isLoading: false,
        snapshots: {
          wf1: { workflow: { id: "wf1" }, steps: [], tasks: [task] },
          wf2: { workflow: { id: "wf2" }, steps: [], tasks: [] },
        },
      } as unknown as AppState["kanbanMulti"],
    });

    const handlers = registerTasksHandlers(store);
    handlers["task.updated"]!(
      makeMessage({ ...makeTask("t1", null, "wf2"), old_workflow_id: "wf1" }),
    );

    const state = store.getState();
    expect(state.kanban.tasks).toHaveLength(0);
    expect(state.kanbanMulti.snapshots.wf1.tasks).toHaveLength(0);
    expect(state.kanbanMulti.snapshots.wf2.tasks).toHaveLength(1);
    expect(state.kanbanMulti.snapshots.wf2.tasks[0]?.id).toBe("t1");
    expect(state.kanbanMulti.snapshots.wf2.tasks[0]?.workflowStepId).toBe("step1");
  });
});

describe("task.deleted cleanup", () => {
  it("removes the deleted task from recent task history", () => {
    const store = makeStore({
      kanban: {
        workflowId: "wf1",
        steps: [],
        tasks: [{ id: "t1", primarySessionId: "sess-old", workflowId: "wf1" }],
      } as unknown as AppState["kanban"],
      environmentIdBySessionId: {},
    });

    const handlers = registerTasksHandlers(store);
    handlers["task.deleted"]!(
      makeDeletedMessage({
        task_id: "t1",
        workflow_id: "wf1",
      }),
    );

    expect(removeRecentTask).toHaveBeenCalledTimes(1);
    expect(removeRecentTask).toHaveBeenCalledWith("t1");
  });
});
