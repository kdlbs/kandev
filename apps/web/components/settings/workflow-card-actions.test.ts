import { act, renderHook } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import type { Workflow, WorkflowStep } from "@/lib/types/http";
import { useWorkflowStepActions } from "./workflow-card-actions";

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
