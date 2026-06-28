import { beforeEach, describe, expect, it, vi } from "vitest";
import type { StoreApi } from "zustand";
import { removeRecentTask } from "@/lib/recent-tasks";
import type { AppState } from "@/lib/state/store";
import { registerTasksHandlers } from "./tasks";

vi.mock("@/lib/recent-tasks", () => ({
  removeRecentTask: vi.fn(),
}));

const SESS_OTHER = "sess-other";

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
      tasks: {
        activeTaskId: "t1",
        activeSessionId: "sess-old",
        pinnedSessionId: null,
        lastSessionByTaskId: {},
      },
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
      tasks: {
        activeTaskId: "t1",
        activeSessionId: SESS_OTHER,
        pinnedSessionId: SESS_OTHER,
        lastSessionByTaskId: {},
      },
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
      tasks: {
        activeTaskId: "t2",
        activeSessionId: "sess-old",
        pinnedSessionId: null,
        lastSessionByTaskId: {},
      },
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
      tasks: {
        activeTaskId: "t1",
        activeSessionId: "sess-old",
        pinnedSessionId: null,
        lastSessionByTaskId: {},
      },
      setActiveSessionAuto,
    });

    const handlers = registerTasksHandlers(store);
    handlers["task.updated"]!(makeMessage(makeTask("t1", "sess-old")));

    expect(setActiveSessionAuto).not.toHaveBeenCalled();
  });
});

// Regression: even when the user happens to be sitting on the previous
// primary, an explicit pin on it must override primary-follow-focus —
// otherwise a workflow profile switch silently yanks them off the session
// they deliberately clicked into.
describe("task.updated primary-session focus follow (pinning)", () => {
  let store: ReturnType<typeof makeStore>;
  let setActiveSessionAuto: ReturnType<typeof vi.fn>;

  beforeEach(() => {
    setActiveSessionAuto = vi.fn();
    vi.mocked(removeRecentTask).mockClear();
  });

  it("does NOT follow focus when the user has pinned the previous primary", () => {
    store = makeStore({
      kanban: {
        workflowId: "wf1",
        steps: [],
        tasks: [{ id: "t1", primarySessionId: "sess-old", workflowId: "wf1" }],
      } as unknown as AppState["kanban"],
      tasks: {
        activeTaskId: "t1",
        activeSessionId: "sess-old",
        pinnedSessionId: "sess-old",
        lastSessionByTaskId: {},
      },
      setActiveSessionAuto,
    });

    const handlers = registerTasksHandlers(store);
    handlers["task.updated"]!(makeMessage(makeTask("t1", "sess-new")));

    expect(setActiveSessionAuto).not.toHaveBeenCalled();
  });

  it("does NOT follow focus when active-session drift orphaned a non-terminal pin", () => {
    store = makeStore({
      kanban: {
        workflowId: "wf1",
        steps: [],
        tasks: [{ id: "t1", primarySessionId: "sess-drifted", workflowId: "wf1" }],
      } as unknown as AppState["kanban"],
      tasks: {
        activeTaskId: "t1",
        activeSessionId: "sess-drifted",
        pinnedSessionId: "sess-pinned",
        lastSessionByTaskId: {},
      },
      taskSessions: {
        items: {
          "sess-pinned": { id: "sess-pinned", task_id: "t1", state: "RUNNING" },
          "sess-drifted": { id: "sess-drifted", task_id: "t1", state: "COMPLETED" },
        },
      } as unknown as AppState["taskSessions"],
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

describe("task.updated repository preservation", () => {
  it("preserves repository metadata when a rename update omits repo fields", () => {
    const repo = {
      id: "task-repo-1",
      repository_id: "repo-a",
      base_branch: "main",
      checkout_branch: "feature/rename",
      position: 0,
    };
    const existingTask = {
      id: "t1",
      workflowStepId: "step1",
      title: "Old title",
      position: 0,
      repositoryId: "repo-a",
      repositories: [repo],
    };
    const store = makeStore({
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

    const handlers = registerTasksHandlers(store);
    handlers["task.updated"]!(
      makeMessage({
        ...makeTask("t1", null),
        title: "Renamed task",
        repository_id: undefined,
        repositories: undefined,
      }),
    );

    const state = store.getState();
    const kanbanTask = state.kanban.tasks.find((task) => task.id === "t1");
    const snapshotTask = state.kanbanMulti.snapshots.wf1.tasks.find((task) => task.id === "t1");
    expect(kanbanTask?.title).toBe("Renamed task");
    expect(kanbanTask?.repositoryId).toBe("repo-a");
    expect(kanbanTask?.repositories).toEqual([repo]);
    expect(snapshotTask?.repositoryId).toBe("repo-a");
    expect(snapshotTask?.repositories).toEqual([repo]);
  });

  it("does not preserve stale repository rows when the primary repository changes", () => {
    const existingTask = {
      id: "t1",
      workflowStepId: "step1",
      title: "Old title",
      position: 0,
      repositoryId: "repo-a",
      repositories: [
        {
          id: "task-repo-1",
          repository_id: "repo-a",
          base_branch: "main",
          position: 0,
        },
      ],
    };
    const store = makeStore({
      kanban: {
        workflowId: "wf1",
        steps: [],
        tasks: [existingTask],
      } as unknown as AppState["kanban"],
    });

    const handlers = registerTasksHandlers(store);
    handlers["task.updated"]!(
      makeMessage({
        ...makeTask("t1", null),
        repository_id: "repo-b",
        repositories: undefined,
      }),
    );

    const task = store.getState().kanban.tasks.find((item) => item.id === "t1");
    expect(task?.repositoryId).toBe("repo-b");
    expect(task?.repositories).toBeUndefined();
  });
});

describe("task.updated repository clearing", () => {
  it("clears repository metadata when an update explicitly sends an empty repository list", () => {
    const existingTask = {
      id: "t1",
      workflowStepId: "step1",
      title: "Old title",
      position: 0,
      repositoryId: "repo-a",
      repositories: [
        {
          id: "task-repo-1",
          repository_id: "repo-a",
          base_branch: "main",
          position: 0,
        },
      ],
    };
    const store = makeStore({
      kanban: {
        workflowId: "wf1",
        steps: [],
        tasks: [existingTask],
      } as unknown as AppState["kanban"],
    });

    const handlers = registerTasksHandlers(store);
    handlers["task.updated"]!(
      makeMessage({
        ...makeTask("t1", null),
        repositories: [],
      }),
    );

    const task = store.getState().kanban.tasks.find((item) => item.id === "t1");
    expect(task?.repositoryId).toBeUndefined();
    expect(task?.repositories).toEqual([]);
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

  it("removes the deleted task from lastSessionByTaskId", () => {
    const store = makeStore({
      kanban: {
        workflowId: "wf1",
        steps: [],
        tasks: [{ id: "t1", primarySessionId: "sess-old", workflowId: "wf1" }],
      } as unknown as AppState["kanban"],
      tasks: {
        activeTaskId: null,
        activeSessionId: null,
        pinnedSessionId: null,
        lastSessionByTaskId: { t1: "sess-old", t2: SESS_OTHER },
      },
      environmentIdBySessionId: {},
    });

    const handlers = registerTasksHandlers(store);
    handlers["task.deleted"]!(makeDeletedMessage({ task_id: "t1", workflow_id: "wf1" }));

    const state = store.getState();
    expect(state.tasks.lastSessionByTaskId).not.toHaveProperty("t1");
    expect(state.tasks.lastSessionByTaskId).toHaveProperty("t2", SESS_OTHER);
  });
});
