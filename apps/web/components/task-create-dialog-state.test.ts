import { describe, expect, it } from "vitest";
import { computeDialogDefaultStepId } from "./task-create-dialog-defaults";
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

  it("falls back to the lowest-position snapshot step when no start step exists", () => {
    expect(
      computeDialogDefaultStepId({
        selectedWorkflowId: null,
        workflowId: "provided",
        fetchedSteps: null,
        defaultStepId: null,
        effectiveWorkflowId: "provided",
        snapshots: {
          provided: {
            workflowId: "provided",
            workflowName: "provided",
            steps: [
              { id: "provided-2", title: "Two", color: "gray", position: 2 },
              { id: "provided-1", title: "One", color: "green", position: 1 },
            ],
            tasks: [],
          },
        },
      }),
    ).toBe("provided-1");
  });

  it("ignores a stale default step while a newly selected workflow loads", () => {
    expect(
      computeDialogDefaultStepId({
        selectedWorkflowId: "selected",
        workflowId: "original",
        fetchedSteps: null,
        defaultStepId: "original-start",
        effectiveWorkflowId: "selected",
        snapshots: {
          original: snapshot("original"),
          selected: snapshot("selected"),
        },
      }),
    ).toBe("selected-start");
  });
});
