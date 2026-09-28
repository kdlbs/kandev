import { describe, expect, it } from "vitest";
import { registerTasksHandlers } from "./tasks";
import { makeStore } from "./tasks.test-helpers";

const TASK_ID = "task-1";
const WORKFLOW_ID = "workflow-1";
const STEP_ID = "step-1";
const WORKSPACE_ID = "workspace-1";
const UPDATED_AT = "2026-08-01T18:00:00Z";

function summaryMessage(summary: Record<string, unknown>) {
  return {
    id: "summary-message",
    type: "notification" as const,
    action: "task.status_summary.updated" as const,
    payload: {
      task_id: TASK_ID,
      workspace_id: WORKSPACE_ID,
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

describe("task.status_summary.updated archived cache", () => {
  it("zeros queued_prompt_count on matching sidebarArchivedTasks rows", () => {
    const store = makeStore({
      kanban: { workflowId: WORKFLOW_ID, steps: [], tasks: [] },
      kanbanMulti: { isLoading: false, snapshots: {} },
      sidebarArchivedTasks: {
        itemsByWorkspaceId: {
          [WORKSPACE_ID]: [
            {
              id: TASK_ID,
              workflowStepId: STEP_ID,
              title: "Archived task",
              isArchived: true,
              statusSummary: {
                revision: 5,
                updated_at: UPDATED_AT,
                queued_prompt_count: 11,
              },
            },
          ],
        },
        loadedByWorkspaceId: { [WORKSPACE_ID]: true },
        loadingByWorkspaceId: {},
        errorByWorkspaceId: {},
        revisionByWorkspaceId: { [WORKSPACE_ID]: 1 },
      },
    } as never);

    registerTasksHandlers(store)["task.status_summary.updated"]!(
      summaryMessage({
        revision: 6,
        updated_at: "2026-08-01T18:01:00Z",
        // omit queued_prompt_count (backend omitempty for 0)
      }),
    );

    const archived = store.getState().sidebarArchivedTasks.itemsByWorkspaceId[WORKSPACE_ID]?.[0];
    expect(archived?.statusSummary).toMatchObject({ revision: 6 });
    expect(archived?.statusSummary?.queued_prompt_count).toBeUndefined();
  });
});

describe("task.status_summary.updated sidebar invalidation", () => {
  it("patches display-only summaries without refreshing the sidebar query", () => {
    const store = makeStore({
      kanban: {
        workflowId: WORKFLOW_ID,
        steps: [],
        tasks: [
          {
            id: TASK_ID,
            workspaceId: WORKSPACE_ID,
            workflowStepId: STEP_ID,
            title: "Task",
            statusSummary: {
              revision: 1,
              updated_at: UPDATED_AT,
              last_activity_at: UPDATED_AT,
            },
          },
        ],
      },
      kanbanMulti: { isLoading: false, snapshots: {} },
      sidebarArchivedTasks: {
        itemsByWorkspaceId: {},
        loadedByWorkspaceId: {},
        loadingByWorkspaceId: {},
        errorByWorkspaceId: {},
        revisionByWorkspaceId: { [WORKSPACE_ID]: 4 },
      },
    } as never);

    registerTasksHandlers(store)["task.status_summary.updated"]!(
      summaryMessage({
        revision: 2,
        updated_at: "2026-08-01T18:01:00Z",
        last_activity_at: UPDATED_AT,
        pending_action: "clarification",
      }),
    );

    expect(store.getState().sidebarArchivedTasks.revisionByWorkspaceId[WORKSPACE_ID]).toBe(4);
    expect(
      store.getState().sidebarStatusSummaryByWorkspaceId[WORKSPACE_ID]?.[TASK_ID],
    ).toMatchObject({ revision: 2, pending_action: "clarification" });
  });

  it("refreshes ordering/filter data only for a newer query-relevant summary", () => {
    const store = makeStore({
      kanban: {
        workflowId: WORKFLOW_ID,
        steps: [],
        tasks: [
          {
            id: TASK_ID,
            workspaceId: WORKSPACE_ID,
            workflowStepId: STEP_ID,
            title: "Task",
            statusSummary: {
              revision: 3,
              updated_at: UPDATED_AT,
              last_activity_at: UPDATED_AT,
            },
          },
        ],
      },
      kanbanMulti: { isLoading: false, snapshots: {} },
      sidebarArchivedTasks: {
        itemsByWorkspaceId: {},
        loadedByWorkspaceId: {},
        loadingByWorkspaceId: {},
        errorByWorkspaceId: {},
        revisionByWorkspaceId: { [WORKSPACE_ID]: 4 },
      },
    } as never);
    const handler = registerTasksHandlers(store)["task.status_summary.updated"]!;

    handler(
      summaryMessage({
        revision: 2,
        updated_at: "2026-08-01T18:01:00Z",
        last_activity_at: "2026-08-01T17:00:00Z",
      }),
    );
    expect(store.getState().sidebarArchivedTasks.revisionByWorkspaceId[WORKSPACE_ID]).toBe(4);

    handler(
      summaryMessage({
        revision: 4,
        updated_at: "2026-08-01T18:02:00Z",
        last_activity_at: "2026-08-01T19:00:00Z",
      }),
    );
    expect(store.getState().sidebarArchivedTasks.revisionByWorkspaceId[WORKSPACE_ID]).toBe(5);
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
