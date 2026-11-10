import { act, renderHook } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { createAppStore } from "@/lib/state/store";
import { isTaskRemovalPending } from "@/lib/state/task-removal";
import type { SidebarTaskPageResponse, Task } from "@/lib/types/http";

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
const TASK_P = "task-parent";
const TASK_D = "task-child";
const TASK_C = "task-cached-candidate";
const WORKSPACE = "workspace-1";

function deferred<T>() {
  let resolve!: (value: T | PromiseLike<T>) => void;
  const promise = new Promise<T>((res) => {
    resolve = res;
  });
  return { promise, resolve };
}

function makeTask(
  id: string,
  opts: { sessionId?: string | null; parentTaskId?: string | null } = {},
) {
  return {
    id,
    workspaceId: WORKSPACE,
    workflowId: "workflow-1",
    workflowStepId: "step-1",
    title: id,
    position: 0,
    primarySessionId: opts.sessionId ?? null,
    parentTaskId: opts.parentTaskId ?? null,
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

function makeServerTask(
  id: string,
  opts: { sessionId?: string | null; parentTaskId?: string | null } = {},
): Task {
  return {
    id,
    workspace_id: WORKSPACE,
    workflow_id: "workflow-1",
    workflow_step_id: "step-1",
    title: id,
    state: "IN_PROGRESS",
    priority: "medium",
    position: 0,
    primary_session_id: opts.sessionId ?? null,
    parent_id: opts.parentTaskId ?? undefined,
    created_at: "2026-09-26T10:00:00Z",
    updated_at: "2026-09-26T10:00:00Z",
  } as unknown as Task;
}

function serverPage(tasks: Task[]): SidebarTaskPageResponse {
  return {
    entries: tasks.map((task) => ({ kind: "task", task_id: task.id, task })),
    has_next: false,
  } as unknown as SidebarTaskPageResponse;
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

function startCascadeRemoval(
  result: ReturnType<typeof renderCoordinator>,
  action: "archive" | "delete",
  mutation: Promise<void>,
): Promise<unknown> {
  let removal!: Promise<unknown>;
  act(() => {
    removal = result.current.runTaskRemoval(
      action,
      { taskId: TASK_P, mutate: () => mutation },
      { cascade: true },
    );
  });
  return removal;
}

/** P's cached liveness read is held; D is only in the server fallback page. */
function holdParentRead(extraReads: Record<string, unknown> = {}) {
  const read = deferred<{ id: string; archived_at: null }>();
  fetchTaskMock.mockImplementation(async (taskId: string) => {
    if (taskId === TASK_P) return read.promise;
    if (taskId === TASK_D) {
      return { id: TASK_D, archived_at: null, workspace_id: WORKSPACE, parent_id: TASK_P };
    }
    if (extraReads[taskId]) return extraReads[taskId];
    return { id: taskId, archived_at: null, workspace_id: WORKSPACE };
  });
  return read;
}

beforeEach(() => {
  vi.clearAllMocks();
  fetchTaskMock.mockImplementation(async (taskId: string) => ({
    id: taskId,
    archived_at: null,
  }));
  listTaskSessionsMock.mockResolvedValue({ sessions: [] });
  querySidebarTasksMock.mockResolvedValue(serverPage([]));
});

async function checkPendingCascadeFallback(action: "archive" | "delete") {
  const store = makeStore([
    makeTask(TASK_A, { sessionId: "session-a" }),
    makeTask(TASK_P, { sessionId: "session-p" }),
  ]);
  const parentRead = holdParentRead();
  querySidebarTasksMock.mockResolvedValue(
    serverPage([
      makeServerTask(TASK_D, { parentTaskId: TASK_P }),
      makeServerTask(TASK_C, { sessionId: "session-c" }),
    ]),
  );
  const mutation = deferred<void>();
  const cascadeMutation = deferred<void>();
  const result = renderCoordinator(store);

  const removal = startSelectedRemoval(result, mutation.promise);
  await vi.waitFor(() => expect(fetchTaskMock).toHaveBeenCalledWith(TASK_P, { cache: "no-store" }));

  const cascadeRemoval = startCascadeRemoval(result, action, cascadeMutation.promise);

  await act(async () => {
    parentRead.resolve({ id: TASK_P, archived_at: null });
    await parentRead.promise;
    await Promise.resolve();
  });

  await vi.waitFor(() => expect(store.getState().tasks.activeTaskId).toBe(TASK_C));
  expect(softNavigateMock).toHaveBeenCalledWith(`/t/${TASK_C}`, "replace");
  expect(softNavigateMock).not.toHaveBeenCalledWith(`/t/${TASK_D}`, "replace");

  await act(async () => {
    cascadeMutation.resolve();
    await cascadeRemoval;
    mutation.resolve();
    await removal;
  });
  expect(store.getState().tasks.activeTaskId).toBe(TASK_C);
}

describe("cascade ancestry across an independent removal", () => {
  it.each(["archive", "delete"] as const)(
    "does not select a fallback child covered by a pending cascade %s",
    (action) => checkPendingCascadeFallback(action),
  );

  it("does not select a fallback child after its cascade removal completed", async () => {
    const store = makeStore([
      makeTask(TASK_A, { sessionId: "session-a" }),
      makeTask(TASK_P, { sessionId: "session-p" }),
    ]);
    const parentRead = holdParentRead();
    querySidebarTasksMock.mockResolvedValue(
      serverPage([
        makeServerTask(TASK_D, { parentTaskId: TASK_P }),
        makeServerTask(TASK_C, { sessionId: "session-c" }),
      ]),
    );
    const mutation = deferred<void>();
    const result = renderCoordinator(store);

    const removal = startSelectedRemoval(result, mutation.promise);
    const cascadeRemoval = startCascadeRemoval(result, "archive", Promise.resolve());

    await act(async () => {
      await cascadeRemoval;
    });
    expect(isTaskRemovalPending(store.getState().taskRemoval, TASK_P, WORKSPACE)).toBe(false);
    expect(store.getState().kanban.tasks.some((task) => task.id === TASK_P)).toBe(false);

    await act(async () => {
      parentRead.resolve({ id: TASK_P, archived_at: null });
      await parentRead.promise;
      await Promise.resolve();
    });

    await vi.waitFor(() => expect(store.getState().tasks.activeTaskId).toBe(TASK_C));
    expect(softNavigateMock).not.toHaveBeenCalledWith(`/t/${TASK_D}`, "replace");

    await act(async () => {
      mutation.resolve();
      await removal;
    });
    expect(store.getState().tasks.activeTaskId).toBe(TASK_C);
  });

  it("rechecks a fallback child's ancestry before the final switch", async () => {
    const store = makeStore([makeTask(TASK_A, { sessionId: "session-a" })]);
    const sessionsRead = deferred<{ sessions: Array<{ id: string; task_id: string }> }>();
    listTaskSessionsMock.mockImplementation((taskId: string) =>
      taskId === TASK_D ? sessionsRead.promise : Promise.resolve({ sessions: [] }),
    );
    fetchTaskMock.mockImplementation(async (taskId: string) => {
      if (taskId === TASK_D) {
        return { id: TASK_D, archived_at: null, workspace_id: WORKSPACE, parent_id: TASK_P };
      }
      return { id: taskId, archived_at: null, workspace_id: WORKSPACE };
    });
    querySidebarTasksMock.mockResolvedValue(
      serverPage([makeServerTask(TASK_D), makeServerTask(TASK_C, { sessionId: "session-c" })]),
    );
    const mutation = deferred<void>();
    const cascadeMutation = deferred<void>();
    const result = renderCoordinator(store);

    const removal = startSelectedRemoval(result, mutation.promise);
    await vi.waitFor(() =>
      expect(listTaskSessionsMock).toHaveBeenCalledWith(TASK_D, { cache: "no-store" }),
    );

    const cascadeRemoval = startCascadeRemoval(result, "archive", cascadeMutation.promise);

    await act(async () => {
      sessionsRead.resolve({ sessions: [{ id: "session-d", task_id: TASK_D }] });
      await sessionsRead.promise;
      await Promise.resolve();
    });

    await vi.waitFor(() => expect(store.getState().tasks.activeTaskId).toBe(TASK_C));
    expect(store.getState().tasks.activeSessionId).toBe("session-c");
    expect(softNavigateMock).not.toHaveBeenCalledWith(`/t/${TASK_D}`, "replace");

    await act(async () => {
      cascadeMutation.resolve();
      await cascadeRemoval;
      mutation.resolve();
      await removal;
    });
    expect(store.getState().tasks.activeTaskId).toBe(TASK_C);
  });
});
