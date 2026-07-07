import { act, renderHook } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import {
  createWorkflowAction,
  createWorkflowStepAction,
  listWorkflowStepsAction,
  updateWorkflowStepAction,
} from "@/app/actions/workspaces";
import type { Workflow, WorkflowStep } from "@/lib/types/http";
import { useWorkflowSaveActions, useWorkflowStepActions } from "./workflow-card-actions";

vi.mock("@/app/actions/workspaces", () => ({
  createWorkflowAction: vi.fn(),
  createWorkflowStepAction: vi.fn(),
  updateWorkflowStepAction: vi.fn(),
  deleteWorkflowStepAction: vi.fn(),
  reorderWorkflowStepsAction: vi.fn(),
  listWorkflowStepsAction: vi.fn(),
  getStepTaskCount: vi.fn(),
  getWorkflowTaskCount: vi.fn(),
  exportWorkflowAction: vi.fn(),
  bulkMoveTasks: vi.fn(),
}));

const workflow = {
  id: "wf-1",
  workspace_id: "ws-1",
  name: "Workflow",
  created_at: "",
  updated_at: "",
} as Workflow;

function step(id: string, name: string, position: number, isStartStep: boolean): WorkflowStep {
  return {
    id,
    workflow_id: workflow.id,
    name,
    position,
    color: "bg-slate-500",
    allow_manual_move: true,
    is_start_step: isStartStep,
    created_at: "",
    updated_at: "",
  };
}

function renderNewWorkflowStepActions(initialSteps: WorkflowStep[]) {
  let steps = initialSteps;
  const setWorkflowSteps = vi.fn(
    (updater: ((prev: WorkflowStep[]) => WorkflowStep[]) | WorkflowStep[]) => {
      steps = typeof updater === "function" ? updater(steps) : updater;
    },
  );
  const view = renderHook(() =>
    useWorkflowStepActions({
      workflow,
      isNewWorkflow: true,
      workflowSteps: steps,
      setWorkflowSteps,
      refreshWorkflowSteps: vi.fn(),
      setStepToDelete: vi.fn(),
      setStepTaskCount: vi.fn(),
      setTargetStepForMigration: vi.fn(),
      setStepDeleteOpen: vi.fn(),
      toast: vi.fn(),
    }),
  );
  return { ...view, getSteps: () => steps };
}

describe("useWorkflowStepActions", () => {
  it("keeps one start step while editing a new workflow locally", async () => {
    const { result, getSteps } = renderNewWorkflowStepActions([
      step("step-1", "Todo", 0, true),
      step("step-2", "Plan", 1, false),
    ]);

    await act(async () => {
      await result.current.handleUpdateWorkflowStep("step-2", { is_start_step: true });
    });

    expect(
      getSteps()
        .filter((s) => s.is_start_step)
        .map((s) => s.id),
    ).toEqual(["step-2"]);
  });
});

describe("useWorkflowSaveActions", () => {
  it("remaps added template step pull sources to backend-created template steps", async () => {
    const createdWorkflowId = "wf-created" as Workflow["id"];
    const templateName = "Template Step";
    const addedName = "Added Step";
    const backendTemplateId = "backend-template";
    const backendAddedId = "backend-added";
    const draftTemplateId = "draft-template";

    vi.mocked(createWorkflowAction).mockResolvedValue({
      ...workflow,
      id: createdWorkflowId,
      workflow_template_id: "template-1",
    });
    vi.mocked(listWorkflowStepsAction).mockResolvedValue({
      steps: [
        {
          ...step(backendTemplateId, templateName, 0, true),
          id: backendTemplateId,
          workflow_id: createdWorkflowId,
        },
      ],
      total: 1,
    });
    vi.mocked(createWorkflowStepAction).mockResolvedValue({
      ...step(backendAddedId, addedName, 1, false),
      id: backendAddedId,
      workflow_id: createdWorkflowId,
    });
    vi.mocked(updateWorkflowStepAction).mockResolvedValue(
      step(backendAddedId, addedName, 1, false),
    );

    const templateStep = step(draftTemplateId, templateName, 0, true);
    const addedStep = {
      ...step("draft-added", addedName, 1, false),
      pull_from_step_id: draftTemplateId,
    };
    const { result } = renderHook(() =>
      useWorkflowSaveActions({
        workflow: { ...workflow, workflow_template_id: "template-1" },
        isNewWorkflow: true,
        workflowSteps: [templateStep, addedStep],
        templateStepCount: 1,
        onSaveWorkflow: vi.fn(),
        onWorkflowCreated: vi.fn(),
        toast: vi.fn(),
      }),
    );

    await act(async () => {
      await result.current.handleSaveWorkflow();
    });

    expect(updateWorkflowStepAction).toHaveBeenCalledWith(backendAddedId, {
      pull_from_step_id: backendTemplateId,
    });
  });
});
