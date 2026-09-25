import { describe, expect, it } from "vitest";
import { registerTasksHandlers } from "./tasks";
import { makeMessage, makeStore } from "./tasks.test-helpers";

const TASK_ID = "office-task";
const WORKFLOW_ID = "office-workflow";
const STEP_ID = "office-step";

function cachedOfficeTask() {
  return {
    id: TASK_ID,
    title: "Office task",
    workflowId: WORKFLOW_ID,
    workflowStepId: STEP_ID,
    position: 0,
    isFromOffice: true,
  };
}

function storeWithCachedOfficeTask() {
  return makeStore({
    kanban: {
      workflowId: WORKFLOW_ID,
      steps: [],
      tasks: [cachedOfficeTask()],
    },
    kanbanMulti: {
      isLoading: false,
      snapshots: {
        [WORKFLOW_ID]: {
          workflowId: WORKFLOW_ID,
          workflowName: "Office",
          steps: [],
          tasks: [cachedOfficeTask()],
        },
      },
    },
  } as never);
}

describe("task.updated Office identity preservation", () => {
  // @covers AC-TASKS-SUBTASK-REPARENTING-DRAG-DROP-001.4
  it("preserves Office identity when a lightweight update omits it", () => {
    const store = storeWithCachedOfficeTask();

    registerTasksHandlers(store)["task.updated"]!(
      makeMessage({
        task_id: TASK_ID,
        workflow_id: WORKFLOW_ID,
        workflow_step_id: STEP_ID,
        title: "Renamed Office task",
        state: "IN_PROGRESS",
      }),
    );

    expect(store.getState().kanban.tasks[0]?.isFromOffice).toBe(true);
    expect(store.getState().kanbanMulti.snapshots[WORKFLOW_ID]?.tasks[0]?.isFromOffice).toBe(true);
  });

  it("clears Office identity when task.updated explicitly sets it to false", () => {
    const store = storeWithCachedOfficeTask();

    registerTasksHandlers(store)["task.updated"]!(
      makeMessage({
        task_id: TASK_ID,
        workflow_id: WORKFLOW_ID,
        workflow_step_id: STEP_ID,
        title: "Project removed",
        state: "IN_PROGRESS",
        is_from_office: false,
      }),
    );

    expect(store.getState().kanban.tasks[0]?.isFromOffice).toBe(false);
    expect(store.getState().kanbanMulti.snapshots[WORKFLOW_ID]?.tasks[0]?.isFromOffice).toBe(false);
  });
});
