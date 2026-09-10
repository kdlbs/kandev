import { act, renderHook } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { createAppStore } from "@/lib/state/store";
import { taskRemovalCoversTask } from "@/lib/state/task-removal";

const fetchTaskMock = vi.fn();
const listTaskSessionsMock = vi.fn();
const softNavigateMock = vi.fn();
const performLayoutSwitchMock = vi.fn();

vi.mock("@/lib/api", () => ({
  fetchTask: (...args: unknown[]) => fetchTaskMock(...args),
  listTaskSessions: (...args: unknown[]) => listTaskSessionsMock(...args),
}));

vi.mock("@/lib/links", () => ({
  linkToTask: (taskId: string) => `/t/${taskId}`,
  linkToTaskOverview: () => "/?home=overview",
}));

vi.mock("@/lib/routing/client-router", () => ({
  softNavigate: (...args: unknown[]) => softNavigateMock(...args),
}));

vi.mock("@/lib/state/dockview-store", () => ({
  performLayoutSwitch: (...args: unknown[]) => performLayoutSwitchMock(...args),
}));

import { useTaskRemoval } from "./use-task-removal";

const TASK_A = "task-a";
const TASK_B = "task-b";

function deferred<T>() {
  let resolve!: (value: T | PromiseLike<T>) => void;
  let reject!: (reason?: unknown) => void;
  const promise = new Promise<T>((res, rej) => {
    resolve = res;
    reject = rej;
  });
  return { promise, resolve, reject };
}

function makeTask(id: string, sessionId: string) {
  return {
    id,
    workspaceId: "workspace-1",
    workflowId: "workflow-1",
    workflowStepId: "step-1",
    title: id,
    position: id === TASK_A ? 0 : 1,
    primarySessionId: sessionId,
  };
}

function makeStore() {
  const store = createAppStore();
  store.setState((state) => ({
    ...state,
    kanban: {
      ...state.kanban,
      tasks: [makeTask(TASK_A, "session-a"), makeTask(TASK_B, "session-b")],
    },
    kanbanMulti: {
      ...state.kanbanMulti,
      snapshots: {
        "workflow-1": {
          workflowId: "workflow-1",
          workflowName: "Workflow",
          steps: [],
          tasks: [makeTask(TASK_A, "session-a"), makeTask(TASK_B, "session-b")],
        },
      },
    },
    workspaces: { ...state.workspaces, activeId: "workspace-1" },
  }));
  store.getState().setActiveSession(TASK_A, "session-a");
  return store;
}

beforeEach(() => {
  vi.clearAllMocks();
  fetchTaskMock.mockImplementation(async (taskId: string) => ({
    id: taskId,
    archived_at: null,
  }));
  listTaskSessionsMock.mockResolvedValue({ sessions: [] });
});

describe("useTaskRemoval coordinator", () => {
  it("publishes departure before mutation and releases after destination commit", async () => {
    const store = makeStore();
    const mutation = deferred<void>();
    const { result } = renderHook(() => useTaskRemoval({ store }));

    let removal!: Promise<unknown>;
    act(() => {
      removal = result.current.runTaskRemoval("delete", {
        taskId: TASK_A,
        mutate: () => mutation.promise,
      });
    });

    expect(taskRemovalCoversTask(store.getState().taskRemoval, TASK_A)).toBe(true);
    expect(store.getState().tasks.activeTaskId).toBe(TASK_A);

    mutation.resolve();
    await removal;

    expect(store.getState().tasks.activeTaskId).toBe(TASK_B);
    expect(softNavigateMock).toHaveBeenCalledWith(`/t/${TASK_B}`, "replace");
    expect(taskRemovalCoversTask(store.getState().taskRemoval, TASK_A)).toBe(false);
    expect(store.getState().kanban.tasks.some((task) => task.id === TASK_A)).toBe(false);
  });

  it("does not issue a duplicate mutation for an overlapping pending target", async () => {
    const store = makeStore();
    const firstMutation = deferred<void>();
    const mutate = vi.fn(() => firstMutation.promise);
    const { result } = renderHook(() => useTaskRemoval({ store }));

    let first!: Promise<unknown>;
    act(() => {
      first = result.current.runTaskRemoval("delete", { taskId: TASK_A, mutate });
    });
    const second = await result.current.runTaskRemoval("delete", {
      taskId: TASK_A,
      mutate,
    });

    expect(second.skipped).toBe(true);
    expect(mutate).toHaveBeenCalledOnce();
    firstMutation.resolve();
    await first;
  });

  it("notifies once after a fully successful batch", async () => {
    const store = makeStore();
    const notifySuccess = vi.fn();
    const { result } = renderHook(() => useTaskRemoval({ store, notifySuccess }));

    await result.current.runTaskRemovalBatch("archive", [
      { taskId: TASK_A, mutate: async () => undefined },
      { taskId: TASK_B, mutate: async () => undefined },
    ]);

    expect(notifySuccess).toHaveBeenCalledOnce();
    expect(notifySuccess).toHaveBeenCalledWith("archive", 2);
  });

  it("does not commit an automatic fallback after leave-and-return navigation", async () => {
    const store = makeStore();
    const candidate = deferred<{ id: string; archived_at: null }>();
    const mutation = deferred<void>();
    fetchTaskMock.mockReturnValueOnce(candidate.promise);
    const { result } = renderHook(() => useTaskRemoval({ store }));

    let removal!: Promise<unknown>;
    act(() => {
      removal = result.current.runTaskRemoval("delete", {
        taskId: TASK_A,
        mutate: () => mutation.promise,
      });
    });

    store.getState().setActiveTask("task-other");
    store.getState().setActiveTask(TASK_A);
    candidate.resolve({ id: TASK_B, archived_at: null });
    mutation.resolve();
    await removal;

    expect(store.getState().tasks.activeTaskId).toBe(TASK_A);
    expect(softNavigateMock).not.toHaveBeenCalledWith(`/t/${TASK_B}`, "replace");
  });
});
