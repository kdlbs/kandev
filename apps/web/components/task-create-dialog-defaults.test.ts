import { describe, expect, it } from "vitest";
import * as taskCreateDefaults from "./task-create-dialog-defaults";

const { computeDialogDefaultStepId, computeSingleWorkflowFallbackId } = taskCreateDefaults;
const HIDDEN_WORKFLOW_ID = "improve-kandev";

type WorkflowContextResolver = (
  workflowId: string | null,
  defaultStepId: string | null,
  workflows: Array<{ id: string; hidden?: boolean }>,
  allowHiddenWorkflow: boolean,
) => { workflowId: string | null; defaultStepId: string | null };

describe("computeSingleWorkflowFallbackId", () => {
  it("selects the sole visible workflow when hidden workflows are also loaded", () => {
    const workflowId = computeSingleWorkflowFallbackId(null, null, [
      { id: "kanban", hidden: false },
      { id: HIDDEN_WORKFLOW_ID, hidden: true },
    ]);

    expect(workflowId).toBe("kanban");
  });
});

describe("resolveTaskCreateWorkflowContext", () => {
  it.each([
    {
      name: "drops an unlocked hidden workflow inherited from task context",
      workflowId: HIDDEN_WORKFLOW_ID,
      defaultStepId: "improve",
      allowHiddenWorkflow: false,
      expected: { workflowId: null, defaultStepId: null },
    },
    {
      name: "preserves a locked hidden workflow from a feature wrapper",
      workflowId: HIDDEN_WORKFLOW_ID,
      defaultStepId: "improve",
      allowHiddenWorkflow: true,
      expected: { workflowId: HIDDEN_WORKFLOW_ID, defaultStepId: "improve" },
    },
    {
      name: "preserves an ordinary visible workflow",
      workflowId: "kanban",
      defaultStepId: "backlog",
      allowHiddenWorkflow: false,
      expected: { workflowId: "kanban", defaultStepId: "backlog" },
    },
  ])("$name", ({ workflowId, defaultStepId, allowHiddenWorkflow, expected }) => {
    const resolver = (
      taskCreateDefaults as typeof taskCreateDefaults & {
        resolveTaskCreateWorkflowContext?: WorkflowContextResolver;
      }
    ).resolveTaskCreateWorkflowContext;

    expect(resolver).toBeTypeOf("function");
    if (!resolver) return;

    expect(
      resolver(
        workflowId,
        defaultStepId,
        [
          { id: "kanban", hidden: false },
          { id: HIDDEN_WORKFLOW_ID, hidden: true },
        ],
        allowHiddenWorkflow,
      ),
    ).toEqual(expected);
  });
});

describe("computeDialogDefaultStepId", () => {
  it("uses the fetched start step when visible fallback replaces hidden context", () => {
    expect(
      computeDialogDefaultStepId({
        selectedWorkflowId: null,
        workflowId: null,
        fetchedSteps: [
          { id: "visible-backlog", title: "Backlog", position: 0 },
          {
            id: "visible-start",
            title: "In Progress",
            position: 1,
            is_start_step: true,
          },
        ],
        defaultStepId: null,
        effectiveWorkflowId: "visible",
        snapshots: {},
      }),
    ).toBe("visible-start");
  });
});
