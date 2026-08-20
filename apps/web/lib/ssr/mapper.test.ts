import { describe, expect, it } from "vitest";

import { snapshotToState } from "./mapper";
import { taskId, workflowId, workspaceId } from "@/lib/types/ids";
import type { WorkflowSnapshot } from "@/lib/types/http";
import { partitionWipTasks } from "@/lib/kanban/wip-queue";

const now = "2026-07-10T12:00:00Z";
const workflowID = workflowId("workflow-1");
const workspaceID = workspaceId("workspace-1");

function snapshotWithPendingAction(action: unknown): WorkflowSnapshot {
  return {
    workflow: {
      id: workflowID,
      workspace_id: workspaceID,
      name: "Workflow",
      created_at: now,
      updated_at: now,
    },
    steps: [
      {
        id: "step-1",
        workflow_id: workflowID,
        name: "Todo",
        position: 0,
        color: "bg-neutral-400",
        allow_manual_move: true,
      },
    ],
    tasks: [
      {
        id: taskId("task-1"),
        workspace_id: workspaceID,
        workflow_id: workflowID,
        workflow_step_id: "step-1",
        position: 0,
        title: "Task",
        description: "",
        state: "TODO",
        priority: "medium",
        primary_session_pending_action: action,
        created_at: now,
        updated_at: now,
      } as WorkflowSnapshot["tasks"][number],
    ],
  };
}

// eslint-disable-next-line max-lines-per-function -- test describe block, splitting hurts readability
describe("snapshotToState", () => {
  it("keeps known primary session pending action values", () => {
    const state = snapshotToState(snapshotWithPendingAction("permission"));

    expect(state.kanban?.tasks[0]?.primarySessionPendingAction).toBe("permission");
  });

  it("drops unrecognized primary session pending action values", () => {
    const state = snapshotToState(snapshotWithPendingAction("unknown"));

    expect(state.kanban?.tasks[0]?.primarySessionPendingAction).toBeUndefined();
  });

  it("hydrates task metadata into the initial kanban state", () => {
    const snapshot = snapshotWithPendingAction(undefined);
    snapshot.tasks[0].metadata = {
      port_forwarding_enabled: true,
      unrelated: "preserve",
    };

    const state = snapshotToState(snapshot);

    expect(state.kanban?.tasks[0]?.metadata).toEqual(snapshot.tasks[0].metadata);
  });

  it.each([
    [0, 0],
    [3, 3],
    [undefined, undefined],
  ])("maps active subagent count %s", (wireValue, expected) => {
    const snapshot = snapshotWithPendingAction(undefined);
    snapshot.tasks[0].active_subagent_count = wireValue;

    const state = snapshotToState(snapshot);

    expect(state.kanban?.tasks[0]?.activeSubagentCount).toBe(expected);
  });

  it.each([
    [true, true],
    [false, false],
    [undefined, false],
  ])(
    "maps auto_start_failed %s so a page reload does not hide or resurrect the badge",
    (wireValue, expected) => {
      const snapshot = snapshotWithPendingAction(undefined);
      snapshot.tasks[0].auto_start_failed = wireValue;

      const state = snapshotToState(snapshot);

      expect(state.kanban?.tasks[0]?.autoStartFailed).toBe(expected);
    },
  );

  it("hydrates the task status summary into the initial kanban state", () => {
    const snapshot = snapshotWithPendingAction(undefined);
    snapshot.tasks[0].status_summary = {
      revision: 4,
      updated_at: now,
      primary_session: { id: "session-1", state: "WAITING_FOR_INPUT" },
    };

    const state = snapshotToState(snapshot);

    expect(state.kanban?.tasks[0]?.statusSummary).toEqual(snapshot.tasks[0].status_summary);
  });

  it("preserves workflow step WIP fields", () => {
    const state = snapshotToState({
      workflow: {
        id: workflowID,
        workspace_id: workspaceID,
        name: "Workflow",
        created_at: now,
        updated_at: now,
      },
      steps: [
        {
          id: "step-1",
          workflow_id: workflowID,
          name: "Review",
          position: 1,
          color: "bg-blue-500",
          allow_manual_move: true,
          wip_limit: 2,
          pull_from_step_id: "step-0",
        },
      ],
      tasks: [],
    } as unknown as WorkflowSnapshot);

    expect(state.kanban?.steps[0]).toMatchObject({
      wip_limit: 2,
      pull_from_step_id: "step-0",
    });
  });

  it("forwards task WIP admission + overflow fields into the kanban state", () => {
    // The HTTP snapshot path must preserve the backend queue projection.
    const snapshot = snapshotWithPendingAction(undefined);
    snapshot.tasks[0].state = "IN_PROGRESS";
    snapshot.tasks[0].priority = "high";
    snapshot.tasks[0].wip_admitted = false;
    snapshot.tasks[0].queued_for_step_id = "step-1";
    snapshot.tasks[0].queued_at = now;
    snapshot.tasks.push({
      ...snapshot.tasks[0],
      id: taskId("task-2"),
      priority: "critical",
    });

    const state = snapshotToState(snapshot);
    const task = state.kanban?.tasks.find((candidate) => candidate.id === taskId("task-1"));
    const tasks = state.kanban?.tasks ?? [];

    expect(task).toBeDefined();
    if (!task) return;

    expect(task).toMatchObject({
      state: "IN_PROGRESS",
      priority: "high",
      createdAt: now,
      wipAdmitted: false,
      queuedForStepId: "step-1",
      queuedAt: now,
    });
    const partition = partitionWipTasks(tasks, "step-1");
    expect(partition).toMatchObject({ admitted: [] });
    expect(partition.queued.map((queuedTask) => queuedTask.id)).toEqual([
      taskId("task-2"),
      taskId("task-1"),
    ]);
  });
});
