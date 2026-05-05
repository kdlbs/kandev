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
const TASK_1 = "task-1";
const TASK_2 = "task-2";
const TARGET_WORKFLOW_ID = "wf-2";
const TARGET_STEP_ID = "step-2";

describe("useTaskWorkflowMove", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("dedupes and filters task ids before moving", async () => {
    mockBulkMoveSelectedTasks.mockResolvedValue({ moved_count: 2 });
    const { result } = renderHook(() => useTaskWorkflowMove());

    await act(async () => {
      await result.current([TASK_1, "", TASK_1, TASK_2], TARGET_WORKFLOW_ID, TARGET_STEP_ID);
    });

    expect(mockBulkMoveSelectedTasks).toHaveBeenCalledWith({
      task_ids: [TASK_1, TASK_2],
      target_workflow_id: TARGET_WORKFLOW_ID,
      target_step_id: TARGET_STEP_ID,
    });
  });

  it("does nothing for an empty task id list", async () => {
    const { result } = renderHook(() => useTaskWorkflowMove());

    await act(async () => {
      await result.current([""], TARGET_WORKFLOW_ID, TARGET_STEP_ID);
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
      await result.current([TASK_1], TARGET_WORKFLOW_ID, TARGET_STEP_ID);
      await result.current([TASK_1, TASK_2], TARGET_WORKFLOW_ID, TARGET_STEP_ID);
      await result.current([TASK_1], TARGET_WORKFLOW_ID, TARGET_STEP_ID);
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

    await expect(result.current([TASK_1], TARGET_WORKFLOW_ID, TARGET_STEP_ID)).rejects.toThrow(
      error,
    );

    expect(mockToast).toHaveBeenCalledWith({
      title: "Failed to move task",
      description: "cannot move running task",
      variant: "error",
    });
  });
});
