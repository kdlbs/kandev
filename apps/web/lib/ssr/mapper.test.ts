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
  it("keeps known primary session pending action values", () => {
    const state = snapshotToState(snapshotWithPendingAction("permission"));

    expect(state.kanban?.tasks[0]?.primarySessionPendingAction).toBe("permission");
  });

  it("drops unrecognized primary session pending action values", () => {
    const state = snapshotToState(snapshotWithPendingAction("unknown"));

    expect(state.kanban?.tasks[0]?.primarySessionPendingAction).toBeUndefined();
  });
});
