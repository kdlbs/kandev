import { describe, expect, it } from "vitest";
import { createAppStore } from "@/lib/state/store";
import { registerTasksHandlers } from "./tasks";

type PendingAction = "clarification" | "permission";

function pendingStore(taskPendingAction?: PendingAction) {
  const task = {
    id: "t1",
    workflowId: "wf1",
    workflowStepId: "step1",
    title: "Task",
    position: 0,
    taskPendingAction,
  };
  return createAppStore({
    kanban: { workflowId: "wf1", steps: [], tasks: [task] },
    kanbanMulti: {
      isLoading: false,
      snapshots: {
        wf1: { workflowId: "wf1", workflowName: "Workflow", steps: [], tasks: [task] },
      },
    },
  });
}

function taskUpdatedMessage(taskPendingAction: PendingAction | null | undefined) {
  const payload = {
    task_id: "t1",
    workflow_id: "wf1",
    workflow_step_id: "step1",
    title: "Task",
    state: "IN_PROGRESS" as const,
    is_ephemeral: false,
    ...(taskPendingAction !== undefined ? { task_pending_action: taskPendingAction } : {}),
  };
  return {
    id: "message-1",
    type: "notification" as const,
    action: "task.updated" as const,
    payload,
  };
}

function taskPendingActions(store: ReturnType<typeof pendingStore>) {
  return {
    main: store.getState().kanban.tasks[0]?.taskPendingAction,
    multi: store.getState().kanbanMulti.snapshots.wf1.tasks[0]?.taskPendingAction,
  };
}

describe("task.updated task-wide pending action", () => {
  it("applies permission to main and multi-workflow snapshots", () => {
    const store = pendingStore();
    registerTasksHandlers(store)["task.updated"]!(taskUpdatedMessage("permission"));
    expect(taskPendingActions(store)).toEqual({ main: "permission", multi: "permission" });
  });

  it("clears both snapshots when the payload carries explicit null", () => {
    const store = pendingStore("clarification");
    registerTasksHandlers(store)["task.updated"]!(taskUpdatedMessage(null));
    expect(taskPendingActions(store)).toEqual({ main: null, multi: null });
  });

  it("preserves both snapshots when the payload omits the field", () => {
    const store = pendingStore("permission");
    registerTasksHandlers(store)["task.updated"]!(taskUpdatedMessage(undefined));
    expect(taskPendingActions(store)).toEqual({ main: "permission", multi: "permission" });
  });
});
