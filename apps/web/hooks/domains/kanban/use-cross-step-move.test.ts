import { act, renderHook } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { createAppStore } from "@/lib/state/store";
import type { Task } from "@/components/kanban-card";

const mockMoveTaskById = vi.fn();
vi.mock("@/hooks/use-task-actions", () => ({
  useTaskActions: () => ({ moveTaskById: mockMoveTaskById }),
}));

const WORKFLOW_ID = "wf1";
const SOURCE_STEP = "step-source";
const TARGET_STEP = "step-target";
let store = createAppStore();

vi.mock("@/components/state-provider", () => ({
  useAppStoreApi: () => store,
}));

import { useCrossStepMove } from "./use-swimlane-kanban-dnd";

beforeEach(() => {
  store = createAppStore({
    kanbanMulti: {
      snapshots: {
        [WORKFLOW_ID]: {
          workflowId: WORKFLOW_ID,
          workflowName: "Workflow",
          steps: [],
          tasks: [
            {
              id: "moving",
              workflowId: WORKFLOW_ID,
              workflowStepId: SOURCE_STEP,
              title: "Moving",
              position: 0,
            },
            {
              id: "occupant",
              workflowId: WORKFLOW_ID,
              workflowStepId: TARGET_STEP,
              title: "Occupant",
              position: 0,
            },
          ],
        },
      },
      isLoading: false,
      orderRevisionByStepId: {},
      pendingReorderBandKeys: {},
      withheldReorderByBandKey: {},
    },
  });
  mockMoveTaskById.mockReset();
});

describe("useCrossStepMove", () => {
  it("moves the card to the target step without a client-computed position (server always assigns arrival position)", async () => {
    mockMoveTaskById.mockResolvedValue({});
    const { result } = renderHook(() => useCrossStepMove(WORKFLOW_ID));
    const task = { id: "moving", workflowStepId: SOURCE_STEP } as Task;

    await act(async () => {
      await result.current("moving", TARGET_STEP, task);
    });

    expect(mockMoveTaskById).toHaveBeenCalledWith("moving", {
      workflow_id: WORKFLOW_ID,
      workflow_step_id: TARGET_STEP,
    });
    expect(mockMoveTaskById.mock.calls[0]?.[1]).not.toHaveProperty("position");
    const tasks = store.getState().kanbanMulti.snapshots[WORKFLOW_ID].tasks;
    expect(tasks.find((t) => t.id === "moving")?.workflowStepId).toBe(TARGET_STEP);
  });

  it("restores the owned original stage on failure", async () => {
    mockMoveTaskById.mockRejectedValue(new Error("move failed"));
    const onMoveError = vi.fn();
    const { result } = renderHook(() => useCrossStepMove(WORKFLOW_ID, onMoveError));
    const task = { id: "moving", workflowStepId: SOURCE_STEP } as Task;

    await act(async () => {
      await result.current("moving", TARGET_STEP, task);
    });

    const tasks = store.getState().kanbanMulti.snapshots[WORKFLOW_ID].tasks;
    expect(tasks.find((t) => t.id === "moving")?.workflowStepId).toBe(SOURCE_STEP);
    expect(onMoveError).toHaveBeenCalledWith(expect.objectContaining({ taskId: "moving" }));
  });
});
