import { renderHook } from "@testing-library/react";
import { beforeEach, expect, it, vi } from "vitest";
import { createAppStore } from "@/lib/state/store";
import { useTaskRemoval } from "./use-task-removal";

const api = vi.hoisted(() => ({
  fetchTask: vi.fn(),
  replaceTaskUrl: vi.fn(),
  setActiveSession: vi.fn(),
}));
vi.mock("@/lib/api", () => ({ fetchTask: api.fetchTask, listTaskSessions: vi.fn() }));
vi.mock("@/lib/links", () => ({
  linkToTaskOverview: () => "/?home=overview",
  replaceTaskUrl: api.replaceTaskUrl,
}));

beforeEach(() => vi.clearAllMocks());
const PARENT_TASK_ID = "task-parent";

// @covers AC-TASKS-THREADS-ACTIONS-002.4
it.each(["task-A", null])(
  "stay-on-listing prunes snapshots without navigation after active %s",
  async (activeTaskId) => {
    const tasks = [PARENT_TASK_ID, "task-child", "task-next"].map((id) => ({
      id,
      title: id,
      workflowId: "workflow",
      workflowStepId: "step",
      position: 0,
      parentTaskId: id === "task-child" ? PARENT_TASK_ID : null,
    }));
    const store = createAppStore();
    store.setState((state) => ({
      tasks: { ...state.tasks, activeTaskId },
      kanban: { ...state.kanban, tasks },
      kanbanMulti: {
        ...state.kanbanMulti,
        snapshots: { workflow: { ...state.kanban, tasks } },
      },
      setActiveSession: api.setActiveSession,
    }));
    const { result } = renderHook(() => useTaskRemoval({ store, stayOnListing: true }));

    await result.current.removeTaskFromBoard(PARENT_TASK_ID, {
      wasActiveTaskId: PARENT_TASK_ID,
      excludeTaskTree: true,
    });

    expect(store.getState().kanbanMulti.snapshots.workflow.tasks).toEqual([tasks[2]]);
    expect(store.getState().kanban.tasks).toEqual([tasks[2]]);
    expect(api.fetchTask).not.toHaveBeenCalled();
    expect(api.replaceTaskUrl).not.toHaveBeenCalled();
    expect(api.setActiveSession).not.toHaveBeenCalled();
  },
);
