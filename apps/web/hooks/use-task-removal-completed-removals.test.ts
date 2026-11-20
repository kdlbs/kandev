import { act, renderHook } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { createAppStore } from "@/lib/state/store";
import { isTaskRemovalPending } from "@/lib/state/task-removal";

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

function deferred<T>() {
  let resolve!: (value: T | PromiseLike<T>) => void;
  const promise = new Promise<T>((res) => {
    resolve = res;
  });
  return { promise, resolve };
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

function makeStore(tasks: ReturnType<typeof makeTask>[]) {
  const store = createAppStore();
  // The workspace scope must settle before board writes: a later activeId
  // change isolates unchanged projections and drops their rows.
  store.setState((state) => ({
    ...state,
    workspaces: { ...state.workspaces, activeId: WORKSPACE },
  }));
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
  store.getState().setActiveSession(TASK_A, "session-a");
  return store;
}

function makeThreeTaskStore(sessionIdForB: string | null = "session-b") {
  return makeStore([
    makeTask(TASK_A, "session-a"),
    makeTask(TASK_B, sessionIdForB),
    makeTask(TASK_C, "session-c"),
  ]);
}

function makeDelayedCandidateRead(taskId: string) {
  const read = deferred<{ id: string; archived_at: null }>();
  fetchTaskMock.mockImplementation(async (candidateId: string) =>
    candidateId === taskId ? read.promise : { id: candidateId, archived_at: null },
  );
  return read;
}

function renderCoordinator(store: ReturnType<typeof makeStore>) {
  return renderHook(() => useTaskRemoval({ store })).result;
}

function startSelectedRemoval(
  result: ReturnType<typeof renderCoordinator>,
  mutation: Promise<void>,
): Promise<unknown> {
  let removal!: Promise<unknown>;
  act(() => {
    removal = result.current.runTaskRemoval("archive", {
      taskId: TASK_A,
      mutate: () => mutation,
    });
  });
  return removal;
}

async function settleIndependentRemoval(
  store: ReturnType<typeof makeStore>,
  result: ReturnType<typeof renderCoordinator>,
  action: "archive" | "delete",
  taskId: string,
) {
  let removal!: Promise<unknown>;
  act(() => {
    removal = result.current.runTaskRemoval(action, {
      taskId,
      mutate: async () => undefined,
    });
  });
  await act(async () => {
    await removal;
  });
  expect(isTaskRemovalPending(store.getState().taskRemoval, taskId, WORKSPACE)).toBe(false);
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

describe("completed archive during a delayed candidate read", () => {
  it("selects the next eligible task, never the archived candidate", async () => {
    const store = makeThreeTaskStore();
    const candidateRead = makeDelayedCandidateRead(TASK_B);
    const mutation = deferred<void>();
    const result = renderCoordinator(store);
    const removal = startSelectedRemoval(result, mutation.promise);
    await vi.waitFor(() =>
      expect(fetchTaskMock).toHaveBeenCalledWith(TASK_B, { cache: "no-store" }),
    );

    await settleIndependentRemoval(store, result, "archive", TASK_B);
    expect(store.getState().kanban.tasks.some((task) => task.id === TASK_B)).toBe(false);

    await act(async () => {
      candidateRead.resolve({ id: TASK_B, archived_at: null });
      await candidateRead.promise;
      await Promise.resolve();
    });

    expect(store.getState().tasks.activeTaskId).toBe(TASK_C);
    expect(softNavigateMock).toHaveBeenCalledWith(`/t/${TASK_C}`, "replace");
    expect(softNavigateMock).not.toHaveBeenCalledWith(`/t/${TASK_B}`, "replace");

    await act(async () => {
      mutation.resolve();
      await removal;
    });
    expect(store.getState().tasks.activeTaskId).toBe(TASK_C);
  });
});

describe("completed deletion during a delayed candidate read", () => {
  it("selects the next eligible task, never the deleted candidate", async () => {
    const store = makeThreeTaskStore();
    const candidateRead = makeDelayedCandidateRead(TASK_B);
    const mutation = deferred<void>();
    const result = renderCoordinator(store);
    const removal = startSelectedRemoval(result, mutation.promise);

    await settleIndependentRemoval(store, result, "delete", TASK_B);

    await act(async () => {
      candidateRead.resolve({ id: TASK_B, archived_at: null });
      await candidateRead.promise;
      await Promise.resolve();
    });

    expect(store.getState().tasks.activeTaskId).toBe(TASK_C);
    expect(softNavigateMock).not.toHaveBeenCalledWith(`/t/${TASK_B}`, "replace");

    await act(async () => {
      mutation.resolve();
      await removal;
    });
    expect(store.getState().tasks.activeTaskId).toBe(TASK_C);
  });
});

describe("completed archive during a delayed session read", () => {
  it("selects the next eligible task, never the archived candidate", async () => {
    const store = makeThreeTaskStore(null);
    const sessionsRead = deferred<{ sessions: Array<{ id: string; task_id: string }> }>();
    listTaskSessionsMock.mockImplementation((taskId: string) =>
      taskId === TASK_B ? sessionsRead.promise : Promise.resolve({ sessions: [] }),
    );
    const mutation = deferred<void>();
    const result = renderCoordinator(store);
    const removal = startSelectedRemoval(result, mutation.promise);
    await vi.waitFor(() =>
      expect(listTaskSessionsMock).toHaveBeenCalledWith(TASK_B, { cache: "no-store" }),
    );

    await settleIndependentRemoval(store, result, "archive", TASK_B);

    await act(async () => {
      sessionsRead.resolve({ sessions: [{ id: "session-b", task_id: TASK_B }] });
      await sessionsRead.promise;
      await Promise.resolve();
    });

    expect(store.getState().tasks.activeTaskId).toBe(TASK_C);
    expect(store.getState().tasks.activeSessionId).toBe("session-c");
    expect(softNavigateMock).not.toHaveBeenCalledWith(`/t/${TASK_B}`, "replace");

    await act(async () => {
      mutation.resolve();
      await removal;
    });
    expect(store.getState().tasks.activeTaskId).toBe(TASK_C);
  });
});
