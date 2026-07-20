import { describe, expect, it } from "vitest";

import { snapshotToState } from "./mapper";
import { taskId, workflowId, workspaceId } from "@/lib/types/ids";
import type { WorkflowSnapshot } from "@/lib/types/http";

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
        priority: 0,
        primary_session_pending_action: action,
        created_at: now,
        updated_at: now,
      } as WorkflowSnapshot["tasks"][number],
    ],
  };
}

describe("snapshotToState", () => {
  it("seeds workflow snapshots for Query hydration", () => {
    const snapshot = snapshotWithPendingAction("permission");
    const state = snapshotToState(snapshot);

    expect(state.workflowSnapshots?.itemsByWorkflowId?.[workflowID]).toBe(snapshot);
  });

  it("returns an empty seed when the snapshot has no workflow", () => {
    const state = snapshotToState({
      ...snapshotWithPendingAction("unknown"),
      workflow: null,
    } as unknown as WorkflowSnapshot);

    expect(state).toEqual({});
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

    expect(state.workflowSnapshots?.itemsByWorkflowId?.[workflowID]?.steps[0]).toMatchObject({
      wip_limit: 2,
      pull_from_step_id: "step-0",
    });
  });
});
