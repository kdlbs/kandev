import { describe, expect, it } from "vitest";
import { workflowSnapshotToKanbanData } from "./snapshot";
import type { WorkflowSnapshot } from "@/lib/types/http";
import { workflowId, workspaceId } from "@/lib/types/ids";

describe("workflowSnapshotToKanbanData", () => {
  it("preserves workflow step WIP settings for the kanban UI", () => {
    const data = workflowSnapshotToKanbanData({
      workflow: {
        id: workflowId("workflow-1"),
        workspace_id: workspaceId("workspace-1"),
        name: "Review",
        created_at: "2026-07-13T00:00:00Z",
        updated_at: "2026-07-13T00:00:00Z",
      },
      steps: [
        {
          id: "step-1",
          workflow_id: workflowId("workflow-1"),
          name: "Review",
          position: 0,
          color: "bg-blue-500",
          allow_manual_move: true,
          wip_limit: 2,
          pull_from_step_id: "step-0",
        },
      ],
      tasks: [],
    } as WorkflowSnapshot);

    expect(data.steps[0]).toMatchObject({
      wip_limit: 2,
      pull_from_step_id: "step-0",
    });
  });
});
