import { act, renderHook } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { DragStartEvent } from "@dnd-kit/core";
import type { Task } from "@/components/kanban-card";

const reorderBand = vi.fn();
vi.mock("@/hooks/domains/kanban/use-step-reorder", () => ({
  useStepReorder: () => ({ reorderBand, isBandPending: () => false }),
}));

const mockMoveTaskById = vi.fn();
vi.mock("@/hooks/use-task-actions", () => ({
  useTaskActions: () => ({ moveTaskById: mockMoveTaskById }),
}));

const storeState = {
  kanbanMulti: { snapshots: {} as Record<string, { tasks: unknown[] }> },
  setWorkflowSnapshot: vi.fn(),
};
const store = { getState: () => storeState };
vi.mock("@/components/state-provider", () => ({
  useAppStoreApi: () => store,
}));

import { useSwimlaneKanbanDnd } from "./use-swimlane-kanban-dnd";

const WORKFLOW_ID = "wf1";

function makeTask(id: string, stepId: string): Task {
  return { id, workflowStepId: stepId, title: id } as Task;
}

beforeEach(() => {
  reorderBand.mockReset();
  mockMoveTaskById.mockReset();
});

describe("useSwimlaneKanbanDnd — Escape/drop-outside cancellation (AC.9)", () => {
  it("clears the active task and issues no reorder or move request on handleDragCancel", () => {
    const tasks = [makeTask("a", "step-1"), makeTask("b", "step-1")];
    const { result } = renderHook(() =>
      // eslint-disable-next-line @typescript-eslint/no-explicit-any -- test-only cast: exercises the non-presentation hook directly
      (useSwimlaneKanbanDnd as any)({ tasks, workflowId: WORKFLOW_ID }),
    );

    act(() => {
      result.current.handleDragStart({ active: { id: "a" } } as DragStartEvent);
    });
    expect(result.current.activeTask?.id).toBe("a");

    act(() => {
      result.current.handleDragCancel();
    });

    expect(result.current.activeTask).toBeNull();
    expect(reorderBand).not.toHaveBeenCalled();
    expect(mockMoveTaskById).not.toHaveBeenCalled();
  });
});
