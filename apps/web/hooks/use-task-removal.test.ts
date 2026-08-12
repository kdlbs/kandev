import { describe, it, expect, vi, beforeEach } from "vitest";
import { renderHook } from "@testing-library/react";
import type { StoreApi } from "zustand";

const replaceTaskUrlMock = vi.fn();
const performLayoutSwitchMock = vi.fn();
const listTaskSessionsMock = vi.fn();
const fetchTaskMock = vi.fn();
const OVERVIEW_URL = "/?home=overview";

vi.mock("@/lib/links", () => ({
  linkToTaskOverview: () => "/?home=overview",
  replaceTaskUrl: (...args: unknown[]) => replaceTaskUrlMock(...args),
}));

vi.mock("@/lib/state/dockview-store", () => ({
  performLayoutSwitch: (...args: unknown[]) => performLayoutSwitchMock(...args),
}));

vi.mock("@/lib/api", () => ({
  listTaskSessions: (...args: unknown[]) => listTaskSessionsMock(...args),
  fetchTask: (...args: unknown[]) => fetchTaskMock(...args),
}));

import { useTaskRemoval, selectNextTaskAfterRemoval } from "./use-task-removal";
import { setRecentTasks } from "@/lib/recent-tasks";

type TaskRow = {
  id: string;
  primarySessionId: string | null;
  parentTaskId?: string | null;
};

const CASCADE_PARENT: TaskRow = { id: "task-parent", primarySessionId: "sess-parent" };
const CASCADE_CHILD: TaskRow = {
  id: "task-child",
  primarySessionId: "sess-child",
  parentTaskId: CASCADE_PARENT.id,
};
const CASCADE_DEEP_CHILD: TaskRow = {
  id: "task-deep-child",
  primarySessionId: "sess-deep-child",
  parentTaskId: CASCADE_CHILD.id,
};

const RECENT_TASK_VISITED_AT = "2026-06-07T10:00:00Z";
const REMOVED_TASK_VISITED_AT = "2026-06-07T09:00:00Z";
const OLD_TASK_VISITED_AT = "2026-06-06T10:00:00Z";
const CASCADE_CHILD_VISITED_AT = "2026-06-07T12:00:00Z";
const CASCADE_DEEP_CHILD_VISITED_AT = "2026-06-07T11:00:00Z";

type FakeState = {
  tasks: { activeTaskId: string | null; activeSessionId: string | null };
  kanban: { tasks: TaskRow[] };
  kanbanMulti: { snapshots: Record<string, { tasks: TaskRow[] }> };
  environmentIdBySessionId: Record<string, string>;
  taskSessionsByTask: {
    itemsByTaskId: Record<string, never[]>;
    loadedByTaskId: Record<string, boolean>;
    loadingByTaskId: Record<string, boolean>;
  };
  setActiveTask: ReturnType<typeof vi.fn>;
  setActiveSession: ReturnType<typeof vi.fn>;
  setWorkflowSnapshot: ReturnType<typeof vi.fn>;
  setTaskSessionsForTask: ReturnType<typeof vi.fn>;
  setTaskSessionsLoading: ReturnType<typeof vi.fn>;
};

function makeStore(init: {
  activeTaskId: string | null;
  activeSessionId?: string | null;
  remainingTasks: TaskRow[];
  canonicalTasks?: TaskRow[];
}): StoreApi<FakeState> & { getRecorded: () => FakeState } {
  const state: FakeState = {
    tasks: {
      activeTaskId: init.activeTaskId,
      activeSessionId: init.activeSessionId ?? null,
    },
    kanban: { tasks: init.canonicalTasks ?? [] },
    kanbanMulti: {
      snapshots: { "wf-1": { tasks: init.remainingTasks } },
    },
    environmentIdBySessionId: { "sess-next": "env-next", "sess-A": "env-A" },
    taskSessionsByTask: {
      itemsByTaskId: {},
      loadedByTaskId: {},
      loadingByTaskId: {},
    },
    setActiveTask: vi.fn() as ReturnType<typeof vi.fn>,
    setActiveSession: vi.fn() as ReturnType<typeof vi.fn>,
    setWorkflowSnapshot: vi.fn((wfId: string, snapshot: { tasks: TaskRow[] }) => {
      state.kanbanMulti.snapshots[wfId] = snapshot;
    }) as unknown as ReturnType<typeof vi.fn>,
    setTaskSessionsForTask: vi.fn() as ReturnType<typeof vi.fn>,
    setTaskSessionsLoading: vi.fn() as ReturnType<typeof vi.fn>,
  };

  const api: StoreApi<FakeState> = {
    getState: () => state,
    setState: (updater: unknown) => {
      const next =
        typeof updater === "function"
          ? (updater as (s: FakeState) => FakeState)(state)
          : (updater as FakeState);
      Object.assign(state, next);
    },
    subscribe: () => () => {},
    getInitialState: () => state,
  } as unknown as StoreApi<FakeState>;

  return Object.assign(api, { getRecorded: () => state }) as StoreApi<FakeState> & {
    getRecorded: () => FakeState;
  };
}

function mockLocation() {
  const hrefSetter = vi.fn();
  const originalLocation = window.location;
  Object.defineProperty(window, "location", {
    configurable: true,
    value: {
      get href() {
        return "";
      },
      set href(value: string) {
        hrefSetter(value);
      },
    },
  });
  return {
    hrefSetter,
    restore: () =>
      Object.defineProperty(window, "location", {
        configurable: true,
        value: originalLocation,
      }),
  };
}

const nextTask: TaskRow = { id: "task-next", primarySessionId: "sess-next" };
const recentTaskId = "task-recent";
const recentSessionId = "sess-recent";

beforeEach(() => {
  vi.clearAllMocks();
  fetchTaskMock.mockResolvedValue({ archived_at: null });
  window.localStorage.clear();
});

function makeStoreWithOldAndRecentTasks(activeTaskId: string | null) {
  const oldTask = { id: "task-old", primarySessionId: "sess-old" };
  const recentTask = { id: recentTaskId, primarySessionId: recentSessionId };
  const store = makeStore({
    activeTaskId,
    activeSessionId: "sess-A",
    remainingTasks: [{ id: "task-A", primarySessionId: "sess-A" }, oldTask, recentTask],
  });
  store.getRecorded().environmentIdBySessionId = {
    ...store.getRecorded().environmentIdBySessionId,
    "sess-old": "env-old",
    [recentSessionId]: "env-recent",
  };
  return store;
}

function setRecentTaskFirst() {
  setRecentTasks([
    {
      taskId: recentTaskId,
      title: "Recent task",
      visitedAt: RECENT_TASK_VISITED_AT,
    },
    {
      taskId: "task-A",
      title: "Removed task",
      visitedAt: REMOVED_TASK_VISITED_AT,
    },
    {
      taskId: "task-old",
      title: "Old task",
      visitedAt: OLD_TASK_VISITED_AT,
    },
  ]);
}

function setRemovedTaskFirst() {
  setRecentTasks([
    {
      taskId: "task-A",
      title: "Removed task",
      visitedAt: RECENT_TASK_VISITED_AT,
    },
    {
      taskId: recentTaskId,
      title: "Recent task",
      visitedAt: REMOVED_TASK_VISITED_AT,
    },
    {
      taskId: "task-old",
      title: "Old task",
      visitedAt: OLD_TASK_VISITED_AT,
    },
  ]);
}

describe("useTaskRemoval — switch guard (current store wins)", () => {
  it("switches to next task when activeTaskId === taskId (user still on removed task)", async () => {
    const store = makeStore({
      activeTaskId: "task-A",
      activeSessionId: "sess-A",
      remainingTasks: [{ id: "task-A", primarySessionId: "sess-A" }, nextTask],
    });
    const { result } = renderHook(() =>
      useTaskRemoval({ store: store as unknown as StoreApi<never> }),
    );

    await result.current.removeTaskFromBoard("task-A", {
      wasActiveTaskId: "task-A",
      wasActiveSessionId: "sess-A",
    });

    expect(store.getRecorded().setActiveSession).toHaveBeenCalledWith("task-next", "sess-next");
    expect(replaceTaskUrlMock).toHaveBeenCalledWith("task-next");
  });

  it("does NOT switch when user manually moved to a different task during in-flight archive", async () => {
    const store = makeStore({
      activeTaskId: "task-B",
      activeSessionId: "sess-B",
      remainingTasks: [{ id: "task-B", primarySessionId: "sess-B" }, nextTask],
    });
    const { result } = renderHook(() =>
      useTaskRemoval({ store: store as unknown as StoreApi<never> }),
    );

    await result.current.removeTaskFromBoard("task-A", {
      wasActiveTaskId: "task-A",
      wasActiveSessionId: "sess-A",
    });

    expect(store.getRecorded().setActiveSession).not.toHaveBeenCalled();
    expect(store.getRecorded().setActiveTask).not.toHaveBeenCalled();
    expect(replaceTaskUrlMock).not.toHaveBeenCalled();
  });
});

describe("useTaskRemoval — switch guard (WS-clear fallback)", () => {
  it("switches when WS cleared activeTaskId AND wasActiveTaskId matches removed task", async () => {
    const store = makeStore({
      activeTaskId: null,
      activeSessionId: null,
      remainingTasks: [nextTask],
    });
    const { result } = renderHook(() =>
      useTaskRemoval({ store: store as unknown as StoreApi<never> }),
    );

    await result.current.removeTaskFromBoard("task-A", {
      wasActiveTaskId: "task-A",
      wasActiveSessionId: "sess-A",
    });

    expect(store.getRecorded().setActiveSession).toHaveBeenCalledWith("task-next", "sess-next");
    expect(replaceTaskUrlMock).toHaveBeenCalledWith("task-next");
  });

  it("does NOT switch when no opts provided and WS already cleared activeTaskId", async () => {
    const store = makeStore({
      activeTaskId: null,
      activeSessionId: null,
      remainingTasks: [nextTask],
    });
    const { result } = renderHook(() =>
      useTaskRemoval({ store: store as unknown as StoreApi<never> }),
    );

    await result.current.removeTaskFromBoard("task-A");

    expect(store.getRecorded().setActiveSession).not.toHaveBeenCalled();
    expect(store.getRecorded().setActiveTask).not.toHaveBeenCalled();
    expect(replaceTaskUrlMock).not.toHaveBeenCalled();
  });

  it("does NOT switch when activeTaskId is null AND wasActiveTaskId does not match removed task", async () => {
    const store = makeStore({
      activeTaskId: null,
      activeSessionId: null,
      remainingTasks: [nextTask],
    });
    const { result } = renderHook(() =>
      useTaskRemoval({ store: store as unknown as StoreApi<never> }),
    );

    await result.current.removeTaskFromBoard("task-A", {
      wasActiveTaskId: "task-B",
      wasActiveSessionId: "sess-B",
    });

    expect(store.getRecorded().setActiveSession).not.toHaveBeenCalled();
    expect(store.getRecorded().setActiveTask).not.toHaveBeenCalled();
    expect(replaceTaskUrlMock).not.toHaveBeenCalled();
  });

  it("redirects to the explicit overview when no remaining tasks AND user is still on removed task", async () => {
    const store = makeStore({
      activeTaskId: "task-A",
      activeSessionId: "sess-A",
      remainingTasks: [{ id: "task-A", primarySessionId: "sess-A" }],
    });
    const hrefSetter = vi.fn();
    const originalLocation = window.location;
    Object.defineProperty(window, "location", {
      configurable: true,
      value: {
        get href() {
          return "";
        },
        set href(value: string) {
          hrefSetter(value);
        },
      },
    });

    try {
      const { result } = renderHook(() =>
        useTaskRemoval({ store: store as unknown as StoreApi<never> }),
      );
      const removeResult = await result.current.removeTaskFromBoard("task-A", {
        wasActiveTaskId: "task-A",
        wasActiveSessionId: "sess-A",
      });
      expect(removeResult.switchedTaskId).toBeNull();
      expect(hrefSetter).toHaveBeenCalledWith(OVERVIEW_URL);
    } finally {
      Object.defineProperty(window, "location", {
        configurable: true,
        value: originalLocation,
      });
    }
  });
});

describe("useTaskRemoval — next task selection", () => {
  it("redirects home when a stale snapshot candidate is absent from the canonical board", async () => {
    const staleTask = { id: "task-stale", primarySessionId: "sess-stale" };
    const store = makeStore({
      activeTaskId: "task-A",
      activeSessionId: "sess-A",
      remainingTasks: [{ id: "task-A", primarySessionId: "sess-A" }, staleTask],
      canonicalTasks: [],
    });
    fetchTaskMock.mockRejectedValueOnce(new Error("task not found"));
    const hrefSetter = vi.fn();
    const originalLocation = window.location;
    Object.defineProperty(window, "location", {
      configurable: true,
      value: {
        get href() {
          return "";
        },
        set href(value: string) {
          hrefSetter(value);
        },
      },
    });

    try {
      const { result } = renderHook(() =>
        useTaskRemoval({ store: store as unknown as StoreApi<never> }),
      );

      const removal = await result.current.removeTaskFromBoard("task-A", {
        wasActiveTaskId: "task-A",
        wasActiveSessionId: "sess-A",
      });

      expect(removal.switchedTaskId).toBeNull();
      expect(store.getRecorded().setActiveSession).not.toHaveBeenCalled();
      expect(replaceTaskUrlMock).not.toHaveBeenCalled();
      expect(hrefSetter).toHaveBeenCalledWith(OVERVIEW_URL);
    } finally {
      Object.defineProperty(window, "location", {
        configurable: true,
        value: originalLocation,
      });
    }
  });

  it("switches to the most recent remaining task instead of the first snapshot task", async () => {
    const store = makeStoreWithOldAndRecentTasks("task-A");
    setRecentTaskFirst();

    const { result } = renderHook(() =>
      useTaskRemoval({ store: store as unknown as StoreApi<never> }),
    );

    await result.current.removeTaskFromBoard("task-A", {
      wasActiveTaskId: "task-A",
      wasActiveSessionId: "sess-A",
    });

    expect(store.getRecorded().setActiveSession).toHaveBeenCalledWith(
      recentTaskId,
      recentSessionId,
    );
    expect(replaceTaskUrlMock).toHaveBeenCalledWith(recentTaskId);
  });

  it("skips the removed active task when it is first in recent history", async () => {
    const store = makeStoreWithOldAndRecentTasks("task-A");
    setRemovedTaskFirst();

    const { result } = renderHook(() =>
      useTaskRemoval({ store: store as unknown as StoreApi<never> }),
    );

    await result.current.removeTaskFromBoard("task-A", {
      wasActiveTaskId: "task-A",
      wasActiveSessionId: "sess-A",
    });

    expect(store.getRecorded().setActiveSession).toHaveBeenCalledWith(
      recentTaskId,
      recentSessionId,
    );
    expect(replaceTaskUrlMock).toHaveBeenCalledWith(recentTaskId);
  });

  it("can switch before removing the task from board state", async () => {
    const store = makeStoreWithOldAndRecentTasks("task-A");
    setRemovedTaskFirst();

    const { result } = renderHook(() =>
      useTaskRemoval({ store: store as unknown as StoreApi<never> }),
    );

    await result.current.removeTaskFromBoard("task-A", {
      wasActiveTaskId: "task-A",
      wasActiveSessionId: "sess-A",
      switchOnly: true,
    });

    expect(store.getRecorded().setWorkflowSnapshot).not.toHaveBeenCalled();
    expect(store.getRecorded().setActiveSession).toHaveBeenCalledWith(
      recentTaskId,
      recentSessionId,
    );
    expect(replaceTaskUrlMock).toHaveBeenCalledWith(recentTaskId);
  });
});

describe("useTaskRemoval — cascade tree exclusion", () => {
  it("skips a cascade tree collected from workflow and canonical projections", async () => {
    const unrelated: TaskRow = { id: "task-unrelated", primarySessionId: "sess-unrelated" };
    const store = makeStore({
      activeTaskId: CASCADE_PARENT.id,
      activeSessionId: CASCADE_PARENT.primarySessionId,
      remainingTasks: [CASCADE_PARENT, CASCADE_CHILD, CASCADE_DEEP_CHILD, unrelated],
      canonicalTasks: [
        CASCADE_DEEP_CHILD,
        { ...CASCADE_CHILD, parentTaskId: CASCADE_DEEP_CHILD.id },
      ],
    });
    setRecentTasks([
      { taskId: CASCADE_CHILD.id, title: "Child", visitedAt: CASCADE_CHILD_VISITED_AT },
      {
        taskId: CASCADE_DEEP_CHILD.id,
        title: "Deep child",
        visitedAt: CASCADE_DEEP_CHILD_VISITED_AT,
      },
      { taskId: unrelated.id, title: "Unrelated", visitedAt: RECENT_TASK_VISITED_AT },
    ]);

    const { result } = renderHook(() =>
      useTaskRemoval({ store: store as unknown as StoreApi<never> }),
    );

    await result.current.removeTaskFromBoard(CASCADE_PARENT.id, {
      wasActiveTaskId: CASCADE_PARENT.id,
      wasActiveSessionId: CASCADE_PARENT.primarySessionId,
      switchOnly: true,
      excludeTaskTree: true,
    } as never);

    expect(store.getRecorded().setActiveSession).toHaveBeenCalledWith(
      unrelated.id,
      unrelated.primarySessionId,
    );
  });

  it("can still switch to an active child when cascade exclusion is not requested", async () => {
    const store = makeStore({
      activeTaskId: CASCADE_PARENT.id,
      activeSessionId: CASCADE_PARENT.primarySessionId,
      remainingTasks: [CASCADE_PARENT, CASCADE_CHILD],
    });
    setRecentTasks([
      { taskId: CASCADE_CHILD.id, title: "Child", visitedAt: CASCADE_CHILD_VISITED_AT },
    ]);

    const { result } = renderHook(() =>
      useTaskRemoval({ store: store as unknown as StoreApi<never> }),
    );

    await result.current.removeTaskFromBoard(CASCADE_PARENT.id, {
      wasActiveTaskId: CASCADE_PARENT.id,
      wasActiveSessionId: CASCADE_PARENT.primarySessionId,
      switchOnly: true,
    });

    expect(store.getRecorded().setActiveSession).toHaveBeenCalledWith(
      CASCADE_CHILD.id,
      CASCADE_CHILD.primarySessionId,
    );
  });
});

describe("useTaskRemoval — cascade no-candidate transition", () => {
  it("does not navigate home when switch-only mode has no safe candidate", async () => {
    const store = makeStore({
      activeTaskId: CASCADE_PARENT.id,
      activeSessionId: CASCADE_PARENT.primarySessionId,
      remainingTasks: [CASCADE_PARENT, CASCADE_CHILD],
    });
    const location = mockLocation();

    try {
      const { result } = renderHook(() =>
        useTaskRemoval({ store: store as unknown as StoreApi<never> }),
      );

      const removal = await result.current.removeTaskFromBoard(CASCADE_PARENT.id, {
        wasActiveTaskId: CASCADE_PARENT.id,
        wasActiveSessionId: CASCADE_PARENT.primarySessionId,
        switchOnly: true,
        excludeTaskTree: true,
      } as never);

      expect(removal.switchedTaskId).toBeNull();
      expect(store.getRecorded().setActiveSession).not.toHaveBeenCalled();
      expect(location.hrefSetter).not.toHaveBeenCalled();
    } finally {
      location.restore();
    }
  });

  it("opens the overview after final cascade cleanup when no safe task remains", async () => {
    const store = makeStore({
      activeTaskId: CASCADE_PARENT.id,
      activeSessionId: CASCADE_PARENT.primarySessionId,
      remainingTasks: [CASCADE_PARENT, CASCADE_CHILD],
    });
    const location = mockLocation();

    try {
      const { result } = renderHook(() =>
        useTaskRemoval({ store: store as unknown as StoreApi<never> }),
      );

      const removal = await result.current.removeTaskFromBoard(CASCADE_PARENT.id, {
        wasActiveTaskId: CASCADE_PARENT.id,
        wasActiveSessionId: CASCADE_PARENT.primarySessionId,
        excludeTaskTree: true,
      } as never);

      expect(removal.switchedTaskId).toBeNull();
      expect(location.hrefSetter).toHaveBeenCalledWith(OVERVIEW_URL);
    } finally {
      location.restore();
    }
  });
});

describe("selectNextTaskAfterRemoval — stale-candidate rejection", () => {
  // The exported helper takes the full kanban task shape; the tests use the
  // minimal TaskRow, so cast at the boundary exactly as the store-based tests do.
  const select = selectNextTaskAfterRemoval as unknown as (
    remaining: TaskRow[],
    removedTaskId: string,
    isLive: (taskId: string) => Promise<boolean>,
    excludedTaskIds?: ReadonlySet<string>,
  ) => Promise<TaskRow | null>;

  it("skips every excluded descendant while preserving recent-task order", async () => {
    const child: TaskRow = { ...CASCADE_CHILD, parentTaskId: null };
    const deepChild: TaskRow = { ...CASCADE_DEEP_CHILD, parentTaskId: null };
    const unrelated: TaskRow = { id: "task-unrelated", primarySessionId: "sess-unrelated" };
    setRecentTasks([
      { taskId: child.id, title: "Child", visitedAt: CASCADE_CHILD_VISITED_AT },
      { taskId: deepChild.id, title: "Deep child", visitedAt: CASCADE_DEEP_CHILD_VISITED_AT },
      { taskId: unrelated.id, title: "Unrelated", visitedAt: RECENT_TASK_VISITED_AT },
    ]);

    await expect(
      select(
        [child, deepChild, unrelated],
        CASCADE_PARENT.id,
        async () => true,
        new Set([CASCADE_PARENT.id, child.id, deepChild.id]),
      ),
    ).resolves.toBe(unrelated);
  });

  it("returns null (redirect home) when the only remaining candidate is not a live board member", async () => {
    // A task deleted moments earlier can linger in a snapshot that races the
    // local delete. It is not the removed task, but it is no longer a live
    // board member, so it must not be chosen — the caller then redirects home.
    const stale: TaskRow = { id: "task-A-stale", primarySessionId: "sess-A" };

    await expect(select([stale], "task-B", async () => false)).resolves.toBeNull();
  });

  it("returns the candidate when it is a live board member", async () => {
    // Guards against over-rejection: a genuinely present task is still selected.
    const live: TaskRow = { id: "task-A-live", primarySessionId: "sess-A" };

    await expect(select([live], "task-B", async () => true)).resolves.toBe(live);
  });
});
