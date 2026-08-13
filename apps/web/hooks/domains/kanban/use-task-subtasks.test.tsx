import { describe, expect, it } from "vitest";
import { renderHook } from "@testing-library/react";
import { StateProvider } from "@/components/state-provider";
import type { KanbanState } from "@/lib/state/slices/kanban/types";
import { useTaskSubtasks } from "./use-task-subtasks";

function makeTask(overrides: Partial<KanbanState["tasks"][number]>): KanbanState["tasks"][number] {
  return {
    id: "id",
    workflowId: "wf-1",
    workflowStepId: "step-1",
    title: "Task",
    position: 0,
    ...overrides,
  };
}

function renderWithState(initialState: Parameters<typeof StateProvider>[0]["initialState"]) {
  return renderHook(() => useTaskSubtasks("parent-1"), {
    wrapper: ({ children }) => (
      <StateProvider initialState={initialState}>{children}</StateProvider>
    ),
  });
}

describe("useTaskSubtasks", () => {
  it("returns only direct children of the given task", () => {
    const { result } = renderWithState({
      kanban: {
        workflowId: "wf-1",
        steps: [],
        tasks: [
          makeTask({ id: "parent-1", title: "Parent" }),
          makeTask({ id: "child-1", title: "Child 1", parentTaskId: "parent-1", position: 1 }),
          makeTask({ id: "child-2", title: "Child 2", parentTaskId: "parent-1", position: 2 }),
          makeTask({ id: "other", title: "Unrelated", parentTaskId: "other-parent" }),
        ],
      },
    });
    expect(result.current.map((t) => t.id)).toEqual(["child-1", "child-2"]);
  });

  it("excludes grandchildren", () => {
    const { result } = renderWithState({
      kanban: {
        workflowId: "wf-1",
        steps: [],
        tasks: [
          makeTask({ id: "parent-1", title: "Parent" }),
          makeTask({ id: "child-1", title: "Child", parentTaskId: "parent-1", position: 1 }),
          makeTask({
            id: "grandchild-1",
            title: "Grandchild",
            parentTaskId: "child-1",
            position: 1,
          }),
        ],
      },
    });
    expect(result.current.map((t) => t.id)).toEqual(["child-1"]);
  });

  it("includes cross-workflow children from kanbanMulti snapshots, de-duplicated against the active board", () => {
    const { result } = renderWithState({
      kanban: {
        workflowId: "wf-1",
        steps: [],
        tasks: [
          makeTask({ id: "parent-1", title: "Parent" }),
          makeTask({ id: "child-1", title: "Child 1", parentTaskId: "parent-1", position: 1 }),
        ],
      },
      kanbanMulti: {
        isLoading: false,
        snapshots: {
          "wf-2": {
            workflowId: "wf-2",
            workflowName: "Other workflow",
            steps: [],
            tasks: [
              // Duplicate of the active-board child-1: must not appear twice.
              makeTask({ id: "child-1", title: "Child 1", parentTaskId: "parent-1", position: 1 }),
              makeTask({
                id: "child-3",
                title: "Cross-workflow child",
                parentTaskId: "parent-1",
                position: 2,
              }),
            ],
          },
        },
      },
    });
    expect(result.current.map((t) => t.id)).toEqual(["child-1", "child-3"]);
  });
});

describe("useTaskSubtasks — ordering and stability", () => {
  it("orders by position then createdAt", () => {
    const { result } = renderWithState({
      kanban: {
        workflowId: "wf-1",
        steps: [],
        tasks: [
          makeTask({ id: "parent-1", title: "Parent" }),
          makeTask({
            id: "b",
            title: "B",
            parentTaskId: "parent-1",
            position: 1,
            createdAt: "2026-01-02T00:00:00Z",
          }),
          makeTask({
            id: "a",
            title: "A",
            parentTaskId: "parent-1",
            position: 1,
            createdAt: "2026-01-01T00:00:00Z",
          }),
        ],
      },
    });
    expect(result.current.map((t) => t.id)).toEqual(["a", "b"]);
  });

  it("returns a stable empty reference across renders when there are no subtasks", () => {
    const { result, rerender } = renderWithState({
      kanban: {
        workflowId: "wf-1",
        steps: [],
        tasks: [makeTask({ id: "parent-1", title: "Parent" })],
      },
    });
    const first = result.current;
    rerender();
    expect(result.current).toEqual([]);
    expect(result.current).toBe(first);
  });
});
