import { renderHook } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const moveTasks = vi.fn();
const sortByDisplayOrder = vi.fn();
const getWorkflowIdForTask = vi.fn();
const eligibleSelectedIds = vi.fn();

vi.mock("@/hooks/use-task-workflow-move", () => ({ useTaskWorkflowMove: () => moveTasks }));
vi.mock("@/hooks/use-task-multi-select", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@/hooks/use-task-multi-select")>();
  return {
    ...actual,
    useTaskMultiSelectStore: () => ({
      sortByDisplayOrder,
      getWorkflowIdForTask,
      eligibleSelectedIds,
    }),
  };
});
vi.mock("@/components/kanban-card-menu-items", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@/components/kanban-card-menu-items")>();
  return {
    ...actual,
    useKanbanCardMoveTargets: () => ({
      currentWorkflowId: "wf1",
      workflowItems: [],
      stepsByWorkflowId: {},
    }),
  };
});

import { useKanbanCardMoveMenuActions, type Task } from "./kanban-card";

const task = { id: "t1", title: "T", workflowStepId: "s1" } as Task;

beforeEach(() => {
  moveTasks.mockReset().mockResolvedValue(undefined);
  sortByDisplayOrder.mockReset().mockImplementation((ids: string[]) => ids);
  getWorkflowIdForTask.mockReset().mockReturnValue("wf1");
  eligibleSelectedIds.mockReset().mockImplementation((ids: string[]) => ids);
});
afterEach(() => {
  vi.clearAllMocks();
});

describe("useKanbanCardMoveMenuActions — priority-filter eligibility", () => {
  it("filters a hidden selected task out of the move-selection ids", () => {
    eligibleSelectedIds.mockImplementation((ids: string[]) => ids.filter((id) => id !== "hidden"));
    const selectedIds = new Set(["t1", "hidden"]);
    const { result } = renderHook(() =>
      useKanbanCardMoveMenuActions({
        task,
        steps: [],
        isSelected: true,
        selectedIds,
        onMove: undefined,
      }),
    );

    result.current.sendSelectionToWorkflow("wf2", "s2");

    expect(eligibleSelectedIds).toHaveBeenCalledWith(["t1", "hidden"]);
    expect(moveTasks).toHaveBeenCalledWith(["t1"], "wf2", "s2", "workflow");
  });

  it("moves the whole selection when nothing is filtered out", () => {
    const selectedIds = new Set(["t1", "t2"]);
    const { result } = renderHook(() =>
      useKanbanCardMoveMenuActions({
        task,
        steps: [],
        isSelected: true,
        selectedIds,
        onMove: undefined,
      }),
    );

    result.current.sendSelectionToWorkflow("wf2", "s2");

    expect(moveTasks).toHaveBeenCalledWith(["t1", "t2"], "wf2", "s2", "workflow");
  });

  it("falls back to just this card's task when not part of the selection", () => {
    const { result } = renderHook(() =>
      useKanbanCardMoveMenuActions({
        task,
        steps: [],
        isSelected: false,
        selectedIds: new Set(["other"]),
        onMove: undefined,
      }),
    );

    result.current.sendSelectionToWorkflow("wf2", "s2");

    expect(eligibleSelectedIds).not.toHaveBeenCalled();
    expect(moveTasks).toHaveBeenCalledWith(["t1"], "wf2", "s2", "workflow");
  });
});
