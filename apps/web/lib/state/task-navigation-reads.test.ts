import { afterEach, describe, expect, it, vi } from "vitest";
import { createAppStore } from "@/lib/state/store";
import { applyTaskOverviewPatch } from "@/lib/ws/handlers/task-overview";
import type { TaskNavigationIdentity } from "./task-navigation-reads";
import {
  beginTaskNavigation,
  createTaskNavigationReads,
  isTaskNavigationCurrent,
  readTaskNavigationIdentity,
} from "./task-navigation-reads";
import {
  ACTIVE_TASK_ID,
  RETAINED_TASK_ID,
  deferred,
  identity,
} from "./task-navigation-reads.test-helpers";

const apiMocks = vi.hoisted(() => ({
  fetchTask: vi.fn(),
  listTaskSessions: vi.fn(),
}));

vi.mock("@/lib/api", () => apiMocks);

afterEach(() => {
  vi.useRealTimers();
});

describe("task navigation canonical overview refresh", () => {
  afterEach(() => {
    apiMocks.fetchTask.mockReset();
    apiMocks.listTaskSessions.mockReset();
  });

  function makeStore() {
    return createAppStore({
      workspaces: { activeId: "workspace", items: [] },
      tasks: { activeTaskId: ACTIVE_TASK_ID },
      kanban: {
        workflowId: "workflow",
        tasks: [
          {
            id: ACTIVE_TASK_ID,
            workspaceId: "workspace",
            workflowId: "workflow",
            workflowStepId: "in-progress",
            updatedAt: "2026-10-15T10:00:00Z",
            title: "Original task",
          },
        ],
      },
    } as never);
  }

  function refreshedTask() {
    return {
      id: ACTIVE_TASK_ID,
      workspace_id: "workspace",
      workflow_id: "workflow",
      workflow_step_id: "review",
      updated_at: "2026-10-15T10:01:00Z",
      title: "Refreshed task",
    } as TaskNavigationIdentity["task"];
  }

  it("repairs shared placement after a move missed before websocket connection", async () => {
    const store = makeStore();
    apiMocks.fetchTask.mockResolvedValue(refreshedTask());
    apiMocks.listTaskSessions.mockResolvedValue({ sessions: [], total: 0 });

    await readTaskNavigationIdentity(store, ACTIVE_TASK_ID);

    expect(store.getState().taskOverview.byId[ACTIVE_TASK_ID]).toMatchObject({
      workflowStepId: "review",
      title: "Refreshed task",
    });
    expect(store.getState().kanban.tasks[0].workflowStepId).toBe("review");
    expect(store.getState().taskOverview.reads).toEqual({});
  });

  it("preserves a live placement patch while accepting other refreshed fields", async () => {
    const store = makeStore();
    const response = deferred<TaskNavigationIdentity["task"]>();
    apiMocks.fetchTask.mockReturnValue(response.promise);
    apiMocks.listTaskSessions.mockResolvedValue({ sessions: [], total: 0 });
    const read = readTaskNavigationIdentity(store, ACTIVE_TASK_ID);
    await Promise.resolve();
    store.setState((state) =>
      applyTaskOverviewPatch(
        state,
        ACTIVE_TASK_ID,
        {
          id: ACTIVE_TASK_ID,
          workflowStepId: "done",
        },
        "workspace",
      ),
    );
    response.resolve(refreshedTask());
    await read;

    expect(store.getState().taskOverview.byId[ACTIVE_TASK_ID]).toMatchObject({
      workflowStepId: "done",
      title: "Refreshed task",
    });
    expect(store.getState().taskOverview.reads).toEqual({});
  });

  it("discards a refresh after its workspace scope changes", async () => {
    const store = makeStore();
    const response = deferred<TaskNavigationIdentity["task"]>();
    apiMocks.fetchTask.mockReturnValue(response.promise);
    apiMocks.listTaskSessions.mockResolvedValue({ sessions: [], total: 0 });
    const read = readTaskNavigationIdentity(store, ACTIVE_TASK_ID);
    const outcome = read.catch((error: unknown) => error);
    await Promise.resolve();
    store.setState((state) => ({
      workspaces: { ...state.workspaces, activeId: "other-workspace" },
      workspaceContextGeneration: state.workspaceContextGeneration + 1,
    }));
    response.resolve(refreshedTask());

    expect(await outcome).toMatchObject({ name: "AbortError" });
    expect(store.getState().taskOverview.byId[ACTIVE_TASK_ID]).toBeUndefined();
    expect(store.getState().taskOverview.reads).toEqual({});
  });
});

describe("task navigation store scope", () => {
  it("keeps task identity available when the session list fails", async () => {
    apiMocks.fetchTask.mockResolvedValue({
      id: "task-a",
      primary_session_id: "session-a",
    } as TaskNavigationIdentity["task"]);
    apiMocks.listTaskSessions.mockRejectedValue(new Error("session list unavailable"));
    const store = createAppStore();

    const result = await readTaskNavigationIdentity(store, "task-a");

    expect(result.task.id).toBe("task-a");
    expect(result.sessionListUnavailable).toBe(true);
    expect(result.allSessionsResponse.sessions).toEqual([]);
    expect(store.getState().tasks.activeTaskId).toBeNull();
  });
});

describe("task navigation store scope identity", () => {
  it("rejects an old-store read after a new identity owner replaces its scope", async () => {
    const taskA = deferred<TaskNavigationIdentity["task"]>();
    const sessionsA = deferred<TaskNavigationIdentity["allSessionsResponse"]>();
    const taskB = deferred<TaskNavigationIdentity["task"]>();
    const sessionsB = deferred<TaskNavigationIdentity["allSessionsResponse"]>();
    apiMocks.fetchTask.mockImplementation((id: string) =>
      id === "task-a" ? taskA.promise : taskB.promise,
    );
    apiMocks.listTaskSessions.mockImplementation((id: string) =>
      id === "task-a" ? sessionsA.promise : sessionsB.promise,
    );
    const store = createAppStore();
    const owner = {};
    const contextA = beginTaskNavigation(store, owner, "/t/task-a");
    const publishedTasks: string[] = [];
    const oldRead = readTaskNavigationIdentity(store, "task-a", { context: contextA }).then(
      (result) => {
        if (!isTaskNavigationCurrent(store, contextA)) return;
        store.getState().setActiveTask(result.task.id);
        store
          .getState()
          .setTaskSessionsForTask(result.task.id, result.allSessionsResponse.sessions ?? [], {});
        publishedTasks.push(result.task.id);
      },
    );

    store.getState().setActiveWorkspace("workspace-b");
    const contextB = beginTaskNavigation(store, owner, "/t/task-b");
    const newRead = readTaskNavigationIdentity(store, "task-b", { context: contextB }).then(
      (result) => {
        if (!isTaskNavigationCurrent(store, contextB)) return;
        store.getState().setActiveTask(result.task.id);
        store
          .getState()
          .setTaskSessionsForTask(result.task.id, result.allSessionsResponse.sessions ?? [], {});
        publishedTasks.push(result.task.id);
      },
    );

    taskA.resolve({ id: "task-a" } as TaskNavigationIdentity["task"]);
    sessionsA.resolve({
      sessions: [{ id: "session-a", task_id: "task-a" }],
      total: 1,
    } as TaskNavigationIdentity["allSessionsResponse"]);
    await expect(oldRead).rejects.toMatchObject({ name: "AbortError" });
    expect(publishedTasks).toEqual([]);
    expect(store.getState().tasks.activeTaskId).toBeNull();
    expect(store.getState().taskSessionsByTask.loadedByTaskId["task-a"]).toBeUndefined();

    taskB.resolve({ id: "task-b" } as TaskNavigationIdentity["task"]);
    sessionsB.resolve({
      sessions: [{ id: "session-b", task_id: "task-b" }],
      total: 1,
    } as TaskNavigationIdentity["allSessionsResponse"]);
    await newRead;
    expect(publishedTasks).toEqual(["task-b"]);
    expect(store.getState().tasks.activeTaskId).toBe("task-b");
    expect(store.getState().taskSessionsByTask.loadedByTaskId["task-b"]).toBe(true);
  });
});

describe("task navigation read sharing", () => {
  it("joins route and task-surface identity reads and reuses the resolved result", async () => {
    const response = deferred<TaskNavigationIdentity>();
    const load = vi.fn(() => response.promise);
    const reads = createTaskNavigationReads(load);
    const owner = {};
    const generation = reads.beginNavigation(owner, "/t/task-1");

    const routeRead = reads.read("task-1", generation);
    const taskSurfaceRead = reads.read("task-1", generation);
    expect(taskSurfaceRead).toBe(routeRead);
    await Promise.resolve();
    expect(load).toHaveBeenCalledTimes(1);

    response.resolve(identity("task-1"));
    await expect(routeRead).resolves.toMatchObject({ task: { id: "task-1" } });
    await expect(reads.read("task-1", generation)).resolves.toMatchObject({
      task: { id: "task-1" },
    });
  });

  it("does not reuse an old result after an A-B-A navigation", async () => {
    const taskAOld = deferred<TaskNavigationIdentity>();
    const taskB = deferred<TaskNavigationIdentity>();
    const taskANew = deferred<TaskNavigationIdentity>();
    const signals: AbortSignal[] = [];
    const load = vi.fn((taskId: string, signal: AbortSignal) => {
      signals.push(signal);
      if (taskId !== "task-1") return taskB.promise;
      return load.mock.calls.filter(([id]) => id === "task-1").length === 1
        ? taskAOld.promise
        : taskANew.promise;
    });
    const reads = createTaskNavigationReads(load);
    const owner = {};
    const oldAGeneration = reads.beginNavigation(owner, "/t/task-1");
    const oldA = reads.read("task-1", oldAGeneration);
    const bGeneration = reads.beginNavigation(owner, "/t/task-2");
    const b = reads.read("task-2", bGeneration);
    const newAGeneration = reads.beginNavigation(owner, "/t/task-1");
    const newA = reads.read("task-1", newAGeneration);

    expect(reads.isCurrent(oldAGeneration)).toBe(false);
    expect(signals[0].aborted).toBe(true);
    expect(newA).not.toBe(oldA);
    await Promise.resolve();
    expect(load).toHaveBeenCalledTimes(3);

    taskAOld.resolve(identity("task-1-old"));
    taskB.resolve(identity("task-2"));
    taskANew.resolve(identity("task-1-new"));
    await expect(oldA).rejects.toMatchObject({ name: "AbortError" });
    await expect(b).rejects.toMatchObject({ name: "AbortError" });
    await expect(newA).resolves.toMatchObject({ task: { id: "task-1-new" } });
    expect(reads.isCurrent(newAGeneration)).toBe(true);
  });

  it("discards a read when its authentication scope changes", async () => {
    const response = deferred<TaskNavigationIdentity>();
    const load = vi.fn(() => response.promise);
    let scopeIsCurrent = true;
    const reads = createTaskNavigationReads(load, () => scopeIsCurrent);
    const generation = reads.beginNavigation({}, "/t/task-1");
    const result = reads.read("task-1", generation);

    scopeIsCurrent = false;
    response.resolve(identity("task-1"));
    await expect(result).rejects.toMatchObject({ name: "AbortError" });
  });

  it("aborts both identity requests when the route owner releases its read", async () => {
    let requestSignal: AbortSignal | undefined;
    const reads = createTaskNavigationReads((_, signal) => {
      requestSignal = signal;
      return new Promise((_, reject) => {
        signal.addEventListener("abort", () =>
          reject(Object.assign(new Error(), { name: "AbortError" })),
        );
      });
    });
    const generation = reads.beginNavigation({}, "/t/task-1");
    const release = reads.retain("task-1", generation);
    const result = reads.read("task-1", generation);
    release();
    await Promise.resolve();

    expect(requestSignal?.aborted).toBe(true);
    await expect(result).rejects.toMatchObject({ name: "AbortError" });
  });
});

describe("task navigation read retention", () => {
  // @covers AC-PLATFORM-INTERACTIVE-READS-005.3
  it("keeps active and retained reads when older records are pruned", async () => {
    const activeResponse = deferred<TaskNavigationIdentity>();
    let activeSignal: AbortSignal | undefined;
    const load = vi.fn((taskId: string, signal: AbortSignal) => {
      if (taskId === ACTIVE_TASK_ID) {
        activeSignal = signal;
        return activeResponse.promise;
      }
      return Promise.resolve(identity(taskId));
    });
    const reads = createTaskNavigationReads(load);
    const generation = reads.beginNavigation({}, `/t/${ACTIVE_TASK_ID}`);
    const activeRead = reads.read(ACTIVE_TASK_ID, generation);
    const activeOutcome = activeRead.then(
      () => "resolved",
      () => "rejected",
    );
    const releaseRetained = reads.retain(RETAINED_TASK_ID, generation);
    try {
      await reads.read(RETAINED_TASK_ID, generation);
      for (let id = 2; id <= 8; id++) {
        await reads.read(`task-${id}`, generation);
      }

      expect(activeSignal?.aborted).toBe(false);
      expect(reads.getSnapshot(ACTIVE_TASK_ID, generation).phase).toBe("loading");
      expect(reads.getSnapshot(RETAINED_TASK_ID, generation).phase).toBe("succeeded");
    } finally {
      activeResponse.resolve(identity(ACTIVE_TASK_ID));
      releaseRetained();
    }

    await expect(activeOutcome).resolves.toBe("resolved");
  });
});
