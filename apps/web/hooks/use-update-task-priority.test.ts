import { act, renderHook } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { useUpdateTaskPriority } from "@/hooks/use-update-task-priority";
import { updateTask } from "@/lib/api/domains/kanban-api";

const mockToast = vi.fn();

vi.mock("@/components/toast-provider", () => ({
  useToast: () => ({ toast: mockToast }),
}));

vi.mock("@/lib/api/domains/kanban-api", () => ({
  updateTask: vi.fn(),
}));

const mockUpdateTask = vi.mocked(updateTask);
const TASK_ID = "task-1";

describe("useUpdateTaskPriority", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("submits only the priority field", async () => {
    mockUpdateTask.mockResolvedValue({ id: TASK_ID, priority: "critical" } as never);
    const { result } = renderHook(() => useUpdateTaskPriority());

    await act(async () => {
      await result.current(TASK_ID, "critical");
    });

    expect(mockUpdateTask).toHaveBeenCalledWith(TASK_ID, { priority: "critical" });
    expect(mockToast).not.toHaveBeenCalled();
  });

  it("reselecting the same priority completes without error (AC-003.5)", async () => {
    mockUpdateTask.mockResolvedValue({ id: TASK_ID, priority: "critical" } as never);
    const { result } = renderHook(() => useUpdateTaskPriority());

    await act(async () => {
      await result.current(TASK_ID, "critical");
      await result.current(TASK_ID, "critical");
    });

    expect(mockUpdateTask).toHaveBeenCalledTimes(2);
    expect(mockToast).not.toHaveBeenCalled();
  });

  it("surfaces a failure to the user without throwing (AC-003.7)", async () => {
    const error = new Error("network down");
    mockUpdateTask.mockRejectedValue(error);
    const { result } = renderHook(() => useUpdateTaskPriority());

    await act(async () => {
      await result.current(TASK_ID, "critical");
    });

    expect(mockToast).toHaveBeenCalledWith({
      title: "Failed to update task",
      description: "network down",
      variant: "error",
    });
  });
});
