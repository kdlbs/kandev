import { act, renderHook } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { useTaskWorkflowMove } from "@/hooks/use-task-workflow-move";
import { bulkMoveSelectedTasks } from "@/lib/api";

const mockToast = vi.fn();

vi.mock("@/components/toast-provider", () => ({
  useToast: () => ({ toast: mockToast }),
}));

vi.mock("@/lib/api", () => ({
  bulkMoveSelectedTasks: vi.fn(),
}));

const mockBulkMoveSelectedTasks = vi.mocked(bulkMoveSelectedTasks);

describe("useTaskWorkflowMove", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("dedupes and filters task ids before moving", async () => {
    mockBulkMoveSelectedTasks.mockResolvedValue({ moved_count: 2 });
    const { result } = renderHook(() => useTaskWorkflowMove());

    await act(async () => {
      await result.current(["task-1", "", "task-1", "task-2"], "wf-2", "step-2");
    });

    expect(mockBulkMoveSelectedTasks).toHaveBeenCalledWith({
      task_ids: ["task-1", "task-2"],
      target_workflow_id: "wf-2",
      target_step_id: "step-2",
    });
  });

  it("does nothing for an empty task id list", async () => {
    const { result } = renderHook(() => useTaskWorkflowMove());

    await act(async () => {
      await result.current([""], "wf-2", "step-2");
    });

    expect(mockBulkMoveSelectedTasks).not.toHaveBeenCalled();
    expect(mockToast).not.toHaveBeenCalled();
  });

  it("shows singular, plural, and zero-count success messages", async () => {
    mockBulkMoveSelectedTasks
      .mockResolvedValueOnce({ moved_count: 1 })
      .mockResolvedValueOnce({ moved_count: 2 })
      .mockResolvedValueOnce({ moved_count: 0 });
    const { result } = renderHook(() => useTaskWorkflowMove());

    await act(async () => {
      await result.current(["task-1"], "wf-2", "step-2");
      await result.current(["task-1", "task-2"], "wf-2", "step-2");
      await result.current(["task-1"], "wf-2", "step-2");
    });

    expect(mockToast).toHaveBeenNthCalledWith(1, {
      title: "Moved task to workflow",
      description: "Switch to the destination workflow to see it.",
      variant: "success",
    });
    expect(mockToast).toHaveBeenNthCalledWith(2, {
      title: "Moved 2 tasks to workflow",
      description: "Switch to the destination workflow to see them.",
      variant: "success",
    });
    expect(mockToast).toHaveBeenNthCalledWith(3, {
      title: "Moved 0 tasks to workflow",
      description: "Switch to the destination workflow to see them.",
      variant: "success",
    });
  });

  it("shows an error toast and rethrows move failures", async () => {
    const error = new Error("cannot move running task");
    mockBulkMoveSelectedTasks.mockRejectedValue(error);
    const { result } = renderHook(() => useTaskWorkflowMove());

    await expect(result.current(["task-1"], "wf-2", "step-2")).rejects.toThrow(error);

    expect(mockToast).toHaveBeenCalledWith({
      title: "Failed to move task",
      description: "cannot move running task",
      variant: "error",
    });
  });
});
