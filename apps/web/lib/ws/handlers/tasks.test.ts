import { beforeEach, describe, expect, it, vi } from "vitest";
import type { StoreApi } from "zustand";
import type { AppState } from "@/lib/state/store";
import { registerTasksHandlers } from "./tasks";

const SESS_OTHER = "sess-other";
const SESS_DRIFTED = "sess-drifted";
const SESS_PINNED = "sess-pinned";

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
function makeFollowStore(opts: {
  primarySessionId: string | null;
  activeSessionId: string | null;
  pinnedSessionId: string | null;
  setActiveSessionAuto: (taskId: string, sessionId: string) => void;
}) {
  return makeStore({
    kanban: {
      workflowId: "wf1",
      steps: [],
      tasks: [{ id: "t1", primarySessionId: opts.primarySessionId, workflowId: "wf1" }],
    } as unknown as AppState["kanban"],
    tasks: {
      activeTaskId: "t1",
      activeSessionId: opts.activeSessionId,
      pinnedSessionId: opts.pinnedSessionId,
      lastSessionByTaskId: {},
    },
    setActiveSessionAuto: opts.setActiveSessionAuto,
  });
}

describe("task.updated primary-session focus follow", () => {
  let store: ReturnType<typeof makeStore>;
  let setActiveSessionAuto: ReturnType<typeof vi.fn<(taskId: string, sessionId: string) => void>>;

  beforeEach(() => {
    setActiveSessionAuto = vi.fn();
  });

  it("follows focus to the new primary when the user is on the previous primary", () => {
    store = makeFollowStore({
      primarySessionId: "sess-old",
      activeSessionId: "sess-old",
      pinnedSessionId: null,
      setActiveSessionAuto,
    });

    const handlers = registerTasksHandlers(store);
    handlers["task.updated"]!(makeMessage(makeTask("t1", "sess-new")));

    expect(setActiveSessionAuto).toHaveBeenCalledTimes(1);
    expect(setActiveSessionAuto).toHaveBeenCalledWith("t1", "sess-new");
  });

  it("does NOT follow focus when the user is on a different session than the previous primary", () => {
    // User manually selected sess-other; primary swapping shouldn't yank them away.
    store = makeFollowStore({
      primarySessionId: "sess-old",
      activeSessionId: SESS_OTHER,
      pinnedSessionId: SESS_OTHER,
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
    store = makeFollowStore({
      primarySessionId: "sess-old",
      activeSessionId: "sess-old",
      pinnedSessionId: null,
      setActiveSessionAuto,
    });

    const handlers = registerTasksHandlers(store);
    handlers["task.updated"]!(makeMessage(makeTask("t1", "sess-old")));

    expect(setActiveSessionAuto).not.toHaveBeenCalled();
  });
});

describe("task.updated workspace sources", () => {
  it("merges authoritative workspace folders into the kanban task", () => {
    const store = makeStore({
      kanban: { workflowId: "wf1", steps: [], tasks: [{ id: "t1", workflowId: "wf1" }] },
    } as unknown as Partial<AppState>);

    registerTasksHandlers(store)["task.updated"]!(
      makeMessage({
        ...makeTask("t1", "sess-1"),
        repositories: [{ id: "tr-1", repository_id: "repo-1", base_branch: "main", position: 0 }],
        workspace_folders: [
          { id: "folder-1", local_path: "/work/docs", display_name: "docs", position: 0 },
        ],
      }),
    );

    expect(store.getState().kanban.tasks[0].workspaceFolders).toEqual([
      { id: "folder-1", local_path: "/work/docs", display_name: "docs", position: 0 },
    ]);
  });
});

// Regression: even when the user happens to be sitting on the previous
// primary, an explicit pin on it must override primary-follow-focus here.
// A pinned user whose session is genuinely retired is followed via the session
// state-transition handoff once that session reaches a terminal state — not
// from task.updated, where a retirement is indistinguishable from a manual
// "Set as Primary" that leaves the pinned session live.
describe("task.updated primary-session focus follow (pinning)", () => {
  let store: ReturnType<typeof makeStore>;
  let setActiveSessionAuto: ReturnType<typeof vi.fn<(taskId: string, sessionId: string) => void>>;

  beforeEach(() => {
    setActiveSessionAuto = vi.fn();
  });

  it("does NOT follow focus when the user has pinned the previous primary", () => {
    store = makeFollowStore({
      primarySessionId: "sess-old",
      activeSessionId: "sess-old",
      pinnedSessionId: "sess-old",
      setActiveSessionAuto,
    });

    const handlers = registerTasksHandlers(store);
    handlers["task.updated"]!(makeMessage(makeTask("t1", "sess-new")));

    expect(setActiveSessionAuto).not.toHaveBeenCalled();
  });

  it("does NOT follow focus when there was no previous primary (first assignment)", () => {
    // Boundary case: a task.updated that assigns the very first primary must not
    // steal focus — there is no replaced session the user was viewing.
    store = makeFollowStore({
      primarySessionId: null,
      activeSessionId: null,
      pinnedSessionId: null,
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
        tasks: [{ id: "t1", primarySessionId: SESS_DRIFTED, workflowId: "wf1" }],
      } as unknown as AppState["kanban"],
      tasks: {
        activeTaskId: "t1",
        activeSessionId: SESS_DRIFTED,
        pinnedSessionId: SESS_PINNED,
        lastSessionByTaskId: {},
      },
      taskSessions: {
        items: {
          [SESS_PINNED]: { id: SESS_PINNED, task_id: "t1", state: "RUNNING" },
          [SESS_DRIFTED]: { id: SESS_DRIFTED, task_id: "t1", state: "COMPLETED" },
        },
      } as unknown as AppState["taskSessions"],
      setActiveSessionAuto,
    });

    const handlers = registerTasksHandlers(store);
    handlers["task.updated"]!(makeMessage(makeTask("t1", "sess-new")));

    expect(setActiveSessionAuto).not.toHaveBeenCalled();
  });
});

describe("task.updated primary-session focus follow (stale pin cleanup)", () => {
  let store: ReturnType<typeof makeStore>;
  let setActiveSessionAuto: ReturnType<typeof vi.fn<(taskId: string, sessionId: string) => void>>;

  beforeEach(() => {
    setActiveSessionAuto = vi.fn();
  });

  it("clears a terminal orphaned pin when following focus to the new primary", () => {
    store = makeStore({
      kanban: {
        workflowId: "wf1",
        steps: [],
        tasks: [{ id: "t1", primarySessionId: SESS_DRIFTED, workflowId: "wf1" }],
      } as unknown as AppState["kanban"],
      tasks: {
        activeTaskId: "t1",
        activeSessionId: SESS_DRIFTED,
        pinnedSessionId: SESS_PINNED,
        lastSessionByTaskId: {},
      },
      taskSessions: {
        items: {
          [SESS_PINNED]: { id: SESS_PINNED, task_id: "t1", state: "COMPLETED" },
          [SESS_DRIFTED]: { id: SESS_DRIFTED, task_id: "t1", state: "COMPLETED" },
        },
      } as unknown as AppState["taskSessions"],
      setActiveSessionAuto,
    });

    const handlers = registerTasksHandlers(store);
    handlers["task.updated"]!(makeMessage(makeTask("t1", "sess-new")));

    expect(setActiveSessionAuto).toHaveBeenCalledWith("t1", "sess-new");
    expect(store.getState().tasks.pinnedSessionId).toBeNull();
  });

  it("clears a deleted orphaned pin when following focus to the new primary", () => {
    store = makeStore({
      kanban: {
        workflowId: "wf1",
        steps: [],
        tasks: [{ id: "t1", primarySessionId: SESS_DRIFTED, workflowId: "wf1" }],
      } as unknown as AppState["kanban"],
      tasks: {
        activeTaskId: "t1",
        activeSessionId: SESS_DRIFTED,
        pinnedSessionId: SESS_PINNED,
        lastSessionByTaskId: {},
      },
      taskSessions: {
        items: {
          [SESS_DRIFTED]: { id: SESS_DRIFTED, task_id: "t1", state: "COMPLETED" },
        },
      } as unknown as AppState["taskSessions"],
      taskSessionsByTask: {
        itemsByTaskId: {
          t1: [{ id: SESS_DRIFTED, task_id: "t1", state: "COMPLETED" }],
        },
        loadedByTaskId: { t1: true },
        loadingByTaskId: {},
      } as unknown as AppState["taskSessionsByTask"],
      setActiveSessionAuto,
    });

    const handlers = registerTasksHandlers(store);
    handlers["task.updated"]!(makeMessage(makeTask("t1", "sess-new")));

    expect(setActiveSessionAuto).toHaveBeenCalledWith("t1", "sess-new");
    expect(store.getState().tasks.pinnedSessionId).toBeNull();
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
