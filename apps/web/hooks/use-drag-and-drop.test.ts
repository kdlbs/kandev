import { act, renderHook } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { KanbanState } from "@/lib/state/slices";
import type { TaskPriority } from "@/lib/types/http";

const moveTaskById = vi.fn();

vi.mock("@/hooks/use-task-actions", () => ({
  useTaskActions: () => ({ moveTaskById }),
}));
vi.mock("@/lib/i18n", () => ({ t: (key: string) => key }));
vi.mock("@/components/task/task-move-error-message", () => ({
  getTaskMoveErrorMessage: () => "error",
}));

function makeTask(
  id: string,
  workflowStepId: string,
  priority?: TaskPriority,
): KanbanState["tasks"][number] {
  return {
    id,
    workflowId: "wf1",
    workflowStepId,
    title: id,
    position: 0,
    priority,
  } as KanbanState["tasks"][number];
}

function makeStoreState(
  tasks: KanbanState["tasks"],
  kanbanSort: "created_desc" | "priority_desc",
  kanbanPriorityFilterTokens: TaskPriority[],
) {
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  let state: any = {
    kanban: { workflowId: "wf1", steps: [], tasks },
    userSettings: { kanbanSort, kanbanPriorityFilterTokens },
  };
  state.hydrate = (patch: Partial<typeof state>) => {
    state = { ...state, ...patch };
  };
  return {
    get: () => state,
    api: {
      getState: () => state,
    },
  };
}

let store: ReturnType<typeof makeStoreState>;

vi.mock("@/components/state-provider", () => ({
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  useAppStore: (selector: (state: any) => unknown) => selector(store.get()),
  useAppStoreApi: () => store.api,
}));

describe("useDragAndDrop — single-card move position is sort/filter-invariant (AC-002.7)", () => {
  beforeEach(() => {
    moveTaskById.mockReset().mockResolvedValue(undefined);
  });
  afterEach(() => {
    vi.clearAllMocks();
  });

  it("computes the same target position under created_desc and under priority_desc with an active filter", async () => {
    // Two tasks already occupy the target step; a third task moves into it.
    // A priority filter hiding "target-low" would shrink the *displayed* set
    // if calcNextPosition read the filtered view, but AC-002.7 requires it to
    // read the workflow's full task set regardless of the active sort/filter.
    const tasksAtCreatedDesc = [
      makeTask("moved", "source", "critical"),
      makeTask("target-high", "target", "high"),
      makeTask("target-low", "target", "low"),
    ];
    store = makeStoreState(tasksAtCreatedDesc, "created_desc", []);
    const { useDragAndDrop } = await import("./use-drag-and-drop");
    const { result: resultA } = renderHook(() =>
      useDragAndDrop({ visibleTasks: tasksAtCreatedDesc }),
    );

    await act(async () => {
      await resultA.current.moveTaskToStep(tasksAtCreatedDesc[0], "target");
    });

    expect(moveTaskById).toHaveBeenNthCalledWith(1, "moved", {
      workflow_id: "wf1",
      workflow_step_id: "target",
      position: 2,
    });

    // Same underlying task set, but priority_desc is active and the filter
    // would hide "target-low" from the board's display.
    const tasksAtPriorityDesc = [
      makeTask("moved", "source", "critical"),
      makeTask("target-high", "target", "high"),
      makeTask("target-low", "target", "low"),
    ];
    store = makeStoreState(tasksAtPriorityDesc, "priority_desc", ["critical", "high"]);
    const { result: resultB } = renderHook(() =>
      useDragAndDrop({ visibleTasks: tasksAtPriorityDesc.filter((t) => t.id !== "target-low") }),
    );

    await act(async () => {
      await resultB.current.moveTaskToStep(tasksAtPriorityDesc[0], "target");
    });

    expect(moveTaskById).toHaveBeenNthCalledWith(2, "moved", {
      workflow_id: "wf1",
      workflow_step_id: "target",
      position: 2,
    });
  });

  it("moveTaskToStep is a no-op when the task is already in the target step", async () => {
    const tasks = [makeTask("t1", "step-a")];
    store = makeStoreState(tasks, "created_desc", []);
    const { useDragAndDrop } = await import("./use-drag-and-drop");
    const { result } = renderHook(() => useDragAndDrop({ visibleTasks: tasks }));

    await act(async () => {
      await result.current.moveTaskToStep(tasks[0], "step-a");
    });

    expect(moveTaskById).not.toHaveBeenCalled();
  });
});
