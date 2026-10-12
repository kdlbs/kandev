import { act, renderHook } from "@testing-library/react";
import { beforeEach, expect, it, vi } from "vitest";
import { useTaskMoveOptions } from "./task-move-context-menu";

const move = vi.hoisted(() => vi.fn());
vi.mock("@/hooks/domains/kanban/use-workflow-move", () => ({
  useWorkflowMove: () => ({ move, isMoving: false }),
}));
vi.mock("@/components/toast-provider", () => ({ useToast: () => ({ toast: vi.fn() }) }));
beforeEach(() => {
  move.mockReset().mockResolvedValue({ disposition: "committed" });
});

it("submits options to the chosen workflow instead of the task's current workflow", async () => {
  const { result } = renderHook(() => useTaskMoveOptions({ taskId: "task", workflowId: "source" }));
  act(() =>
    result.current.openMoveOptionsStep({ id: "review", title: "Review", workflow_id: "target" }),
  );
  expect(move).not.toHaveBeenCalled();
  await act(async () => {
    await result.current.submitMoveOptions({ instructions: "Check keyboard flow" });
  });
  expect(move).toHaveBeenCalledWith("task", {
    workflow_id: "target",
    workflow_step_id: "review",
    position: 0,
    entry_options: { instructions: "Check keyboard flow" },
  });
  expect(result.current.moveOptionsStep).toBeNull();
});

it("moves directly without opening options and retains options after a rejected move", async () => {
  const { result } = renderHook(() => useTaskMoveOptions({ taskId: "task", workflowId: "source" }));
  await act(async () => {
    await result.current.moveImmediately({ id: "review", title: "Review", workflow_id: "target" });
  });
  expect(move).toHaveBeenCalledWith(
    "task",
    expect.objectContaining({ workflow_id: "target", entry_options: undefined }),
  );
  expect(result.current.moveOptionsStep).toBeNull();
  move.mockResolvedValue({ disposition: "failed", error: new Error("Rejected") });
  act(() =>
    result.current.openMoveOptionsStep({ id: "review", title: "Review", workflow_id: "target" }),
  );
  await act(async () => {
    expect(await result.current.submitMoveOptions(undefined)).toBe(false);
  });
  expect(result.current.moveOptionsStep?.id).toBe("review");
});
