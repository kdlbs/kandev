import { act, renderHook, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import {
  moveTask,
  type MoveTaskPayload,
  type WorkflowMoveResponse,
} from "@/lib/api/domains/kanban-api";
import { useWorkflowMove } from "./use-workflow-move";

vi.mock("@/lib/api/domains/kanban-api", () => ({
  moveTask: vi.fn(),
}));

const mockMoveTask = vi.mocked(moveTask);
const destination: MoveTaskPayload = {
  workflow_id: "workflow-1",
  workflow_step_id: "step-2",
  position: 0,
};
const response = { move_id: "move-1" } as WorkflowMoveResponse;

describe("useWorkflowMove", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("returns a committed disposition and tracks the in-flight request", async () => {
    let resolveMove!: (value: WorkflowMoveResponse) => void;
    mockMoveTask.mockReturnValueOnce(
      new Promise<WorkflowMoveResponse>((resolve) => {
        resolveMove = resolve;
      }),
    );
    const { result } = renderHook(() => useWorkflowMove());

    let moveResult;
    let promise!: Promise<unknown>;
    act(() => {
      promise = result.current.move("task-1", destination);
    });
    await waitFor(() => expect(result.current.isMoving).toBe(true));
    resolveMove(response);
    await act(async () => {
      moveResult = await promise;
    });

    expect(mockMoveTask).toHaveBeenCalledWith("task-1", destination);
    expect(moveResult).toEqual({ disposition: "committed", response });
    expect(result.current.isMoving).toBe(false);
  });

  it("returns the typed error disposition and clears loading after a failure", async () => {
    const error = new Error("target is not reachable");
    mockMoveTask.mockRejectedValueOnce(error);
    const { result } = renderHook(() => useWorkflowMove());

    let moveResult;
    await act(async () => {
      moveResult = await result.current.move("task-1", destination);
    });

    expect(moveResult).toEqual({ disposition: "failed", error });
    expect(result.current.isMoving).toBe(false);
  });
});
