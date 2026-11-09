import { act, renderHook } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { createAppStore } from "@/lib/state/store";
import { isTaskRemovalPending, taskRemovalCoversTask } from "@/lib/state/task-removal";

const fetchTaskMock = vi.fn();
const listTaskSessionsMock = vi.fn();
const softNavigateMock = vi.fn();
const performLayoutSwitchMock = vi.fn();
const querySidebarTasksMock = vi.fn();

vi.mock("@/lib/api", () => ({
  fetchTask: (...args: unknown[]) => fetchTaskMock(...args),
  listTaskSessions: (...args: unknown[]) => listTaskSessionsMock(...args),
  querySidebarTasks: (...args: unknown[]) => querySidebarTasksMock(...args),
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
const TASK_C = "task-c";
const WORKSPACE = "workspace-1";
const OVERVIEW_URL = "/?home=overview";

function deferred<T>() {
  let resolve!: (value: T | PromiseLike<T>) => void;
  let reject!: (reason?: unknown) => void;
  const promise = new Promise<T>((res, rej) => {
    resolve = res;
    reject = rej;
  });
  return { promise, resolve, reject };
}

function makeTask(id: string, sessionId: string | null) {
  return {
    id,
    workspaceId: WORKSPACE,
    workflowId: "workflow-1",
    workflowStepId: "step-1",
    title: id,
    position: [TASK_A, TASK_B, TASK_C].indexOf(id),
    primarySessionId: sessionId,
  };
}

function setStoreTasks(
  store: ReturnType<typeof createAppStore>,
  tasks: ReturnType<typeof makeTask>[],
) {
  store.setState((state) => ({
    ...state,
    kanban: { ...state.kanban, tasks },
    kanbanMulti: {
      ...state.kanbanMulti,
      snapshots: {
        "workflow-1": {
          workflowId: "workflow-1",
          workflowName: "Workflow",
          steps: [],
          tasks,
        },
      },
    },
  }));
}

function makeStore(
  tasks: ReturnType<typeof makeTask>[] = [
    makeTask(TASK_A, "session-a"),
    makeTask(TASK_B, "session-b"),
  ],
) {
  const store = createAppStore();
  // The workspace scope must settle before board writes: a later activeId
  // change isolates unchanged projections and drops their rows.
  store.setState((state) => ({
    ...state,
    workspaces: { ...state.workspaces, activeId: WORKSPACE },
  }));
  setStoreTasks(store, tasks);
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
  querySidebarTasksMock.mockResolvedValue({ entries: [], has_next: false });
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

    await act(async () => {
      mutation.resolve();
      await removal;
    });

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
    await act(async () => {
      firstMutation.resolve();
      await first;
    });
  });

  it("notifies once after a fully successful batch", async () => {
    const store = makeStore();
    const notifySuccess = vi.fn();
    const { result } = renderHook(() => useTaskRemoval({ store, notifySuccess }));

    await act(async () => {
      await result.current.runTaskRemovalBatch("archive", [
        { taskId: TASK_A, mutate: async () => undefined },
        { taskId: TASK_B, mutate: async () => undefined },
      ]);
    });

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

    await act(async () => {
      store.getState().setActiveTask("task-other");
      store.getState().setActiveTask(TASK_A);
      candidate.resolve({ id: TASK_B, archived_at: null });
      mutation.resolve();
      await removal;
    });

    expect(store.getState().tasks.activeTaskId).toBe(TASK_A);
    expect(softNavigateMock).not.toHaveBeenCalledWith(`/t/${TASK_B}`, "replace");
  });
});

describe("useTaskRemoval coordinator — preview removal", () => {
  it("removes a previewed task without switching away from the board", async () => {
    const store = makeStore();
    store.setState((state) => ({ ...state, kanbanPreviewedTaskId: TASK_A }));
    const { result } = renderHook(() => useTaskRemoval({ store }));

    await act(async () => {
      await result.current.runTaskRemoval("delete", {
        taskId: TASK_A,
        mutate: async () => undefined,
      });
    });

    expect(store.getState().tasks.activeTaskId).toBe(TASK_A);
    expect(store.getState().kanban.tasks.some((task) => task.id === TASK_A)).toBe(false);
    expect(softNavigateMock).not.toHaveBeenCalledWith(`/t/${TASK_B}`, "replace");
  });

  it("restores a failed preview removal without navigating to a task route", async () => {
    const store = makeStore();
    store.setState((state) => ({ ...state, kanbanPreviewedTaskId: TASK_A }));
    const error = new Error("delete failed");
    const { result } = renderHook(() => useTaskRemoval({ store }));

    await act(async () => {
      await expect(
        result.current.runTaskRemoval("delete", {
          taskId: TASK_A,
          mutate: async () => {
            throw error;
          },
        }),
      ).rejects.toThrow("delete failed");
    });

    expect(store.getState().tasks.activeTaskId).toBe(TASK_A);
    expect(softNavigateMock).not.toHaveBeenCalledWith(`/t/${TASK_A}`, "replace");
    expect(softNavigateMock).not.toHaveBeenCalledWith(`/t/${TASK_B}`, "replace");
  });
});

describe("useTaskRemoval coordinator — cascade requests", () => {
  it("mutates a cascaded parent once when the batch also names a descendant", async () => {
    const store = makeStore();
    const child = { ...makeTask(TASK_B, "session-b"), parentTaskId: TASK_A };
    store.setState((state) => ({
      ...state,
      kanban: { ...state.kanban, tasks: [makeTask(TASK_A, "session-a"), child] },
      kanbanMulti: {
        ...state.kanbanMulti,
        snapshots: {
          "workflow-1": {
            ...state.kanbanMulti.snapshots["workflow-1"],
            tasks: [makeTask(TASK_A, "session-a"), child],
          },
        },
      },
    }));
    const parentMutation = vi.fn(async () => undefined);
    const childMutation = vi.fn(async () => undefined);
    const { result } = renderHook(() => useTaskRemoval({ store }));

    let removal!: Awaited<ReturnType<typeof result.current.runTaskRemovalBatch>>;
    await act(async () => {
      removal = await result.current.runTaskRemovalBatch(
        "delete",
        [
          { taskId: TASK_A, mutate: parentMutation },
          { taskId: TASK_B, mutate: childMutation },
        ],
        { cascade: true },
      );
    });

    expect(parentMutation).toHaveBeenCalledOnce();
    expect(childMutation).not.toHaveBeenCalled();
    expect(removal.succeededTaskIds).toEqual([TASK_A, TASK_B]);
    expect(removal.failedTaskIds).toEqual([]);
  });
});

describe("useTaskRemoval coordinator — recovery validation", () => {
  it.each([404, 410, 401, 403])(
    "does not restore from local state after a %s refusal cannot revalidate the task",
    async (status) => {
      const store = makeStore();
      const error = Object.assign(new Error(`request failed: ${status}`), { status });
      fetchTaskMock.mockImplementation(async (taskId: string) => {
        if (taskId === TASK_A) throw error;
        return { id: taskId, archived_at: null };
      });
      const { result } = renderHook(() => useTaskRemoval({ store }));

      await act(async () => {
        await expect(
          result.current.runTaskRemoval("delete", {
            taskId: TASK_A,
            mutate: async () => {
              throw error;
            },
          }),
        ).rejects.toThrow(`request failed: ${status}`);
      });

      expect(store.getState().tasks.activeTaskId).toBe(TASK_B);
      expect(softNavigateMock).toHaveBeenLastCalledWith(OVERVIEW_URL, "replace");
      expect(softNavigateMock).not.toHaveBeenCalledWith(`/t/${TASK_A}`, "replace");
    },
  );
});
function renderCoordinator(store: ReturnType<typeof makeStore>) {
  return renderHook(() => useTaskRemoval({ store })).result;
}

function makeDelayedCandidateRead(taskId: string) {
  const read = deferred<{ id: string; archived_at: null }>();
  fetchTaskMock.mockImplementation(async (candidateId: string) =>
    candidateId === taskId ? read.promise : { id: candidateId, archived_at: null },
  );
  return read;
}

describe("useTaskRemoval coordinator — delayed replacement reads", () => {
  it("does not select another independently pending archive after a delayed candidate read", async () => {
    const store = makeStore();
    const candidateRead = makeDelayedCandidateRead(TASK_B);
    const firstMutation = deferred<void>();
    const secondMutation = deferred<void>();
    const result = renderCoordinator(store);

    let firstArchive!: Promise<unknown>;
    act(() => {
      firstArchive = result.current.runTaskRemoval("archive", {
        taskId: TASK_A,
        mutate: () => firstMutation.promise,
      });
    });
    await vi.waitFor(() =>
      expect(fetchTaskMock).toHaveBeenCalledWith(TASK_B, { cache: "no-store" }),
    );

    let secondArchive!: Promise<unknown>;
    act(() => {
      secondArchive = result.current.runTaskRemoval("archive", {
        taskId: TASK_B,
        mutate: () => secondMutation.promise,
      });
    });
    expect(isTaskRemovalPending(store.getState().taskRemoval, TASK_B, WORKSPACE)).toBe(true);

    await act(async () => {
      candidateRead.resolve({ id: TASK_B, archived_at: null });
      await candidateRead.promise;
      await Promise.resolve();
    });

    expect(store.getState().tasks.activeTaskId).not.toBe(TASK_B);
    expect(softNavigateMock).not.toHaveBeenCalledWith(`/t/${TASK_B}`, "replace");

    await act(async () => {
      firstMutation.resolve();
      await firstArchive;
    });
    expect(store.getState().tasks.activeTaskId).not.toBe(TASK_B);
    expect(softNavigateMock).toHaveBeenLastCalledWith(OVERVIEW_URL, "replace");

    await act(async () => {
      secondMutation.resolve();
      await secondArchive;
    });
    expect(store.getState().tasks.activeTaskId).not.toBe(TASK_B);
    expect(store.getState().kanban.tasks).toEqual([]);
  });

  it("does not select another independently pending archive after a delayed session load", async () => {
    const store = makeStore([makeTask(TASK_A, "session-a"), makeTask(TASK_B, null)]);
    const sessionsRead = deferred<{ sessions: Array<{ id: string; task_id: string }> }>();
    listTaskSessionsMock.mockImplementation((taskId: string) =>
      taskId === TASK_B ? sessionsRead.promise : Promise.resolve({ sessions: [] }),
    );
    const firstMutation = deferred<void>();
    const secondMutation = deferred<void>();
    const result = renderCoordinator(store);

    let firstArchive!: Promise<unknown>;
    act(() => {
      firstArchive = result.current.runTaskRemoval("archive", {
        taskId: TASK_A,
        mutate: () => firstMutation.promise,
      });
    });
    await vi.waitFor(() =>
      expect(listTaskSessionsMock).toHaveBeenCalledWith(TASK_B, { cache: "no-store" }),
    );

    let secondArchive!: Promise<unknown>;
    act(() => {
      secondArchive = result.current.runTaskRemoval("archive", {
        taskId: TASK_B,
        mutate: () => secondMutation.promise,
      });
    });

    await act(async () => {
      sessionsRead.resolve({ sessions: [{ id: "session-b", task_id: TASK_B }] });
      await sessionsRead.promise;
      await Promise.resolve();
    });

    expect(store.getState().tasks.activeTaskId).not.toBe(TASK_B);
    expect(store.getState().tasks.activeSessionId).not.toBe("session-b");
    expect(softNavigateMock).not.toHaveBeenCalledWith(`/t/${TASK_B}`, "replace");

    await act(async () => {
      firstMutation.resolve();
      secondMutation.resolve();
      await Promise.all([firstArchive, secondArchive]);
    });
    expect(store.getState().tasks.activeTaskId).not.toBe(TASK_B);
    expect(store.getState().tasks.activeSessionId).not.toBe("session-b");
  });
});

describe("useTaskRemoval coordinator — candidate eligibility", () => {
  it("selects the next eligible candidate when a candidate enters another removal", async () => {
    const store = makeStore([
      makeTask(TASK_A, "session-a"),
      makeTask(TASK_B, "session-b"),
      makeTask(TASK_C, "session-c"),
    ]);
    const candidateRead = makeDelayedCandidateRead(TASK_B);
    const firstMutation = deferred<void>();
    const secondMutation = deferred<void>();
    const result = renderCoordinator(store);

    let firstArchive!: Promise<unknown>;
    act(() => {
      firstArchive = result.current.runTaskRemoval("archive", {
        taskId: TASK_A,
        mutate: () => firstMutation.promise,
      });
    });
    let secondArchive!: Promise<unknown>;
    act(() => {
      secondArchive = result.current.runTaskRemoval("archive", {
        taskId: TASK_B,
        mutate: () => secondMutation.promise,
      });
    });

    await act(async () => {
      candidateRead.resolve({ id: TASK_B, archived_at: null });
      await candidateRead.promise;
      await Promise.resolve();
    });

    expect(store.getState().tasks.activeTaskId).toBe(TASK_C);
    expect(store.getState().tasks.activeSessionId).toBe("session-c");
    expect(softNavigateMock).toHaveBeenCalledWith(`/t/${TASK_C}`, "replace");
    expect(softNavigateMock).not.toHaveBeenCalledWith(`/t/${TASK_B}`, "replace");

    await act(async () => {
      firstMutation.resolve();
      secondMutation.resolve();
      await Promise.all([firstArchive, secondArchive]);
    });
  });

  it("opens the overview when every remaining candidate is already pending", async () => {
    const store = makeStore();
    const firstMutation = deferred<void>();
    const secondMutation = deferred<void>();
    const result = renderCoordinator(store);

    let secondArchive!: Promise<unknown>;
    act(() => {
      secondArchive = result.current.runTaskRemoval("archive", {
        taskId: TASK_B,
        mutate: () => secondMutation.promise,
      });
    });

    let firstArchive!: Promise<unknown>;
    act(() => {
      firstArchive = result.current.runTaskRemoval("archive", {
        taskId: TASK_A,
        mutate: () => firstMutation.promise,
      });
    });
    await act(async () => {
      firstMutation.resolve();
      await firstArchive;
    });

    expect(store.getState().tasks.activeTaskId).not.toBe(TASK_B);
    expect(softNavigateMock).not.toHaveBeenCalledWith(`/t/${TASK_B}`, "replace");
    expect(softNavigateMock).toHaveBeenLastCalledWith(OVERVIEW_URL, "replace");

    await act(async () => {
      secondMutation.resolve();
      await secondArchive;
    });
  });
});

describe("useTaskRemoval coordinator — navigation and workspace scope", () => {
  it("keeps later manual navigation instead of committing a delayed candidate", async () => {
    const store = makeStore();
    const candidateRead = makeDelayedCandidateRead(TASK_B);
    const mutation = deferred<void>();
    const result = renderCoordinator(store);

    let archive!: Promise<unknown>;
    act(() => {
      archive = result.current.runTaskRemoval("archive", {
        taskId: TASK_A,
        mutate: () => mutation.promise,
      });
    });

    await act(async () => {
      store.getState().setActiveTask(TASK_C);
      candidateRead.resolve({ id: TASK_B, archived_at: null });
      await candidateRead.promise;
      await Promise.resolve();
    });

    expect(store.getState().tasks.activeTaskId).toBe(TASK_C);
    expect(softNavigateMock).not.toHaveBeenCalledWith(`/t/${TASK_B}`, "replace");

    await act(async () => {
      mutation.resolve();
      await archive;
    });
    expect(store.getState().tasks.activeTaskId).toBe(TASK_C);
    expect(softNavigateMock).not.toHaveBeenCalled();
  });

  it("ignores a pending removal captured for another workspace", async () => {
    const store = makeStore();
    const secondMutation = deferred<void>();
    const result = renderCoordinator(store);

    let secondArchive!: Promise<unknown>;
    act(() => {
      secondArchive = result.current.runTaskRemoval(
        "archive",
        { taskId: TASK_B, mutate: () => secondMutation.promise },
        { workspaceId: "workspace-2" },
      );
    });
    expect(isTaskRemovalPending(store.getState().taskRemoval, TASK_B, WORKSPACE)).toBe(false);

    await act(async () => {
      await result.current.runTaskRemoval("archive", {
        taskId: TASK_A,
        mutate: async () => undefined,
      });
    });

    expect(softNavigateMock).toHaveBeenCalledWith(`/t/${TASK_B}`, "replace");

    await act(async () => {
      secondMutation.resolve();
      await secondArchive;
    });
  });
});
