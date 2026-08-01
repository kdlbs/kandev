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

describe("useTaskMRAutomationOptions", () => {
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

  it("applies an optimistic update immediately, then reconciles with the server response", async () => {
    api.getTaskMRAutomation.mockResolvedValue(baseOptions());
    let resolveUpdate!: (value: TaskMRAutomationOptions) => void;
    api.updateTaskMRAutomation.mockImplementation(
      () =>
        new Promise<TaskMRAutomationOptions>((resolve) => {
          resolveUpdate = resolve;
        }),
    );
    const { result } = renderHook(() => useTaskMRAutomationOptions("task-1"));
    await waitFor(() => expect(result.current.options).not.toBeNull());

    act(() => {
      void result.current.update({ prompt_on_merged: true });
    });

    // Optimistic reflect happens synchronously within the update call.
    await waitFor(() => expect(result.current.options?.prompt_on_merged).toBe(true));
    expect(result.current.saving).toBe(true);

    await act(async () => {
      resolveUpdate(baseOptions({ prompt_on_merged: true }));
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
