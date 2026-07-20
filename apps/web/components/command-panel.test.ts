import { describe, expect, it } from "vitest";
import { taskId, workflowId, workspaceId, type Task } from "@/lib/types/http";
import {
  getCommandPanelActiveTaskResults,
  getCommandPanelSearchTaskResults,
  getCommandPanelStepsFromSnapshots,
} from "./command-panel";

const STEP_ONE = "step-1";
const STEP_REVIEW = "step-review";
const STEP_DOING = "step-doing";

function makeTask(id: string, state: Task["state"], stepId: string, title = id): Task {
  return {
    id: taskId(id),
    workspace_id: workspaceId("ws-1"),
    workflow_id: workflowId("wf-1"),
    workflow_step_id: stepId,
    position: 1,
    title,
    description: "",
    state,
    priority: 0,
    repositories: [],
    created_at: "2026-01-01T00:00:00Z",
    updated_at: "2026-01-01T00:00:00Z",
  };
}

describe("command panel task results", () => {
  it("filters active results to visible non-archived steps and sorts by step position", () => {
    const visibleStepIds = new Set([STEP_REVIEW, STEP_DOING]);
    const stepPositions = new Map([
      [STEP_REVIEW, 3],
      [STEP_DOING, 1],
    ]);

    const results = getCommandPanelActiveTaskResults(
      [
        makeTask("review", "CREATED", STEP_REVIEW),
        makeTask("done", "COMPLETED", STEP_DOING),
        makeTask("hidden", "CREATED", "step-hidden"),
        makeTask("doing", "CREATED", STEP_DOING),
      ],
      visibleStepIds,
      stepPositions,
    );

    expect(results.map((task) => task.id)).toEqual(["doing", "review"]);
  });

  it("keeps archived task search matches after active matches", () => {
    const results = getCommandPanelSearchTaskResults([
      makeTask("failed", "FAILED", STEP_ONE),
      makeTask("active", "CREATED", STEP_ONE),
      makeTask("cancelled", "CANCELLED", STEP_ONE),
    ]);

    expect(results.map((task) => task.id)).toEqual(["active", "failed", "cancelled"]);
  });

  it("derives task result step labels from Query snapshots instead of legacy kanban steps", () => {
    const steps = getCommandPanelStepsFromSnapshots({
      "wf-1": {
        workflowId: "wf-1",
        workflowName: "Workflow",
        tasks: [],
        steps: [
          {
            id: STEP_REVIEW,
            title: "Review",
            color: "bg-green-500",
            position: 0,
            show_in_command_panel: true,
          },
        ],
      },
    });

    expect(steps).toEqual([
      {
        id: STEP_REVIEW,
        title: "Review",
        color: "bg-green-500",
        position: 0,
        show_in_command_panel: true,
      },
    ]);
  });
});
