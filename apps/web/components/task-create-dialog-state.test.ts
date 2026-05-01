import { describe, expect, it } from "vitest";
import { computeDialogDefaultStepId } from "./task-create-dialog-state";
import type { WorkflowSnapshotData } from "@/lib/state/slices/kanban/types";

function snapshot(workflowId: string): WorkflowSnapshotData {
  return {
    workflowId,
    workflowName: workflowId,
    steps: [
      {
        id: `${workflowId}-later`,
        title: "Later",
        color: "gray",
        position: 2,
      },
      {
        id: `${workflowId}-start`,
        title: "Start",
        color: "green",
        position: 1,
        is_start_step: true,
      },
    ],
    tasks: [],
  };
}

describe("computeDialogDefaultStepId", () => {
  it("uses the resolved workflow when falling back to snapshot steps", () => {
    expect(
      computeDialogDefaultStepId({
        selectedWorkflowId: null,
        workflowId: "provided",
        fetchedSteps: null,
        defaultStepId: null,
        effectiveWorkflowId: "provided",
        snapshots: {
          provided: snapshot("provided"),
          single: snapshot("single"),
        },
      }),
    ).toBe("provided-start");
  });
});
