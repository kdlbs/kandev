import { describe, expect, it } from "vitest";
import { registerTasksHandlers } from "./tasks";
import { makeStore } from "./tasks.test-helpers";

const TASK_ID = "task-1";
const WORKFLOW_ID = "workflow-1";
const STEP_ID = "step-1";
const UPDATED_AT = "2026-08-01T18:00:00Z";

function summaryMessage(summary: Record<string, unknown>) {
  return {
    id: "summary-message",
    type: "notification" as const,
    action: "task.status_summary.updated" as const,
    payload: {
      task_id: TASK_ID,
      workspace_id: "workspace-1",
      status_summary: summary,
    },
  } as Parameters<
    NonNullable<ReturnType<typeof registerTasksHandlers>["task.status_summary.updated"]>
  >[0];
}

describe("task.status_summary.updated cache replacement", () => {
  it("replaces the summary in both single and multi-kanban task caches", () => {
    const store = makeStore({
      kanban: {
        workflowId: WORKFLOW_ID,
        steps: [],
        tasks: [{ id: TASK_ID, workflowStepId: STEP_ID, title: "Task" }],
      },
      kanbanMulti: {
        isLoading: false,
        snapshots: {
          [WORKFLOW_ID]: {
            workflowId: WORKFLOW_ID,
            workflowName: "Workflow",
            steps: [],
            tasks: [{ id: TASK_ID, workflowStepId: STEP_ID, title: "Task" }],
          },
        },
      },
    } as never);

    registerTasksHandlers(store)["task.status_summary.updated"]!(
      summaryMessage({
        revision: 2,
        updated_at: UPDATED_AT,
        git: { additions: 4 },
      }),
    );

    expect(store.getState().kanban.tasks[0].statusSummary).toMatchObject({
      revision: 2,
      git: { additions: 4 },
    });
    expect(
      store.getState().kanbanMulti.snapshots[WORKFLOW_ID]?.tasks[0].statusSummary,
    ).toMatchObject({ revision: 2 });
  });
});

describe("task.status_summary.updated monotonicity", () => {
  it("ignores stale or same-revision replacements", () => {
    const store = makeStore({
      kanban: {
        workflowId: WORKFLOW_ID,
        steps: [],
        tasks: [
          {
            id: TASK_ID,
            workflowStepId: STEP_ID,
            title: "Task",
            statusSummary: {
              revision: 3,
              updated_at: UPDATED_AT,
              git: { additions: 7 },
            },
          },
        ],
      },
      kanbanMulti: { isLoading: false, snapshots: {} },
    } as never);
    const handler = registerTasksHandlers(store)["task.status_summary.updated"]!;

    handler(
      summaryMessage({
        revision: 2,
        updated_at: "2026-08-01T17:00:00Z",
        git: { additions: 1 },
      }),
    );
    handler(
      summaryMessage({
        revision: 3,
        updated_at: UPDATED_AT,
        git: { additions: 2 },
      }),
    );

    expect(store.getState().kanban.tasks[0].statusSummary).toMatchObject({
      revision: 3,
      git: { additions: 7 },
    });
  });
});

describe("task.updated summary preservation", () => {
  it("keeps the cached summary when a lightweight task update omits it", () => {
    const store = makeStore({
      kanban: {
        workflowId: WORKFLOW_ID,
        steps: [],
        tasks: [
          {
            id: TASK_ID,
            workflowStepId: STEP_ID,
            title: "Task",
            statusSummary: {
              revision: 4,
              updated_at: UPDATED_AT,
              pending_action: "clarification",
            },
          },
        ],
      },
      kanbanMulti: { isLoading: false, snapshots: {} },
    } as never);

    registerTasksHandlers(store)["task.updated"]!({
      id: "task-update",
      type: "notification",
      action: "task.updated",
      payload: {
        task_id: TASK_ID,
        id: TASK_ID,
        workflow_id: WORKFLOW_ID,
        title: "Renamed task",
        state: "IN_PROGRESS",
      },
    } as never);

    expect(store.getState().kanban.tasks[0].title).toBe("Renamed task");
    expect(store.getState().kanban.tasks[0].statusSummary).toMatchObject({
      revision: 4,
      pending_action: "clarification",
    });
  });
});
