import { act, renderHook, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { TaskMRAutomationOptions } from "@/lib/types/gitlab";

const api = vi.hoisted(() => ({
  getTaskMRAutomation: vi.fn(),
  updateTaskMRAutomation: vi.fn(),
}));

vi.mock("@/lib/api/domains/gitlab-api", () => api);

import { useTaskMRAutomationOptions } from "./use-task-mr-automation";

function baseOptions(overrides: Partial<TaskMRAutomationOptions> = {}): TaskMRAutomationOptions {
  return {
    task_id: "task-1",
    prompt_on_review_requested: false,
    prompt_on_merged: false,
    prompt_on_closed: false,
    review_reviewer_username: "",
    updated_at: "2026-01-01T00:00:00Z",
    mr_states: [],
    ...overrides,
  };
}

function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((res) => {
    resolve = res;
  });
  return { promise, resolve };
}

describe("useTaskMRAutomationOptions fetching", () => {
  beforeEach(() => vi.clearAllMocks());

  it("fetches options on mount for a given taskId", async () => {
    api.getTaskMRAutomation.mockResolvedValue(baseOptions());
    const { result } = renderHook(() => useTaskMRAutomationOptions("task-1"));

    await waitFor(() => expect(result.current.options).not.toBeNull());
    expect(api.getTaskMRAutomation).toHaveBeenCalledWith("task-1", { cache: "no-store" });
    expect(result.current.options?.task_id).toBe("task-1");
    expect(result.current.error).toBeNull();
  });

  it("does not fetch when taskId is null", () => {
    renderHook(() => useTaskMRAutomationOptions(null));
    expect(api.getTaskMRAutomation).not.toHaveBeenCalled();
  });
});

describe("useTaskMRAutomationOptions optimistic updates", () => {
  beforeEach(() => vi.clearAllMocks());

  it("applies an optimistic update immediately, then reconciles with the server response", async () => {
    api.getTaskMRAutomation.mockResolvedValue(baseOptions());
    const update = deferred<TaskMRAutomationOptions>();
    api.updateTaskMRAutomation.mockImplementation(() => update.promise);
    const { result } = renderHook(() => useTaskMRAutomationOptions("task-1"));
    await waitFor(() => expect(result.current.options).not.toBeNull());

    act(() => {
      void result.current.update({ prompt_on_merged: true });
    });

    // Optimistic reflect happens synchronously within the update call.
    await waitFor(() => expect(result.current.options?.prompt_on_merged).toBe(true));
    expect(result.current.saving).toBe(true);

    await act(async () => {
      update.resolve(baseOptions({ prompt_on_merged: true }));
    });

    await waitFor(() => expect(result.current.saving).toBe(false));
    expect(result.current.options?.prompt_on_merged).toBe(true);
    expect(result.current.error).toBeNull();
  });

  it("reverts the optimistic update and surfaces an error on failure (AC27)", async () => {
    api.getTaskMRAutomation.mockResolvedValue(baseOptions());
    api.updateTaskMRAutomation.mockRejectedValue(new Error("network down"));
    const { result } = renderHook(() => useTaskMRAutomationOptions("task-1"));
    await waitFor(() => expect(result.current.options).not.toBeNull());

    await act(async () => {
      await expect(result.current.update({ prompt_on_closed: true })).rejects.toThrow(
        "network down",
      );
    });

    expect(result.current.options?.prompt_on_closed).toBe(false);
    expect(result.current.error).toBe("network down");
    expect(result.current.saving).toBe(false);
  });
});

describe("useTaskMRAutomationOptions races", () => {
  beforeEach(() => vi.clearAllMocks());

  it("does not let a concurrent refresh discard an update's response (independent generations)", async () => {
    api.getTaskMRAutomation.mockResolvedValue(baseOptions());
    const update = deferred<TaskMRAutomationOptions>();
    api.updateTaskMRAutomation.mockImplementation(() => update.promise);
    const { result } = renderHook(() => useTaskMRAutomationOptions("task-1"));
    await waitFor(() => expect(result.current.options).not.toBeNull());

    // Start an update (PATCH in flight)...
    act(() => {
      void result.current.update({ prompt_on_merged: true });
    });
    await waitFor(() => expect(result.current.saving).toBe(true));

    // ...then a refresh races in and resolves first, with a pre-patch
    // snapshot (server hasn't processed the PATCH yet).
    const refresh = deferred<TaskMRAutomationOptions>();
    api.getTaskMRAutomation.mockImplementation(() => refresh.promise);
    act(() => {
      void result.current.refresh();
    });
    await act(async () => {
      refresh.resolve(baseOptions({ prompt_on_merged: false }));
    });

    // The update's response must still land — a refresh racing in must not
    // permanently suppress it.
    await act(async () => {
      update.resolve(baseOptions({ prompt_on_merged: true }));
    });
    await waitFor(() => expect(result.current.saving).toBe(false));
    expect(result.current.options?.prompt_on_merged).toBe(true);
  });

  it("does not let a refresh started before a save overwrite the saved result", async () => {
    api.getTaskMRAutomation.mockResolvedValue(baseOptions());
    const { result } = renderHook(() => useTaskMRAutomationOptions("task-1"));
    await waitFor(() => expect(result.current.options).not.toBeNull());

    // A manual refresh starts and stays in flight...
    const refresh = deferred<TaskMRAutomationOptions>();
    api.getTaskMRAutomation.mockReturnValueOnce(refresh.promise);
    act(() => {
      void result.current.refresh();
    });

    // ...while a save starts and completes first.
    api.updateTaskMRAutomation.mockResolvedValue(baseOptions({ prompt_on_merged: true }));
    await act(async () => {
      await result.current.update({ prompt_on_merged: true });
    });
    expect(result.current.options?.prompt_on_merged).toBe(true);

    // The stale refresh (started before the save) now resolves with
    // pre-save data — it must not flip the saved switch back off.
    await act(async () => {
      refresh.resolve(baseOptions({ prompt_on_merged: false }));
    });
    expect(result.current.options?.prompt_on_merged).toBe(true);
  });
});

describe("useTaskMRAutomationOptions task switching", () => {
  beforeEach(() => vi.clearAllMocks());

  it("does not leak a stale task's response after switching taskId", async () => {
    const taskA = deferred<TaskMRAutomationOptions>();
    api.getTaskMRAutomation.mockImplementation((taskId: string) =>
      taskId === "task-a" ? taskA.promise : Promise.resolve(baseOptions({ task_id: "task-b" })),
    );

    const { result, rerender } = renderHook(
      ({ taskId }: { taskId: string | null }) => useTaskMRAutomationOptions(taskId),
      { initialProps: { taskId: "task-a" } },
    );

    // task-a's fetch is still in flight when the caller switches to task-b.
    rerender({ taskId: "task-b" });
    await waitFor(() => expect(result.current.options?.task_id).toBe("task-b"));

    // task-a's stale response resolves after the switch — it must not
    // overwrite task-b's already-displayed options.
    await act(async () => {
      taskA.resolve(baseOptions({ task_id: "task-a" }));
    });
    expect(result.current.options?.task_id).toBe("task-b");
  });

  it("reloads options when returning to a task whose intermediate switch never resolved", async () => {
    api.getTaskMRAutomation.mockResolvedValueOnce(baseOptions({ task_id: "task-a" }));
    const { result, rerender } = renderHook(
      ({ taskId }: { taskId: string | null }) => useTaskMRAutomationOptions(taskId),
      { initialProps: { taskId: "task-a" } },
    );
    await waitFor(() => expect(result.current.options?.task_id).toBe("task-a"));

    // Switch to task-b, but its fetch never resolves before switching back
    // — so nothing ever marks task-b (or re-marks task-a) as loaded.
    const taskB = deferred<TaskMRAutomationOptions>();
    api.getTaskMRAutomation.mockReturnValueOnce(taskB.promise);
    rerender({ taskId: "task-b" });
    await waitFor(() => expect(result.current.options).toBeNull());

    // Switch back to task-a before task-b's fetch resolves. This must
    // re-fetch task-a, not leave options stuck at null.
    api.getTaskMRAutomation.mockResolvedValueOnce(baseOptions({ task_id: "task-a" }));
    rerender({ taskId: "task-a" });
    await waitFor(() => expect(result.current.options?.task_id).toBe("task-a"));
  });
});
