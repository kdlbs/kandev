import { describe, expect, it, vi } from "vitest";
import { createAppStore } from "@/lib/state/store";
import type { TaskNavigationIdentity } from "./task-navigation-reads";
import {
  beginTaskNavigation,
  createTaskNavigationReads,
  isTaskNavigationCurrent,
  readTaskNavigationIdentity,
} from "./task-navigation-reads";

const apiMocks = vi.hoisted(() => ({
  fetchTask: vi.fn(),
  listTaskSessions: vi.fn(),
}));

vi.mock("@/lib/api", () => apiMocks);

function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((next) => {
    resolve = next;
  });
  return { promise, resolve };
}

function identity(taskId: string): TaskNavigationIdentity {
  return {
    task: { id: taskId } as TaskNavigationIdentity["task"],
    allSessionsResponse: { sessions: [], total: 0 },
  };
}

describe("task navigation reads", () => {
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
    expect(reads.read("task-1", generation)).toBe(routeRead);
  });

  it("does not reuse an old result after an A-B-A navigation", async () => {
    const taskAOld = deferred<TaskNavigationIdentity>();
    const taskB = deferred<TaskNavigationIdentity>();
    const taskANew = deferred<TaskNavigationIdentity>();
    const load = vi.fn((taskId: string) => {
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
    expect(newA).not.toBe(oldA);
    await Promise.resolve();
    expect(load).toHaveBeenCalledTimes(3);

    taskAOld.resolve(identity("task-1-old"));
    taskB.resolve(identity("task-2"));
    taskANew.resolve(identity("task-1-new"));
    await expect(oldA).resolves.toMatchObject({ task: { id: "task-1-old" } });
    await expect(b).resolves.toMatchObject({ task: { id: "task-2" } });
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
    await expect(result).rejects.toThrow("Task navigation read scope changed");
  });
});

describe("task navigation store scope", () => {
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
    await expect(oldRead).rejects.toThrow("Task navigation read scope changed");
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
