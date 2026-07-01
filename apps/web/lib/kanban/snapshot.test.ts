import { describe, expect, it } from "vitest";
import { workflowSnapshotToKanbanData } from "./snapshot";
import type { WorkflowSnapshot } from "@/lib/types/http";

describe("workflowSnapshotToKanbanData", () => {
  it("preserves workflow step WIP settings for the kanban UI", () => {
    const data = workflowSnapshotToKanbanData({
      workflow: { id: "workflow-1", name: "Review" },
      steps: [
        {
          id: "step-1",
          workflow_id: "workflow-1",
          name: "Review",
          position: 0,
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
