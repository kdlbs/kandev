import { act, renderHook } from "@testing-library/react";
import { beforeEach, expect, it, vi } from "vitest";
import { reorderWorkflowStepsAction } from "@/app/actions/workspaces";
import type { Workflow, WorkflowStep } from "@/lib/types/http";
import { useWorkflowStepActions } from "./workflow-card-actions";
import { useWorkflowMutationGuard } from "./workflow-mutation-guard";

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

beforeEach(() => {
  vi.clearAllMocks();
});

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
    events: { on_enter: [] },
    created_at: "",
    updated_at: "",
  };
}

function renderReorderActions(initialSteps: WorkflowStep[]) {
  let currentSteps = initialSteps;
  const setWorkflowSteps = vi.fn(
    (updater: ((previous: WorkflowStep[]) => WorkflowStep[]) | WorkflowStep[]) => {
      currentSteps = typeof updater === "function" ? updater(currentSteps) : updater;
    },
  );
  const view = renderHook(() => {
    const mutationGuard = useWorkflowMutationGuard(initialSteps);
    return {
      actions: useWorkflowStepActions({
        workflow,
        isNewWorkflow: false,
        workflowSteps: initialSteps,
        setWorkflowSteps,
        setStepToDelete: vi.fn(),
        setStepTaskCount: vi.fn(),
        setTargetStepForMigration: vi.fn(),
        setStepDeleteOpen: vi.fn(),
        toast: vi.fn(),
        mutationGuard,
      }),
      mutationGuard,
    };
  });
  return { ...view, setWorkflowSteps, getSteps: () => currentSteps };
}

it("stages a reorder locally without calling persistence", async () => {
  const work = step("work", "Work", 0, true);
  const review = step("review", "Review", 1, false);
  const proposed = [
    { ...review, position: 0 },
    { ...work, position: 1 },
  ];
  const { result, getSteps } = renderReorderActions([work, review]);

  await act(async () => {
    await result.current.actions.handleReorderWorkflowSteps([review, work]);
  });

  expect(getSteps()).toEqual(proposed);
  expect(reorderWorkflowStepsAction).not.toHaveBeenCalled();
});

it("holds a cycle-introducing reorder until the warning is confirmed", async () => {
  const work = step("work", "Work", 0, true);
  work.events = {
    on_enter: [{ type: "auto_start_agent" }],
    on_turn_complete: [{ type: "move_to_next" }],
  };
  const parked = step("parked", "Parked", 1, false);
  const review = step("review", "Review", 2, false);
  review.events = {
    on_turn_complete: [{ type: "move_to_step", config: { step_id: work.id } }],
  };
  const initial = [work, parked, review];
  const proposed = [
    { ...work, position: 0 },
    { ...review, position: 1 },
    { ...parked, position: 2 },
  ];
  const { result, setWorkflowSteps, getSteps } = renderReorderActions(initial);

  await act(async () => {
    await result.current.actions.handleReorderWorkflowSteps([work, review, parked]);
  });
  expect(result.current.mutationGuard.proposal?.severity).toBe("warning");
  expect(setWorkflowSteps).not.toHaveBeenCalled();
  expect(getSteps()).toEqual(initial);

  await act(async () => {
    await result.current.mutationGuard.confirmProposal();
  });
  expect(getSteps()).toEqual(proposed);
  expect(reorderWorkflowStepsAction).not.toHaveBeenCalled();
});
