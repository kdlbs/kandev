import { describe, it, expect, vi, beforeEach } from "vitest";
import { renderHook } from "@testing-library/react";
import { createElement, type ReactNode } from "react";
import { QueryClientProvider } from "@tanstack/react-query";
import type { StoreApi } from "zustand";
import { makeQueryClient } from "@/lib/query/client";
import { qk } from "@/lib/query/keys";
import { LOCATION_CHANGE_EVENT } from "@/lib/routing/navigation-event";
import { sessionId, taskId, workspaceId, workflowId } from "@/lib/types/ids";
import type { Task, TaskSession, WorkflowSnapshot } from "@/lib/types/http";

const replaceTaskUrlMock = vi.fn();
const performLayoutSwitchMock = vi.fn();
const listTaskSessionsMock = vi.fn();

vi.mock("@/lib/links", () => ({
  replaceTaskUrl: (...args: unknown[]) => replaceTaskUrlMock(...args),
}));

vi.mock("@/lib/state/dockview-store", () => ({
  performLayoutSwitch: (...args: unknown[]) => performLayoutSwitchMock(...args),
}));

vi.mock("@/lib/api", () => ({
  listTaskSessions: (...args: unknown[]) => listTaskSessionsMock(...args),
}));

vi.mock("@/lib/api/domains/session-api", () => ({
  fetchTaskSession: vi.fn(),
  listSessionTurns: vi.fn(),
  listTaskSessionMessages: vi.fn(),
  listTaskSessions: (...args: unknown[]) => listTaskSessionsMock(...args),
  searchSessionMessages: vi.fn(),
}));

import { useTaskRemoval } from "./use-task-removal";
import { setRecentTasks } from "@/lib/recent-tasks";

type TaskRow = { id: string; primarySessionId: string | null };

type FakeState = {
  tasks: { activeTaskId: string | null; activeSessionId: string | null };
  workflowSnapshots: Record<string, { tasks: TaskRow[] }>;
  environmentIdBySessionId: Record<string, string>;
  taskSessionsByTask: {
    itemsByTaskId: Record<string, TaskSession[]>;
    loadedByTaskId: Record<string, boolean>;
    loadingByTaskId: Record<string, boolean>;
  };
  setActiveTask: ReturnType<typeof vi.fn>;
  setActiveSession: ReturnType<typeof vi.fn>;
  setTaskSessionsForTask: ReturnType<typeof vi.fn>;
  setTaskSessionsLoading: ReturnType<typeof vi.fn>;
};

function makeStore(init: {
  activeTaskId: string | null;
  activeSessionId?: string | null;
  remainingTasks: TaskRow[];
}): StoreApi<FakeState> & { getRecorded: () => FakeState } {
  const state: FakeState = {
    tasks: {
      activeTaskId: init.activeTaskId,
      activeSessionId: init.activeSessionId ?? null,
    },
    workflowSnapshots: { "wf-1": { tasks: init.remainingTasks } },
    environmentIdBySessionId: { "sess-next": "env-next", "sess-A": "env-A" },
    taskSessionsByTask: {
      itemsByTaskId: {},
      loadedByTaskId: {},
      loadingByTaskId: {},
    },
    setActiveTask: vi.fn() as ReturnType<typeof vi.fn>,
    setActiveSession: vi.fn() as ReturnType<typeof vi.fn>,
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

const nextTask: TaskRow = { id: "task-next", primarySessionId: "sess-next" };
const recentTaskId = "task-recent";
const recentSessionId = "sess-recent";
const sessionTimestamp = "2026-06-24T00:00:00Z";

function makeSession(
  id: string,
  taskIdValue: string,
  overrides: Partial<TaskSession> = {},
): TaskSession {
  return {
    id: sessionId(id),
    task_id: taskId(taskIdValue),
    state: "CREATED",
    started_at: sessionTimestamp,
    updated_at: sessionTimestamp,
    ...overrides,
  } as TaskSession;
}

function renderTaskRemoval(store: StoreApi<FakeState>) {
  const client = makeQueryClient();
  const fakeState = (
    store as StoreApi<FakeState> & { getRecorded?: () => FakeState }
  ).getRecorded?.();
  for (const [workflowId, snapshot] of Object.entries(fakeState?.workflowSnapshots ?? {})) {
    client.setQueryData(
      qk.workflows.snapshot(workflowId),
      makeWorkflowSnapshot(
        snapshot.tasks.map((task) => makeTask(task.id, task.primarySessionId)),
        workflowId,
      ),
    );
  }
  const wrapper = ({ children }: { children: ReactNode }) =>
    createElement(QueryClientProvider, { client }, children);
  const hook = renderHook(() => useTaskRemoval({ store: store as unknown as StoreApi<never> }), {
    wrapper,
  });
  return { ...hook, queryClient: client };
}

function makeTask(id: string, primarySessionId: string | null = null): Task {
  return {
    id: taskId(id),
    workspace_id: workspaceId("ws-1"),
    workflow_id: workflowId("wf-1"),
    workflow_step_id: "step-1",
    position: 0,
    title: id,
    description: "",
    state: "TODO",
    priority: 0,
    repositories: [],
    primary_session_id: primarySessionId ? sessionId(primarySessionId) : null,
    created_at: "2026-06-24T00:00:00Z",
    updated_at: "2026-06-24T00:00:00Z",
  } as Task;
}

function makeWorkflowSnapshot(tasks: Task[], workflowIdValue = "wf-1"): WorkflowSnapshot {
  return {
    workflow: {
      id: workflowId(workflowIdValue),
      workspace_id: workspaceId("ws-1"),
      name: "Workflow",
      sort_order: 0,
      hidden: false,
    },
    steps: [
      {
        id: "step-1",
        workflow_id: workflowId(workflowIdValue),
        name: "Todo",
        position: 0,
        color: "bg-blue-500",
      },
    ],
    tasks,
  } as WorkflowSnapshot;
}

function queryTaskIds(snapshot: WorkflowSnapshot | undefined): string[] {
  return snapshot?.tasks.map((task) => task.id) ?? [];
}

beforeEach(() => {
  vi.clearAllMocks();
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
      visitedAt: "2026-06-07T10:00:00Z",
    },
    {
      taskId: "task-A",
      title: "Removed task",
      visitedAt: "2026-06-07T09:00:00Z",
    },
    {
      taskId: "task-old",
      title: "Old task",
      visitedAt: "2026-06-06T10:00:00Z",
    },
  ]);
}

function setRemovedTaskFirst() {
  setRecentTasks([
    {
      taskId: "task-A",
      title: "Removed task",
      visitedAt: "2026-06-07T10:00:00Z",
    },
    {
      taskId: recentTaskId,
      title: "Recent task",
      visitedAt: "2026-06-07T09:00:00Z",
    },
    {
      taskId: "task-old",
      title: "Old task",
      visitedAt: "2026-06-06T10:00:00Z",
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
    const { result } = renderTaskRemoval(store);

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
    const { result } = renderTaskRemoval(store);

    await result.current.removeTaskFromBoard("task-A", {
      wasActiveTaskId: "task-A",
      wasActiveSessionId: "sess-A",
    });

    expect(store.getRecorded().setActiveSession).not.toHaveBeenCalled();
    expect(store.getRecorded().setActiveTask).not.toHaveBeenCalled();
    expect(replaceTaskUrlMock).not.toHaveBeenCalled();
  });
});

describe("useTaskRemoval — session hydration", () => {
  it("awaits Query hydration instead of returning partial navigation rows", async () => {
    const store = makeStore({
      activeTaskId: "task-A",
      activeSessionId: "sess-A",
      remainingTasks: [{ id: "task-A", primarySessionId: "sess-A" }],
    });
    const partialSession = makeSession("sess-next", "task-next", {
      task_environment_id: "env-next",
    });
    const hydratedSession = makeSession("sess-next", "task-next", {
      task_environment_id: "env-next",
      is_passthrough: true,
      agent_profile_snapshot: { cli_passthrough: true },
    });
    store.getRecorded().taskSessionsByTask.itemsByTaskId["task-next"] = [partialSession];
    store.getRecorded().taskSessionsByTask.loadedByTaskId["task-next"] = true;
    listTaskSessionsMock.mockResolvedValueOnce({ sessions: [hydratedSession], total: 1 });

    const { result, queryClient } = renderTaskRemoval(store);
    queryClient.setQueryData(qk.taskSession.byTask("task-next"), {
      sessions: [partialSession],
      total: 1,
    });
    const sessions = await result.current.loadTaskSessionsForTask("task-next");

    expect(listTaskSessionsMock).toHaveBeenCalledWith("task-next", expect.any(Object));
    expect(store.getRecorded().setTaskSessionsForTask).toHaveBeenCalledWith("task-next", [
      hydratedSession,
    ]);
    expect(sessions).toEqual([hydratedSession]);
  });
});

describe("useTaskRemoval — switch guard (WS-clear fallback)", () => {
  it("switches when WS cleared activeTaskId AND wasActiveTaskId matches removed task", async () => {
    const store = makeStore({
      activeTaskId: null,
      activeSessionId: null,
      remainingTasks: [nextTask],
    });
    const { result } = renderTaskRemoval(store);

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
    const { result } = renderTaskRemoval(store);

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
    const { result } = renderTaskRemoval(store);

    await result.current.removeTaskFromBoard("task-A", {
      wasActiveTaskId: "task-B",
      wasActiveSessionId: "sess-B",
    });

    expect(store.getRecorded().setActiveSession).not.toHaveBeenCalled();
    expect(store.getRecorded().setActiveTask).not.toHaveBeenCalled();
    expect(replaceTaskUrlMock).not.toHaveBeenCalled();
  });

  it("replaces the SPA route with / when no remaining tasks AND user still on removed task", async () => {
    const store = makeStore({
      activeTaskId: "task-A",
      activeSessionId: "sess-A",
      remainingTasks: [{ id: "task-A", primarySessionId: "sess-A" }],
    });
    const locationChanged = vi.fn();
    window.history.replaceState({}, "", "/t/task-A");
    window.addEventListener(LOCATION_CHANGE_EVENT, locationChanged);

    const { result } = renderTaskRemoval(store);
    const removeResult = await result.current.removeTaskFromBoard("task-A", {
      wasActiveTaskId: "task-A",
      wasActiveSessionId: "sess-A",
    });

    window.removeEventListener(LOCATION_CHANGE_EVENT, locationChanged);
    expect(removeResult.switchedTaskId).toBeNull();
    expect(window.location.pathname).toBe("/");
    expect(locationChanged).toHaveBeenCalledOnce();
  });
});

describe("useTaskRemoval — next task selection", () => {
  it("switches to the most recent remaining task instead of the first snapshot task", async () => {
    const store = makeStoreWithOldAndRecentTasks("task-A");
    setRecentTaskFirst();

    const { result } = renderTaskRemoval(store);

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

    const { result } = renderTaskRemoval(store);

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

    const { result } = renderTaskRemoval(store);

    await result.current.removeTaskFromBoard("task-A", {
      wasActiveTaskId: "task-A",
      wasActiveSessionId: "sess-A",
      switchOnly: true,
    });

    expect(store.getRecorded().setActiveSession).toHaveBeenCalledWith(
      recentTaskId,
      recentSessionId,
    );
    expect(replaceTaskUrlMock).toHaveBeenCalledWith(recentTaskId);
  });
});

describe("useTaskRemoval — workflow snapshot Query cache", () => {
  it("does not expose a legacy active kanban task mirror", async () => {
    const store = makeStore({
      activeTaskId: "task-other",
      activeSessionId: "sess-A",
      remainingTasks: [],
    });
    const { result, queryClient } = renderTaskRemoval(store);
    queryClient.setQueryData(
      qk.workflows.snapshot("wf-1"),
      makeWorkflowSnapshot([makeTask("task-A", "sess-A"), makeTask("task-next", "sess-next")]),
    );

    await result.current.removeTaskFromBoard("task-A", {
      wasActiveTaskId: "task-A",
      wasActiveSessionId: "sess-A",
    });

    expect("kanban" in store.getRecorded()).toBe(false);
  });

  it("does not select the next task from a removed active kanban fallback", async () => {
    const store = makeStore({
      activeTaskId: "task-A",
      activeSessionId: "sess-A",
      remainingTasks: [],
    });
    window.history.replaceState({}, "", "/t/task-A");
    const { result } = renderTaskRemoval(store);

    await result.current.removeTaskFromBoard("task-A", {
      wasActiveTaskId: "task-A",
      wasActiveSessionId: "sess-A",
    });

    expect(store.getRecorded().setActiveSession).not.toHaveBeenCalled();
    expect(store.getRecorded().setActiveTask).not.toHaveBeenCalled();
    expect(window.location.pathname).toBe("/");
  });

  it("removes the task from cached workflow snapshots without a store mirror", async () => {
    const store = makeStore({
      activeTaskId: "task-other",
      activeSessionId: "sess-A",
      remainingTasks: [],
    });
    const { result, queryClient } = renderTaskRemoval(store);
    queryClient.setQueryData(
      qk.workflows.snapshot("wf-1"),
      makeWorkflowSnapshot([makeTask("task-A", "sess-A"), makeTask("task-next", "sess-next")]),
    );

    await result.current.removeTaskFromBoard("task-A", {
      wasActiveTaskId: "task-A",
      wasActiveSessionId: "sess-A",
    });

    expect(
      queryTaskIds(queryClient.getQueryData<WorkflowSnapshot>(qk.workflows.snapshot("wf-1"))),
    ).toEqual(["task-next"]);
    expect("setWorkflowSnapshot" in store.getRecorded()).toBe(false);
  });
});
