import { act, cleanup, renderHook, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { TaskEnvironmentLiveResponse } from "@/lib/api/domains/task-environment-api";

const mocks = vi.hoisted(() => ({ fetchTaskEnvironmentLive: vi.fn() }));

vi.mock("@/lib/api/domains/task-environment-api", () => ({
  fetchTaskEnvironmentLive: mocks.fetchTaskEnvironmentLive,
}));

import {
  isExecutorEnvironmentUnavailable,
  useExecutorEnvironmentAvailability,
} from "./use-executor-environment-availability";
import type { ExecutorEnvironmentStatus } from "@/components/task/executor-environment-status";

function status(label: string, tone: ExecutorEnvironmentStatus["tone"]) {
  return { label, tone };
}

function environmentResponse(taskId: string): TaskEnvironmentLiveResponse {
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

function deferred<T>() {
  let resolve: (value: T) => void = () => undefined;
  const promise = new Promise<T>((resolvePromise) => {
    resolve = resolvePromise;
  });
  return { promise, resolve };
}

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

describe("isExecutorEnvironmentUnavailable", () => {
  it("does not block before an environment exists or while it is starting", () => {
    expect(isExecutorEnvironmentUnavailable(null)).toBe(false);
    expect(isExecutorEnvironmentUnavailable(status("starting", "warn"))).toBe(false);
  });

  it("blocks when the environment is no longer usable", () => {
    expect(isExecutorEnvironmentUnavailable(status("exited (137)", "error"))).toBe(true);
    expect(isExecutorEnvironmentUnavailable(status("paused", "warn"))).toBe(true);
    expect(isExecutorEnvironmentUnavailable(status("missing", "warn"))).toBe(true);
  });
});

describe("useExecutorEnvironmentAvailability", () => {
  it("does not read while disabled and refreshes for the newly selected task", async () => {
    const initialRead = deferred<TaskEnvironmentLiveResponse>();
    mocks.fetchTaskEnvironmentLive
      .mockReturnValueOnce(initialRead.promise)
      .mockImplementation(async (taskId: string) => environmentResponse(taskId));
    const hook = renderHook(
      ({ taskId, enabled }) => useExecutorEnvironmentAvailability(taskId, enabled),
      { initialProps: { taskId: "task-1", enabled: false } },
    );

    expect(mocks.fetchTaskEnvironmentLive).not.toHaveBeenCalled();
    hook.rerender({ taskId: "task-1", enabled: true });
    expect(mocks.fetchTaskEnvironmentLive).toHaveBeenCalledTimes(1);

    await act(async () => {
      initialRead.resolve(environmentResponse("task-1"));
      await initialRead.promise;
    });
    await waitFor(() => expect(hook.result.current.status?.tone).toBe("running"));

    hook.rerender({ taskId: "task-2", enabled: true });
    await waitFor(() => expect(mocks.fetchTaskEnvironmentLive).toHaveBeenCalledTimes(2));
    await waitFor(() => expect(hook.result.current.status?.label).toBe("ready"));
    hook.unmount();
  });
});
