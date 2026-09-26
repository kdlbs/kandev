import { describe, it, expect, vi, beforeEach } from "vitest";
import { renderHook } from "@testing-library/react";
import type { StoreApi } from "zustand";

const performLayoutSwitchMock = vi.fn();
const listTaskSessionsMock = vi.fn();
const fetchTaskMock = vi.fn();
const querySidebarTasksMock = vi.fn();
const softNavigateMock = vi.fn();
const OVERVIEW_URL = "/?home=overview";

vi.mock("@/lib/links", () => ({
  linkToTask: (taskId: string) => `/t/${taskId}`,
  linkToTaskOverview: () => "/?home=overview",
}));

vi.mock("@/lib/state/dockview-store", () => ({
  performLayoutSwitch: (...args: unknown[]) => performLayoutSwitchMock(...args),
}));

vi.mock("@/lib/api", () => ({
  listTaskSessions: (...args: unknown[]) => listTaskSessionsMock(...args),
  fetchTask: (...args: unknown[]) => fetchTaskMock(...args),
  querySidebarTasks: (...args: unknown[]) => querySidebarTasksMock(...args),
}));

vi.mock("@/lib/routing/client-router", () => ({
  softNavigate: (...args: unknown[]) => softNavigateMock(...args),
}));

import { useTaskRemoval, selectNextTaskAfterRemoval } from "./use-task-removal";
import { setRecentTasks } from "@/lib/recent-tasks";
import type { SidebarTaskPageResponse, Task } from "@/lib/types/http";
import { makeTaskRemovalStore, type TaskRow } from "./use-task-removal.test-utils";

const TASK_A_ID = "task-A";
const OFF_PAGE_TASK_ID = "task-off-page";
const OFF_PAGE_SESSION_ID = "sess-off-page";
const WORKSPACE_ID = "workspace-1";
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

function makeFallbackTask(title: string): Task {
  return {
    id: OFF_PAGE_TASK_ID,
    workspace_id: WORKSPACE_ID,
    workflow_id: "wf-1",
    workflow_step_id: "step-1",
    title,
    state: "IN_PROGRESS",
    priority: "medium",
    position: 0,
    primary_session_id: OFF_PAGE_SESSION_ID,
    created_at: "2026-09-26T10:00:00Z",
    updated_at: "2026-09-26T10:00:00Z",
  } as Task;
}

const nextTask: TaskRow = { id: "task-next", primarySessionId: "sess-next" };
const recentTaskId = "task-recent";
const recentSessionId = "sess-recent";

beforeEach(() => {
  vi.clearAllMocks();
  querySidebarTasksMock.mockReset();
  fetchTaskMock.mockResolvedValue({ archived_at: null });
  window.localStorage.clear();
});

function makeStoreWithOldAndRecentTasks(activeTaskId: string | null) {
  const oldTask = { id: "task-old", primarySessionId: "sess-old" };
  const recentTask = { id: recentTaskId, primarySessionId: recentSessionId };
  const store = makeTaskRemovalStore({
    activeTaskId,
    activeSessionId: "sess-A",
    remainingTasks: [{ id: TASK_A_ID, primarySessionId: "sess-A" }, oldTask, recentTask],
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
      taskId: TASK_A_ID,
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
      taskId: TASK_A_ID,
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
    const store = makeTaskRemovalStore({
      activeTaskId: TASK_A_ID,
      activeSessionId: "sess-A",
      remainingTasks: [{ id: TASK_A_ID, primarySessionId: "sess-A" }, nextTask],
    });
    const { result } = renderHook(() =>
      useTaskRemoval({ store: store as unknown as StoreApi<never> }),
    );

    await result.current.removeTaskFromBoard(TASK_A_ID, {
      wasActiveTaskId: TASK_A_ID,
      wasActiveSessionId: "sess-A",
    });

    expect(store.getRecorded().setActiveSession).toHaveBeenCalledWith("task-next", "sess-next");
    expect(softNavigateMock).toHaveBeenCalledWith("/t/task-next", "replace");
  });

  it("does NOT switch when user manually moved to a different task during in-flight archive", async () => {
    const store = makeTaskRemovalStore({
      activeTaskId: "task-B",
      activeSessionId: "sess-B",
      remainingTasks: [{ id: "task-B", primarySessionId: "sess-B" }, nextTask],
    });
    const { result } = renderHook(() =>
      useTaskRemoval({ store: store as unknown as StoreApi<never> }),
    );

    await result.current.removeTaskFromBoard(TASK_A_ID, {
      wasActiveTaskId: TASK_A_ID,
      wasActiveSessionId: "sess-A",
    });

    expect(store.getRecorded().setActiveSession).not.toHaveBeenCalled();
    expect(store.getRecorded().setActiveTask).not.toHaveBeenCalled();
    expect(softNavigateMock).not.toHaveBeenCalled();
  });
});

describe("useTaskRemoval — switch guard (WS-clear fallback)", () => {
  it("switches when WS cleared activeTaskId AND wasActiveTaskId matches removed task", async () => {
    const store = makeTaskRemovalStore({
      activeTaskId: null,
      activeSessionId: null,
      remainingTasks: [nextTask],
    });
    const { result } = renderHook(() =>
      useTaskRemoval({ store: store as unknown as StoreApi<never> }),
    );

    await result.current.removeTaskFromBoard(TASK_A_ID, {
      wasActiveTaskId: TASK_A_ID,
      wasActiveSessionId: "sess-A",
    });

    expect(store.getRecorded().setActiveSession).toHaveBeenCalledWith("task-next", "sess-next");
    expect(softNavigateMock).toHaveBeenCalledWith("/t/task-next", "replace");
  });

  it("does NOT switch when no opts provided and WS already cleared activeTaskId", async () => {
    const store = makeTaskRemovalStore({
      activeTaskId: null,
      activeSessionId: null,
      remainingTasks: [nextTask],
    });
    const { result } = renderHook(() =>
      useTaskRemoval({ store: store as unknown as StoreApi<never> }),
    );

    await result.current.removeTaskFromBoard(TASK_A_ID);

    expect(store.getRecorded().setActiveSession).not.toHaveBeenCalled();
    expect(store.getRecorded().setActiveTask).not.toHaveBeenCalled();
    expect(softNavigateMock).not.toHaveBeenCalled();
  });

  it("does NOT switch when activeTaskId is null AND wasActiveTaskId does not match removed task", async () => {
    const store = makeTaskRemovalStore({
      activeTaskId: null,
      activeSessionId: null,
      remainingTasks: [nextTask],
    });
    const { result } = renderHook(() =>
      useTaskRemoval({ store: store as unknown as StoreApi<never> }),
    );

    await result.current.removeTaskFromBoard(TASK_A_ID, {
      wasActiveTaskId: "task-B",
      wasActiveSessionId: "sess-B",
    });

    expect(store.getRecorded().setActiveSession).not.toHaveBeenCalled();
    expect(store.getRecorded().setActiveTask).not.toHaveBeenCalled();
    expect(softNavigateMock).not.toHaveBeenCalled();
  });

  it("redirects to the explicit overview when no remaining tasks AND user is still on removed task", async () => {
    const store = makeTaskRemovalStore({
      activeTaskId: TASK_A_ID,
      activeSessionId: "sess-A",
      remainingTasks: [{ id: TASK_A_ID, primarySessionId: "sess-A" }],
    });
    const { result } = renderHook(() =>
      useTaskRemoval({ store: store as unknown as StoreApi<never> }),
    );
    const removeResult = await result.current.removeTaskFromBoard(TASK_A_ID, {
      wasActiveTaskId: TASK_A_ID,
      wasActiveSessionId: "sess-A",
    });
    expect(removeResult.switchedTaskId).toBeNull();
    expect(softNavigateMock).toHaveBeenCalledWith(OVERVIEW_URL, "replace");
  });
});

describe("useTaskRemoval — bounded fallback query", () => {
  it("fetches one bounded active page when no cached fallback task is available", async () => {
    const store = makeTaskRemovalStore({
      activeTaskId: TASK_A_ID,
      activeSessionId: "sess-A",
      remainingTasks: [{ id: TASK_A_ID, primarySessionId: "sess-A", workspaceId: WORKSPACE_ID }],
    });
    const serverTask = makeFallbackTask("Off-page fallback");
    querySidebarTasksMock.mockResolvedValue({
      entries: [
        { kind: "group", group_key: "__all__" },
        { kind: "task", task_id: serverTask.id, task: serverTask },
      ],
    } as SidebarTaskPageResponse);
    const { result } = renderHook(() =>
      useTaskRemoval({ store: store as unknown as StoreApi<never> }),
    );

    const removal = await result.current.removeTaskFromBoard(TASK_A_ID, {
      wasActiveTaskId: TASK_A_ID,
      wasActiveSessionId: "sess-A",
      workspaceId: WORKSPACE_ID,
    });

    expect(removal.switchedTaskId).toBe(OFF_PAGE_TASK_ID);
    expect(querySidebarTasksMock).toHaveBeenCalledWith(
      WORKSPACE_ID,
      expect.objectContaining({
        filters: [{ dimension: "archived", op: "is", value: false }],
        page: 1,
        page_size: 100,
      }),
      expect.objectContaining({ cache: "no-store" }),
    );
    expect(store.getRecorded().setActiveSession).toHaveBeenCalledWith(
      OFF_PAGE_TASK_ID,
      OFF_PAGE_SESSION_ID,
    );
    expect(softNavigateMock).toHaveBeenCalledWith(`/t/${OFF_PAGE_TASK_ID}`, "replace");
  });
});

describe("useTaskRemoval — recent off-page fallback", () => {
  it("keeps a recent off-page task ahead of a cached fallback candidate", async () => {
    const oldTask = { id: "task-old", primarySessionId: "sess-old", workspaceId: WORKSPACE_ID };
    const store = makeTaskRemovalStore({
      activeTaskId: TASK_A_ID,
      activeSessionId: "sess-A",
      remainingTasks: [
        { id: TASK_A_ID, primarySessionId: "sess-A", workspaceId: WORKSPACE_ID },
        oldTask,
      ],
    });
    store.getRecorded().environmentIdBySessionId["sess-old"] = "env-old";
    const serverTask = makeFallbackTask("Recent off-page fallback");
    querySidebarTasksMock.mockResolvedValue({
      query_key: "fallback",
      page: 1,
      page_size: 100,
      total_entries: 1,
      total_tasks: 1,
      total_visible_tasks: 1,
      has_previous: false,
      has_next: false,
      entries: [{ kind: "task", task_id: serverTask.id, task: serverTask }],
    } as SidebarTaskPageResponse);
    setRecentTasks([
      { taskId: OFF_PAGE_TASK_ID, title: "Recent", visitedAt: RECENT_TASK_VISITED_AT },
      { taskId: TASK_A_ID, title: "Removed", visitedAt: REMOVED_TASK_VISITED_AT },
    ]);
    const { result } = renderHook(() =>
      useTaskRemoval({ store: store as unknown as StoreApi<never> }),
    );

    const removal = await result.current.removeTaskFromBoard(TASK_A_ID, {
      wasActiveTaskId: TASK_A_ID,
      wasActiveSessionId: "sess-A",
      workspaceId: WORKSPACE_ID,
    });

    expect(removal.switchedTaskId).toBe(OFF_PAGE_TASK_ID);
    expect(store.getRecorded().setActiveSession).toHaveBeenCalledWith(
      OFF_PAGE_TASK_ID,
      OFF_PAGE_SESSION_ID,
    );
  });
});

describe("useTaskRemoval — canonical fallback candidates", () => {
  it("redirects home when a stale snapshot candidate is absent from the canonical board", async () => {
    const staleTask = { id: "task-stale", primarySessionId: "sess-stale" };
    const store = makeTaskRemovalStore({
      activeTaskId: TASK_A_ID,
      activeSessionId: "sess-A",
      remainingTasks: [{ id: TASK_A_ID, primarySessionId: "sess-A" }, staleTask],
      canonicalTasks: [],
    });
    fetchTaskMock.mockRejectedValueOnce(new Error("task not found"));
    const { result } = renderHook(() =>
      useTaskRemoval({ store: store as unknown as StoreApi<never> }),
    );

    const removal = await result.current.removeTaskFromBoard(TASK_A_ID, {
      wasActiveTaskId: TASK_A_ID,
      wasActiveSessionId: "sess-A",
    });

    expect(removal.switchedTaskId).toBeNull();
    expect(store.getRecorded().setActiveSession).not.toHaveBeenCalled();
    expect(softNavigateMock).toHaveBeenCalledWith(OVERVIEW_URL, "replace");
  });
});

describe("useTaskRemoval — workspace-scoped fallback candidates", () => {
  it("does not use a candidate without workspace ownership for a scoped removal", async () => {
    const candidateWithoutWorkspace = { id: "task-unscoped", primarySessionId: "sess-unscoped" };
    const store = makeTaskRemovalStore({
      activeTaskId: TASK_A_ID,
      activeSessionId: "sess-A",
      remainingTasks: [
        { id: TASK_A_ID, primarySessionId: "sess-A", workspaceId: WORKSPACE_ID },
        candidateWithoutWorkspace,
      ],
    });
    const { result } = renderHook(() =>
      useTaskRemoval({ store: store as unknown as StoreApi<never> }),
    );

    const removal = await result.current.removeTaskFromBoard(TASK_A_ID, {
      wasActiveTaskId: TASK_A_ID,
      wasActiveSessionId: "sess-A",
      workspaceId: WORKSPACE_ID,
    });

    expect(removal.switchedTaskId).toBeNull();
    expect(store.getRecorded().setActiveSession).not.toHaveBeenCalled();
    expect(softNavigateMock).toHaveBeenCalledWith(OVERVIEW_URL, "replace");
  });
});

describe("useTaskRemoval — recent task ordering", () => {
  it("switches to the most recent remaining task instead of the first snapshot task", async () => {
    const store = makeStoreWithOldAndRecentTasks(TASK_A_ID);
    setRecentTaskFirst();

    const { result } = renderHook(() =>
      useTaskRemoval({ store: store as unknown as StoreApi<never> }),
    );

    await result.current.removeTaskFromBoard(TASK_A_ID, {
      wasActiveTaskId: TASK_A_ID,
      wasActiveSessionId: "sess-A",
    });

    expect(store.getRecorded().setActiveSession).toHaveBeenCalledWith(
      recentTaskId,
      recentSessionId,
    );
    expect(softNavigateMock).toHaveBeenCalledWith(`/t/${recentTaskId}`, "replace");
  });

  it("skips the removed active task when it is first in recent history", async () => {
    const store = makeStoreWithOldAndRecentTasks(TASK_A_ID);
    setRemovedTaskFirst();

    const { result } = renderHook(() =>
      useTaskRemoval({ store: store as unknown as StoreApi<never> }),
    );

    await result.current.removeTaskFromBoard(TASK_A_ID, {
      wasActiveTaskId: TASK_A_ID,
      wasActiveSessionId: "sess-A",
    });

    expect(store.getRecorded().setActiveSession).toHaveBeenCalledWith(
      recentTaskId,
      recentSessionId,
    );
    expect(softNavigateMock).toHaveBeenCalledWith(`/t/${recentTaskId}`, "replace");
  });

  it("can switch before removing the task from board state", async () => {
    const store = makeStoreWithOldAndRecentTasks(TASK_A_ID);
    setRemovedTaskFirst();

    const { result } = renderHook(() =>
      useTaskRemoval({ store: store as unknown as StoreApi<never> }),
    );

    await result.current.removeTaskFromBoard(TASK_A_ID, {
      wasActiveTaskId: TASK_A_ID,
      wasActiveSessionId: "sess-A",
      switchOnly: true,
    });

    expect(store.getRecorded().setWorkflowSnapshot).not.toHaveBeenCalled();
    expect(store.getRecorded().setActiveSession).toHaveBeenCalledWith(
      recentTaskId,
      recentSessionId,
    );
    expect(softNavigateMock).toHaveBeenCalledWith(`/t/${recentTaskId}`, "replace");
  });
});

describe("useTaskRemoval — cascade tree exclusion", () => {
  it("skips a cascade tree collected from workflow and canonical projections", async () => {
    const unrelated: TaskRow = { id: "task-unrelated", primarySessionId: "sess-unrelated" };
    const store = makeTaskRemovalStore({
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

  it("prefers the workflow snapshot when canonical ancestry is stale", async () => {
    const detachedChild = { ...CASCADE_CHILD, parentTaskId: null };
    const unrelated: TaskRow = { id: "task-unrelated", primarySessionId: "sess-unrelated" };
    const store = makeTaskRemovalStore({
      activeTaskId: CASCADE_PARENT.id,
      activeSessionId: CASCADE_PARENT.primarySessionId,
      remainingTasks: [CASCADE_PARENT, detachedChild, unrelated],
      canonicalTasks: [CASCADE_CHILD],
    });
    setRecentTasks([
      { taskId: detachedChild.id, title: "Detached child", visitedAt: CASCADE_CHILD_VISITED_AT },
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
      detachedChild.id,
      detachedChild.primarySessionId,
    );
  });

  it("can still switch to an active child when cascade exclusion is not requested", async () => {
    const store = makeTaskRemovalStore({
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
    const store = makeTaskRemovalStore({
      activeTaskId: CASCADE_PARENT.id,
      activeSessionId: CASCADE_PARENT.primarySessionId,
      remainingTasks: [CASCADE_PARENT, CASCADE_CHILD],
    });
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
    expect(softNavigateMock).not.toHaveBeenCalled();
  });

  it("opens the overview after final cascade cleanup when no safe task remains", async () => {
    const store = makeTaskRemovalStore({
      activeTaskId: CASCADE_PARENT.id,
      activeSessionId: CASCADE_PARENT.primarySessionId,
      remainingTasks: [CASCADE_PARENT, CASCADE_CHILD],
    });
    const { result } = renderHook(() =>
      useTaskRemoval({ store: store as unknown as StoreApi<never> }),
    );

    const removal = await result.current.removeTaskFromBoard(CASCADE_PARENT.id, {
      wasActiveTaskId: CASCADE_PARENT.id,
      wasActiveSessionId: CASCADE_PARENT.primarySessionId,
      excludeTaskTree: true,
    } as never);

    expect(removal.switchedTaskId).toBeNull();
    expect(softNavigateMock).toHaveBeenCalledWith(OVERVIEW_URL, "replace");
  });

  it("retains the original cascade tree for cleanup after cache pruning", async () => {
    const store = makeTaskRemovalStore({
      activeTaskId: CASCADE_PARENT.id,
      activeSessionId: CASCADE_PARENT.primarySessionId,
      remainingTasks: [CASCADE_PARENT, CASCADE_CHILD],
    });
    const { result } = renderHook(() =>
      useTaskRemoval({ store: store as unknown as StoreApi<never> }),
    );
    const initialRemoval = await result.current.removeTaskFromBoard(CASCADE_PARENT.id, {
      wasActiveTaskId: CASCADE_PARENT.id,
      wasActiveSessionId: CASCADE_PARENT.primarySessionId,
      switchOnly: true,
      excludeTaskTree: true,
    } as never);

    expect(initialRemoval.excludedTaskIds).toEqual(new Set([CASCADE_PARENT.id, CASCADE_CHILD.id]));
    store.getRecorded().kanbanMulti.snapshots["wf-1"].tasks = [
      { ...CASCADE_CHILD, parentTaskId: null },
    ];
    store.getRecorded().kanban.tasks = [];

    await result.current.removeTaskFromBoard(CASCADE_PARENT.id, {
      wasActiveTaskId: CASCADE_PARENT.id,
      wasActiveSessionId: CASCADE_PARENT.primarySessionId,
      excludeTaskTree: true,
      excludedTaskIds: initialRemoval.excludedTaskIds,
    } as never);

    expect(store.getRecorded().kanbanMulti.snapshots["wf-1"].tasks).toEqual([]);
    expect(softNavigateMock).toHaveBeenCalledWith(OVERVIEW_URL, "replace");
  });
});

describe("selectNextTaskAfterRemoval — stale-candidate rejection", () => {
  const select = selectNextTaskAfterRemoval as unknown as (
    remaining: TaskRow[],
    removedTaskId: string,
    isLive: (taskId: string) => Promise<boolean>,
    options?: { excludedTaskIds?: ReadonlySet<string> },
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
      select([child, deepChild, unrelated], CASCADE_PARENT.id, async () => true, {
        excludedTaskIds: new Set([CASCADE_PARENT.id, child.id, deepChild.id]),
      }),
    ).resolves.toBe(unrelated);
  });

  it("returns null (redirect home) when the only remaining candidate is not a live board member", async () => {
    const stale: TaskRow = { id: "task-A-stale", primarySessionId: "sess-A" };

    await expect(select([stale], "task-B", async () => false)).resolves.toBeNull();
  });

  it("returns the candidate when it is a live board member", async () => {
    const live: TaskRow = { id: "task-A-live", primarySessionId: "sess-A" };

    await expect(select([live], "task-B", async () => true)).resolves.toBe(live);
  });
});
