import { act, renderHook } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import {
  createWorkflowAction,
  createWorkflowStepAction,
  deleteWorkflowStepAction,
  getWorkflowTaskCount,
  listWorkflowStepsAction,
  reorderWorkflowStepsAction,
  updateWorkflowStepAction,
} from "@/app/actions/workspaces";
import type { Workflow, WorkflowStep } from "@/lib/types/http";
import {
  useWorkflowDeleteHandlers,
  useWorkflowSaveActions,
  useWorkflowStepActions,
} from "./workflow-card-actions";

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

function renderReadOnlyWorkflowStepActions(initialSteps: WorkflowStep[]) {
  let steps = initialSteps;
  const setWorkflowSteps = vi.fn(
    (updater: ((prev: WorkflowStep[]) => WorkflowStep[]) | WorkflowStep[]) => {
      steps = typeof updater === "function" ? updater(steps) : updater;
    },
  );
  const refreshWorkflowSteps = vi.fn();
  const view = renderHook(() =>
    useWorkflowStepActions({
      workflow,
      isNewWorkflow: false,
      readOnly: true,
      workflowSteps: steps,
      setWorkflowSteps,
      refreshWorkflowSteps,
      setStepToDelete: vi.fn(),
      setStepTaskCount: vi.fn(),
      setTargetStepForMigration: vi.fn(),
      setStepDeleteOpen: vi.fn(),
      toast: vi.fn(),
    }),
  );
  return { ...view, getSteps: () => steps, refreshWorkflowSteps, setWorkflowSteps };
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

  it("refuses to add, update, remove, or reorder steps when readOnly", async () => {
    const initialSteps = [step("step-1", "Todo", 0, true), step("step-2", "Plan", 1, false)];
    const { result, getSteps, refreshWorkflowSteps } =
      renderReadOnlyWorkflowStepActions(initialSteps);

    await act(async () => {
      await result.current.handleAddWorkflowStep();
    });
    await act(async () => {
      await result.current.handleUpdateWorkflowStep("step-2", { name: "Renamed" });
    });
    await act(async () => {
      await result.current.handleRemoveWorkflowStep("step-1");
    });
    await act(async () => {
      await result.current.handleReorderWorkflowSteps([initialSteps[1], initialSteps[0]]);
    });

    expect(getSteps()).toEqual(initialSteps);
    expect(createWorkflowStepAction).not.toHaveBeenCalled();
    expect(updateWorkflowStepAction).not.toHaveBeenCalled();
    expect(deleteWorkflowStepAction).not.toHaveBeenCalled();
    expect(reorderWorkflowStepsAction).not.toHaveBeenCalled();
    expect(refreshWorkflowSteps).not.toHaveBeenCalled();
  });
});

describe("useWorkflowDeleteHandlers", () => {
  it("refuses to open the delete-workflow dialog when readOnly", async () => {
    const wfDel = {
      setDeleteOpen: vi.fn(),
      setWorkflowTaskCount: vi.fn(),
      setWorkflowDeleteLoading: vi.fn(),
      setTargetWorkflowId: vi.fn(),
      setTargetWorkflowSteps: vi.fn(),
      setTargetStepId: vi.fn(),
      targetWorkflowId: "",
      targetStepId: "",
      setMigrateLoading: vi.fn(),
    };
    const { result } = renderHook(() =>
      useWorkflowDeleteHandlers({
        workflow,
        isNewWorkflow: false,
        readOnly: true,
        otherWorkflows: [],
        wfDel,
        deleteWorkflowRun: vi.fn(),
        toast: vi.fn(),
      }),
    );

    await act(async () => {
      await result.current.handleDeleteWorkflowClick();
    });

    expect(getWorkflowTaskCount).not.toHaveBeenCalled();
    expect(wfDel.setDeleteOpen).not.toHaveBeenCalled();
  });
});

describe("useWorkflowSaveActions", () => {
  it("remaps template workflow pull sources between backend-created and added steps", async () => {
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

    const templateStep = {
      ...step(draftTemplateId, templateName, 0, true),
      pull_from_step_id: "draft-added",
    };
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

    expect(createWorkflowStepAction).toHaveBeenCalledWith(
      expect.objectContaining({ pull_from_step_id: "" }),
    );
    expect(updateWorkflowStepAction).toHaveBeenCalledWith(backendAddedId, {
      pull_from_step_id: backendTemplateId,
    });
    expect(updateWorkflowStepAction).toHaveBeenCalledWith(backendTemplateId, {
      pull_from_step_id: backendAddedId,
    });
  });

  it("refuses to save changes to an existing workflow when readOnly", async () => {
    const onSaveWorkflow = vi.fn().mockResolvedValue(undefined);
    const { result } = renderHook(() =>
      useWorkflowSaveActions({
        workflow,
        isNewWorkflow: false,
        readOnly: true,
        workflowSteps: [],
        templateStepCount: 0,
        onSaveWorkflow,
        toast: vi.fn(),
      }),
    );

    await act(async () => {
      await result.current.handleSaveWorkflow();
    });

    expect(onSaveWorkflow).not.toHaveBeenCalled();
  });
});
