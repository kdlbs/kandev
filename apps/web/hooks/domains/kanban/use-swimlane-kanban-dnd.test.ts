import { act, renderHook } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { DragEndEvent, DragStartEvent } from "@dnd-kit/core";
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

import { useCrossStepMove, useSwimlaneKanbanDnd } from "./use-swimlane-kanban-dnd";

const WORKFLOW_ID = "wf1";

function makeTask(id: string, stepId: string): Task {
  return { id, workflowStepId: stepId, title: id } as Task;
}

beforeEach(() => {
  reorderBand.mockReset();
  mockMoveTaskById.mockReset();
  storeState.setWorkflowSnapshot.mockReset();
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

  it("clears the active task and issues no reorder or move request when the pointer is released outside every droppable (over: null)", async () => {
    // This is the real AC.9 drop-outside-any-band path: dnd-kit reports
    // `over: null` on DragEndEvent (not DragCancelEvent/Escape) when the
    // pointer is released over empty space. handleDragEnd's own `if (!over)
    // return` is what implements the no-op — drop-classification.test.ts's
    // "overId: null" case never actually runs in production, since
    // classifyDrop is called after that guard.
    const tasks = [makeTask("a", "step-1"), makeTask("b", "step-1")];
    const { result } = renderHook(() =>
      // eslint-disable-next-line @typescript-eslint/no-explicit-any -- test-only cast: exercises the non-presentation hook directly
      (useSwimlaneKanbanDnd as any)({ tasks, workflowId: WORKFLOW_ID }),
    );

    act(() => {
      result.current.handleDragStart({ active: { id: "a" } } as DragStartEvent);
    });
    expect(result.current.activeTask?.id).toBe("a");

    await act(async () => {
      await result.current.handleDragEnd({
        active: { id: "a" },
        over: null,
      } as DragEndEvent);
    });

    expect(result.current.activeTask).toBeNull();
    expect(reorderBand).not.toHaveBeenCalled();
    expect(mockMoveTaskById).not.toHaveBeenCalled();
    expect(storeState.setWorkflowSnapshot).not.toHaveBeenCalled();
  });
});

describe("useCrossStepMove — rollback on failure", () => {
  it("reverts only the moved task's step, preserving a concurrent update to another task", async () => {
    const taskA = makeTask("a", "step-1");
    const taskB = makeTask("b", "step-1");
    storeState.kanbanMulti.snapshots[WORKFLOW_ID] = { tasks: [taskA, taskB] };

    let rejectMove!: (error: unknown) => void;
    mockMoveTaskById.mockReturnValue(
      new Promise((_resolve, reject) => {
        rejectMove = reject;
      }),
    );

    const { result } = renderHook(() => useCrossStepMove(WORKFLOW_ID));

    let pending!: Promise<void>;
    act(() => {
      pending = result.current("a", "step-2", taskA);
    });

    // A concurrent, unrelated update lands on task b while the move is
    // still in flight — the rollback must not discard it.
    storeState.kanbanMulti.snapshots[WORKFLOW_ID] = {
      tasks: [
        storeState.setWorkflowSnapshot.mock.calls.at(-1)?.[1].tasks.find((t: Task) => t.id === "a"),
        { ...taskB, title: "b (concurrently renamed)" },
      ],
    };

    await act(async () => {
      rejectMove(new Error("network error"));
      await pending;
    });

    const lastCall = storeState.setWorkflowSnapshot.mock.calls.at(-1);
    const finalTasks = lastCall?.[1].tasks as Task[];
    expect(finalTasks.find((t) => t.id === "a")?.workflowStepId).toBe("step-1");
    expect(finalTasks.find((t) => t.id === "b")?.title).toBe("b (concurrently renamed)");
  });
});
