import { afterEach, describe, expect, it, vi } from "vitest";
import { ApiError } from "@/lib/api/client";
import { createAppStore } from "@/lib/state/store";
import type { TaskNavigationIdentity } from "./task-navigation-reads";
import {
  beginTaskNavigation,
  createTaskNavigationReads,
  isTemporaryTaskNavigationError,
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

afterEach(() => {
  vi.useRealTimers();
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

describe("task navigation retry timing", () => {
  it("shares two bounded automatic retries at two and five seconds", async () => {
    vi.useFakeTimers();
    const load = vi
      .fn<(taskId: string) => Promise<TaskNavigationIdentity>>()
      .mockRejectedValueOnce(new TypeError("network unavailable"))
      .mockRejectedValueOnce(new TypeError("network unavailable"))
      .mockResolvedValueOnce(identity("task-1"));
    const reads = createTaskNavigationReads(load);
    const generation = reads.beginNavigation({}, "/t/task-1");
    const first = reads.read("task-1", generation);
    const foreground = reads.read("task-1", generation, { refresh: true });

    expect(foreground).toBe(first);
    await vi.advanceTimersByTimeAsync(0);
    expect(load).toHaveBeenCalledTimes(1);
    await vi.advanceTimersByTimeAsync(1_999);
    expect(load).toHaveBeenCalledTimes(1);
    await vi.advanceTimersByTimeAsync(1);
    expect(load).toHaveBeenCalledTimes(2);
    await vi.advanceTimersByTimeAsync(4_999);
    expect(load).toHaveBeenCalledTimes(2);
    await vi.advanceTimersByTimeAsync(1);
    await expect(first).resolves.toMatchObject({ task: { id: "task-1" } });
    expect(load).toHaveBeenCalledTimes(3);
  });

  it("honors Retry-After and allows one foreground recovery after exhaustion", async () => {
    vi.useFakeTimers();
    const load = vi
      .fn<(taskId: string) => Promise<TaskNavigationIdentity>>()
      .mockRejectedValueOnce(new ApiError("busy", 503, { code: "persistence_unavailable" }, 8))
      .mockRejectedValueOnce(new ApiError("busy", 503, { code: "persistence_unavailable" }))
      .mockRejectedValueOnce(new ApiError("busy", 503, { code: "persistence_unavailable" }))
      .mockResolvedValueOnce(identity("task-1"));
    const reads = createTaskNavigationReads(load);
    const generation = reads.beginNavigation({}, "/t/task-1");
    const initial = reads.read("task-1", generation);
    const failed = expect(initial).rejects.toMatchObject({ status: 503 });

    await vi.advanceTimersByTimeAsync(0);
    await vi.advanceTimersByTimeAsync(7_999);
    expect(load).toHaveBeenCalledTimes(1);
    await vi.advanceTimersByTimeAsync(1);
    await vi.advanceTimersByTimeAsync(2_000);
    await vi.advanceTimersByTimeAsync(5_000);
    await failed;
    expect(load).toHaveBeenCalledTimes(3);

    await expect(reads.read("task-1", generation, { refresh: true })).rejects.toMatchObject({
      status: 503,
    });
    expect(load).toHaveBeenCalledTimes(3);

    const foregroundEpisode = {};
    const foreground = reads.read("task-1", generation, {
      refresh: true,
      foregroundRecoveryEpisode: foregroundEpisode,
    });
    const joinedForeground = reads.read("task-1", generation, {
      refresh: true,
      foregroundRecoveryEpisode: foregroundEpisode,
    });
    expect(joinedForeground).toBe(foreground);
    await vi.advanceTimersByTimeAsync(0);
    await expect(foreground).resolves.toMatchObject({
      task: { id: "task-1" },
    });
    expect(load).toHaveBeenCalledTimes(4);
    await expect(
      reads.read("task-1", generation, {
        refresh: true,
        foregroundRecoveryEpisode: foregroundEpisode,
      }),
    ).resolves.toMatchObject({ task: { id: "task-1" } });
    expect(load).toHaveBeenCalledTimes(4);
  });

  it("does not restart exhausted permanent failures during foreground recovery", async () => {
    const load = vi.fn().mockRejectedValue(new ApiError("unauthorized", 401, null));
    const reads = createTaskNavigationReads(load);
    const generation = reads.beginNavigation({}, "/t/task-1");
    await expect(reads.read("task-1", generation)).rejects.toMatchObject({ status: 401 });

    await expect(
      reads.read("task-1", generation, {
        refresh: true,
        foregroundRecoveryEpisode: {},
      }),
    ).rejects.toMatchObject({ status: 401 });
    expect(load).toHaveBeenCalledTimes(1);
  });
});

describe("task navigation retry policy", () => {
  it("does not retry permanent, parse, authentication, or abort failures", async () => {
    expect(isTemporaryTaskNavigationError(new TypeError("offline"))).toBe(true);
    expect(isTemporaryTaskNavigationError(new ApiError("busy", 429, null))).toBe(true);
    expect(isTemporaryTaskNavigationError(new ApiError("busy", 502, null))).toBe(true);
    expect(isTemporaryTaskNavigationError(new ApiError("busy", 504, null))).toBe(true);
    expect(
      isTemporaryTaskNavigationError(
        new ApiError("busy", 500, { code: "persistence_unavailable" }),
      ),
    ).toBe(true);
    expect(
      isTemporaryTaskNavigationError(
        new ApiError("busy", 503, { code: "persistence_unavailable" }),
      ),
    ).toBe(true);
    for (const error of [
      new ApiError("missing", 404, null),
      new ApiError("unauthorized", 401, null),
      new ApiError("other server failure", 500, { error_code: "persistence_unavailable" }),
      new SyntaxError("bad JSON"),
      Object.assign(new Error("cancelled"), { name: "AbortError" }),
    ]) {
      expect(isTemporaryTaskNavigationError(error)).toBe(false);
    }

    const load = vi.fn().mockRejectedValue(new ApiError("missing", 404, null));
    const reads = createTaskNavigationReads(load);
    const generation = reads.beginNavigation({}, "/t/task-1");
    await expect(reads.read("task-1", generation)).rejects.toMatchObject({ status: 404 });
    expect(load).toHaveBeenCalledTimes(1);
  });

  it("suspends a scheduled retry while the document is hidden", async () => {
    vi.useFakeTimers();
    const visibility = Object.getOwnPropertyDescriptor(document, "visibilityState");
    Object.defineProperty(document, "visibilityState", { configurable: true, value: "visible" });
    const load = vi
      .fn<(taskId: string) => Promise<TaskNavigationIdentity>>()
      .mockRejectedValueOnce(new TypeError("offline"))
      .mockResolvedValueOnce(identity("task-1"));
    const reads = createTaskNavigationReads(load);
    const generation = reads.beginNavigation({}, "/t/task-1");
    const result = reads.read("task-1", generation);
    await vi.advanceTimersByTimeAsync(0);
    await vi.advanceTimersByTimeAsync(1_000);
    Object.defineProperty(document, "visibilityState", { configurable: true, value: "hidden" });
    document.dispatchEvent(new Event("visibilitychange"));
    await vi.advanceTimersByTimeAsync(5_000);
    expect(load).toHaveBeenCalledTimes(1);
    Object.defineProperty(document, "visibilityState", { configurable: true, value: "visible" });
    document.dispatchEvent(new Event("visibilitychange"));
    await vi.advanceTimersByTimeAsync(999);
    expect(load).toHaveBeenCalledTimes(1);
    await vi.advanceTimersByTimeAsync(1);
    await expect(result).resolves.toMatchObject({ task: { id: "task-1" } });
    Object.defineProperty(
      document,
      "visibilityState",
      visibility ?? { configurable: true, value: "visible" },
    );
  });
});

describe("task navigation attempt cancellation", () => {
  it("aborts each failed attempt's pending session request before retrying", async () => {
    vi.useFakeTimers();
    const sessionSignals: AbortSignal[] = [];
    apiMocks.fetchTask.mockImplementation((_id: string, options: { init?: RequestInit }) => {
      const priorSignal = sessionSignals.at(-1);
      if (priorSignal) expect(priorSignal.aborted).toBe(true);
      expect(options.init?.signal?.aborted).toBe(false);
      return Promise.reject(new ApiError("busy", 503, { code: "persistence_unavailable" }));
    });
    apiMocks.listTaskSessions.mockImplementation((_id: string, options: { init?: RequestInit }) => {
      const signal = options.init?.signal as AbortSignal;
      sessionSignals.push(signal);
      return new Promise((_, reject) => {
        signal.addEventListener("abort", () =>
          reject(Object.assign(new Error("aborted"), { name: "AbortError" })),
        );
      });
    });
    const store = createAppStore();
    const generation = beginTaskNavigation(store, {}, "/t/task-a").generation;
    const result = readTaskNavigationIdentity(store, "task-a", { generation });
    const failure = result.catch((error: unknown) => error);

    await vi.advanceTimersByTimeAsync(0);
    expect(sessionSignals[0]?.aborted).toBe(true);
    await vi.advanceTimersByTimeAsync(2_000);
    expect(apiMocks.fetchTask).toHaveBeenCalledTimes(2);
    expect(sessionSignals[1]?.aborted).toBe(true);
    await vi.advanceTimersByTimeAsync(5_000);
    await expect(failure).resolves.toMatchObject({ status: 503 });

    expect(apiMocks.fetchTask).toHaveBeenCalledTimes(3);
    expect(sessionSignals).toHaveLength(3);
    expect(sessionSignals.every((signal) => signal.aborted)).toBe(true);
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

describe("task navigation attempt cancellation", () => {
  it("does not downgrade an aborted session request to the empty-session fallback", async () => {
    let taskRequestSignal: AbortSignal | undefined;
    let sessionRequestSignal: AbortSignal | undefined;
    apiMocks.fetchTask.mockImplementation((_id: string, options: { init?: RequestInit }) => {
      taskRequestSignal = options.init?.signal ?? undefined;
      return new Promise((_, reject) => {
        taskRequestSignal?.addEventListener("abort", () =>
          reject(Object.assign(new Error("aborted"), { name: "AbortError" })),
        );
      });
    });
    apiMocks.listTaskSessions.mockImplementation((_id: string, options: { init?: RequestInit }) => {
      sessionRequestSignal = options.init?.signal ?? undefined;
      return new Promise((_, reject) => {
        sessionRequestSignal?.addEventListener("abort", () =>
          reject(Object.assign(new Error("aborted"), { name: "AbortError" })),
        );
      });
    });
    const store = createAppStore();
    const owner = {};
    const context = beginTaskNavigation(store, owner, "/t/task-a");
    const result = readTaskNavigationIdentity(store, "task-a", { context });
    beginTaskNavigation(store, owner, "/t/task-b");

    expect(taskRequestSignal?.aborted).toBe(true);
    expect(sessionRequestSignal).toBe(taskRequestSignal);
    await expect(result).rejects.toMatchObject({ name: "AbortError" });
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
