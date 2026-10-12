import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { ApiError } from "@/lib/api/client";
import type { TaskEnvironmentLiveResponse } from "@/lib/api/domains/task-environment-api";
import { createEnvironmentLiveResource } from "./environment-live-resource";

function deferred<T>() {
  let resolve: (value: T) => void = () => undefined;
  let reject: (error: unknown) => void = () => undefined;
  const promise = new Promise<T>((resolvePromise, rejectPromise) => {
    resolve = resolvePromise;
    reject = rejectPromise;
  });
  return { promise, resolve, reject };
}

function response(taskId: string): TaskEnvironmentLiveResponse {
  return {
    environment: {
      id: `environment-${taskId}`,
      task_id: taskId,
      repository_id: "repository-1",
      executor_type: "docker",
      executor_id: "executor-1",
      executor_profile_id: "profile-1",
      agent_execution_id: "execution-1",
      control_port: 8765,
      status: "ready",
      created_at: "2026-09-29T00:00:00Z",
    },
  };
}

beforeEach(() => vi.useFakeTimers());
afterEach(() => vi.useRealTimers());

describe("environment live resource sharing", () => {
  it("shares one read and one fastest-cadence timer within a store and task", async () => {
    const firstRequest = deferred<TaskEnvironmentLiveResponse>();
    const requester = vi
      .fn<(taskId: string) => Promise<TaskEnvironmentLiveResponse>>()
      .mockReturnValueOnce(firstRequest.promise)
      .mockImplementation(async (taskId) => response(taskId));
    const resource = createEnvironmentLiveResource(requester);
    const store = {};
    const firstConsumer = Symbol("toolbar");
    const secondConsumer = Symbol("composer");
    const firstListener = vi.fn();
    const secondListener = vi.fn();
    const releaseFirst = resource.subscribe(store, "task-1", firstConsumer, firstListener);
    const releaseSecond = resource.subscribe(store, "task-1", secondConsumer, secondListener);

    resource.setActive(store, "task-1", firstConsumer, false);
    resource.setActive(store, "task-1", secondConsumer, true);
    expect(requester).toHaveBeenCalledTimes(1);
    expect(vi.getTimerCount()).toBe(1);

    firstRequest.resolve(response("task-1"));
    await firstRequest.promise;
    await Promise.resolve();
    expect(resource.getSnapshot(store, "task-1").response?.environment.task_id).toBe("task-1");

    await vi.advanceTimersByTimeAsync(2999);
    expect(requester).toHaveBeenCalledTimes(1);
    await vi.advanceTimersByTimeAsync(1);
    expect(requester).toHaveBeenCalledTimes(2);
    expect(firstListener).toHaveBeenCalled();
    expect(secondListener).toHaveBeenCalled();

    releaseFirst();
    releaseSecond();
    expect(vi.getTimerCount()).toBe(0);
  });
});

describe("environment live resource invalidation", () => {
  it("restarts after invalidation without publishing the pre-reset response", async () => {
    const staleRequest = deferred<TaskEnvironmentLiveResponse>();
    const requestAfterReset = deferred<TaskEnvironmentLiveResponse>();
    const requester = vi
      .fn<(taskId: string) => Promise<TaskEnvironmentLiveResponse>>()
      .mockReturnValueOnce(staleRequest.promise)
      .mockReturnValueOnce(requestAfterReset.promise);
    const resource = createEnvironmentLiveResource(requester);
    const store = {};
    const consumer = Symbol("toolbar");
    const listener = vi.fn();
    const release = resource.subscribe(store, "task-1", consumer, listener);

    resource.invalidate(store, "task-1");
    expect(resource.getSnapshot(store, "task-1")).toMatchObject({
      response: null,
      loading: false,
      notFound: false,
    });

    staleRequest.resolve(response("task-1"));
    await staleRequest.promise;
    await Promise.resolve();
    await Promise.resolve();
    expect(requester).toHaveBeenCalledTimes(2);
    expect(resource.getSnapshot(store, "task-1").response).toBeNull();

    requestAfterReset.resolve(response("task-1"));
    await requestAfterReset.promise;
    await Promise.resolve();
    expect(resource.getSnapshot(store, "task-1").response?.environment.id).toBe(
      "environment-task-1",
    );
    release();
  });
});

describe("environment live resource lifecycle", () => {
  it("keeps stores and task identities isolated and retires the last consumer", async () => {
    const taskOneRequest = deferred<TaskEnvironmentLiveResponse>();
    const taskTwoRequest = deferred<TaskEnvironmentLiveResponse>();
    const requester = vi
      .fn<(taskId: string) => Promise<TaskEnvironmentLiveResponse>>()
      .mockReturnValueOnce(taskOneRequest.promise)
      .mockReturnValueOnce(taskTwoRequest.promise)
      .mockImplementation(async (taskId) => response(taskId));
    const resource = createEnvironmentLiveResource(requester);
    const storeOne = {};
    const storeTwo = {};
    const taskOneConsumer = Symbol("task-one");
    const taskTwoConsumer = Symbol("task-two");
    const releaseTaskOne = resource.subscribe(storeOne, "task-1", taskOneConsumer, vi.fn());
    const releaseTaskTwo = resource.subscribe(storeTwo, "task-1", taskTwoConsumer, vi.fn());

    expect(requester).toHaveBeenCalledTimes(2);
    releaseTaskOne();
    taskOneRequest.resolve(response("task-1"));
    await taskOneRequest.promise;
    await Promise.resolve();
    expect(resource.getSnapshot(storeOne, "task-1").response).toBeNull();
    expect(resource.getSnapshot(storeTwo, "task-1").response).toBeNull();

    taskTwoRequest.resolve(response("task-1"));
    await taskTwoRequest.promise;
    await Promise.resolve();
    expect(resource.getSnapshot(storeTwo, "task-1").response?.environment.task_id).toBe("task-1");
    releaseTaskTwo();

    const taskOneNextConsumer = Symbol("task-one-next");
    const releaseTaskOneNext = resource.subscribe(storeOne, "task-2", taskOneNextConsumer, vi.fn());
    expect(requester).toHaveBeenCalledWith("task-2");
    releaseTaskOneNext();
  });

  it("treats a missing environment as settled while retaining data on other failures", async () => {
    const requester = vi
      .fn<(taskId: string) => Promise<TaskEnvironmentLiveResponse>>()
      .mockRejectedValueOnce(new ApiError("missing", 404, null))
      .mockResolvedValueOnce(response("task-1"))
      .mockRejectedValueOnce(new Error("temporary failure"));
    const resource = createEnvironmentLiveResource(requester);
    const store = {};
    const consumer = Symbol("toolbar");
    const release = resource.subscribe(store, "task-1", consumer, vi.fn());

    await vi.waitFor(() => expect(resource.getSnapshot(store, "task-1").notFound).toBe(true));
    await resource.refresh(store, "task-1");
    expect(resource.getSnapshot(store, "task-1").response?.environment.task_id).toBe("task-1");
    await resource.refresh(store, "task-1");
    expect(resource.getSnapshot(store, "task-1").response?.environment.task_id).toBe("task-1");
    release();
  });
});
