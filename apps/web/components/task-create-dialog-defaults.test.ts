import { describe, expect, it } from "vitest";
import * as taskCreateDefaults from "./task-create-dialog-defaults";

const { computeDialogDefaultStepId, computeSingleWorkflowFallbackId } = taskCreateDefaults;
const HIDDEN_WORKFLOW_ID = "improve-kandev";
const VISIBLE_WORKFLOW_ID = "visible";
const VISIBLE_START_STEP_ID = "visible-start";
const DEV_WORKFLOW_ID = "dev";
const REVIEW_WORKFLOW_ID = "review";
const SUPPORT_WORKFLOW_ID = "support";
const WORKSPACE_ONE = "workspace-1";
const WORKSPACE_TWO = "workspace-2";

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

const WORKFLOW_RESOLUTION_WORKFLOWS = [
  { id: DEV_WORKFLOW_ID, workspaceId: WORKSPACE_ONE },
  { id: REVIEW_WORKFLOW_ID, workspaceId: WORKSPACE_ONE },
  { id: SUPPORT_WORKFLOW_ID, workspaceId: WORKSPACE_TWO },
  { id: "hidden", workspaceId: WORKSPACE_ONE, hidden: true },
];

const WORKFLOW_RESOLUTION_CASES = [
  {
    name: "prefers a locked workflow",
    args: {
      workspaceId: WORKSPACE_ONE,
      lockedWorkflowId: REVIEW_WORKFLOW_ID,
      manualWorkflowId: null,
      lastUsedWorkflowId: DEV_WORKFLOW_ID,
      contextWorkflowId: DEV_WORKFLOW_ID,
      workflows: WORKFLOW_RESOLUTION_WORKFLOWS,
    },
    expected: REVIEW_WORKFLOW_ID,
  },
  {
    name: "prefers a workflow manually selected in the open dialog",
    args: {
      workspaceId: WORKSPACE_ONE,
      lockedWorkflowId: null,
      manualWorkflowId: REVIEW_WORKFLOW_ID,
      lastUsedWorkflowId: DEV_WORKFLOW_ID,
      contextWorkflowId: DEV_WORKFLOW_ID,
      workflows: WORKFLOW_RESOLUTION_WORKFLOWS,
    },
    expected: REVIEW_WORKFLOW_ID,
  },
  {
    name: "prefers workspace last-used over an unlocked conflicting filter",
    args: {
      workspaceId: WORKSPACE_ONE,
      lockedWorkflowId: null,
      manualWorkflowId: null,
      lastUsedWorkflowId: DEV_WORKFLOW_ID,
      contextWorkflowId: REVIEW_WORKFLOW_ID,
      workflows: WORKFLOW_RESOLUTION_WORKFLOWS,
    },
    expected: DEV_WORKFLOW_ID,
  },
  {
    name: "falls back when remembered workflow is hidden",
    args: {
      workspaceId: WORKSPACE_ONE,
      lockedWorkflowId: null,
      manualWorkflowId: null,
      lastUsedWorkflowId: "hidden",
      contextWorkflowId: REVIEW_WORKFLOW_ID,
      workflows: WORKFLOW_RESOLUTION_WORKFLOWS,
    },
    expected: REVIEW_WORKFLOW_ID,
  },
  {
    name: "falls back when remembered workflow belongs to another workspace",
    args: {
      workspaceId: WORKSPACE_ONE,
      lockedWorkflowId: null,
      manualWorkflowId: null,
      lastUsedWorkflowId: SUPPORT_WORKFLOW_ID,
      contextWorkflowId: REVIEW_WORKFLOW_ID,
      workflows: WORKFLOW_RESOLUTION_WORKFLOWS,
    },
    expected: REVIEW_WORKFLOW_ID,
  },
  {
    name: "selects the sole visible workflow without context",
    args: {
      workspaceId: WORKSPACE_TWO,
      lockedWorkflowId: null,
      manualWorkflowId: null,
      lastUsedWorkflowId: null,
      contextWorkflowId: null,
      workflows: [
        { id: SUPPORT_WORKFLOW_ID, workspaceId: WORKSPACE_TWO },
        { id: "hidden", workspaceId: WORKSPACE_TWO, hidden: true },
      ],
    },
    expected: SUPPORT_WORKFLOW_ID,
  },
  {
    name: "leaves multiple workflows unselected without a valid default",
    args: {
      workspaceId: WORKSPACE_ONE,
      lockedWorkflowId: null,
      manualWorkflowId: null,
      lastUsedWorkflowId: null,
      contextWorkflowId: null,
      workflows: WORKFLOW_RESOLUTION_WORKFLOWS,
    },
    expected: null,
  },
];

describe("resolveEffectiveTaskCreateWorkflowId", () => {
  it.each(WORKFLOW_RESOLUTION_CASES)("$name", ({ args, expected }) => {
    expect(taskCreateDefaults.resolveEffectiveTaskCreateWorkflowId(args)).toBe(expected);
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
    {
      name: "drops unlocked workflow context whose metadata cannot be resolved",
      workflowId: "not-yet-loaded",
      defaultStepId: "start",
      allowHiddenWorkflow: false,
      expected: { workflowId: null, defaultStepId: null },
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
          {
            id: "visible-backlog",
            title: "Backlog",
            workflowId: VISIBLE_WORKFLOW_ID,
            position: 0,
          },
          {
            id: VISIBLE_START_STEP_ID,
            title: "In Progress",
            workflowId: VISIBLE_WORKFLOW_ID,
            position: 1,
            is_start_step: true,
          },
        ],
        defaultStepId: null,
        effectiveWorkflowId: VISIBLE_WORKFLOW_ID,
        snapshots: {},
      }),
    ).toBe(VISIBLE_START_STEP_ID);
  });

  it("ignores fetched steps from a previously selected workflow", () => {
    expect(
      computeDialogDefaultStepId({
        selectedWorkflowId: VISIBLE_WORKFLOW_ID,
        workflowId: null,
        fetchedSteps: [
          {
            id: "previous-start",
            title: "Previous start",
            is_start_step: true,
            workflowId: "previous",
          },
        ],
        defaultStepId: null,
        effectiveWorkflowId: VISIBLE_WORKFLOW_ID,
        snapshots: {
          [VISIBLE_WORKFLOW_ID]: {
            workflowId: VISIBLE_WORKFLOW_ID,
            workflowName: "Visible",
            steps: [
              {
                id: VISIBLE_START_STEP_ID,
                title: "Visible start",
                color: "green",
                position: 0,
                is_start_step: true,
              },
            ],
            tasks: [],
          },
        },
      }),
    ).toBe(VISIBLE_START_STEP_ID);
  });
});

describe("resolveTaskCreateWorkflowContext visibility defaults", () => {
  it("treats a workflow with omitted hidden metadata as visible", () => {
    const resolver = (
      taskCreateDefaults as typeof taskCreateDefaults & {
        resolveTaskCreateWorkflowContext?: WorkflowContextResolver;
      }
    ).resolveTaskCreateWorkflowContext;

    expect(resolver).toBeTypeOf("function");
    if (!resolver) return;

    expect(resolver("legacy", "start", [{ id: "legacy" }], false)).toEqual({
      workflowId: "legacy",
      defaultStepId: "start",
    });
  });
});
