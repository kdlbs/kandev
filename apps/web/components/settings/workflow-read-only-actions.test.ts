import { act, renderHook } from "@testing-library/react";
import { beforeEach, expect, it, vi } from "vitest";
import {
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
  vi.mocked(listWorkflowStepsAction).mockResolvedValue({ steps: [], total: 0 });
});

const workflow = {
  id: "wf-1",
  workspace_id: "ws-1",
  name: "Synced workflow",
  source: "github",
  source_path: ".kandev/workflows/review.yaml",
  created_at: "",
  updated_at: "",
} as Workflow;

function step(id: string, position: number): WorkflowStep {
  return {
    id,
    workflow_id: workflow.id,
    name: id,
    position,
    color: "bg-slate-500",
    allow_manual_move: true,
    is_start_step: position === 0,
    events: { on_enter: [] },
    created_at: "",
    updated_at: "",
  };
}

it("refuses every step mutation when the workflow is read-only", async () => {
  const initialSteps = [step("todo", 0), step("review", 1)];
  let currentSteps = initialSteps;
  const setWorkflowSteps = vi.fn(
    (updater: ((previous: WorkflowStep[]) => WorkflowStep[]) | WorkflowStep[]) => {
      currentSteps = typeof updater === "function" ? updater(currentSteps) : updater;
    },
  );
  const { result } = renderHook(() => {
    const mutationGuard = useWorkflowMutationGuard(initialSteps);
    return useWorkflowStepActions({
      workflow,
      isNewWorkflow: false,
      readOnly: true,
      workflowSteps: initialSteps,
      setWorkflowSteps,
      setStepToDelete: vi.fn(),
      setStepTaskCount: vi.fn(),
      setTargetStepForMigration: vi.fn(),
      setStepDeleteOpen: vi.fn(),
      toast: vi.fn(),
      mutationGuard,
    });
  });

  await act(async () => {
    await result.current.handleAddWorkflowStep();
    await result.current.handleUpdateWorkflowStep("review", { name: "Renamed" });
    await result.current.handleRemoveWorkflowStep("todo");
    await result.current.handleReorderWorkflowSteps([...initialSteps].reverse());
  });

  expect(currentSteps).toEqual(initialSteps);
  expect(setWorkflowSteps).not.toHaveBeenCalled();
  expect(createWorkflowStepAction).not.toHaveBeenCalled();
  expect(updateWorkflowStepAction).not.toHaveBeenCalled();
  expect(deleteWorkflowStepAction).not.toHaveBeenCalled();
  expect(reorderWorkflowStepsAction).not.toHaveBeenCalled();
});

it("refuses to open or confirm workflow deletion when read-only", async () => {
  const deleteWorkflowRun = vi.fn();
  const wfDel = {
    setDeleteOpen: vi.fn(),
    setWorkflowTaskCount: vi.fn(),
    setWorkflowDeleteLoading: vi.fn(),
    setTargetWorkflowId: vi.fn(),
    setTargetWorkflowSteps: vi.fn(),
    setTargetStepId: vi.fn(),
    targetWorkflowId: "target-workflow",
    targetStepId: "target-step",
    setMigrateLoading: vi.fn(),
  };
  const { result } = renderHook(() =>
    useWorkflowDeleteHandlers({
      workflow,
      isNewWorkflow: false,
      readOnly: true,
      otherWorkflows: [],
      wfDel,
      deleteWorkflowRun,
      toast: vi.fn(),
    }),
  );

  await act(async () => {
    await result.current.handleDeleteWorkflowClick();
    await result.current.handleDeleteWorkflow();
    await result.current.handleMigrateAndDeleteWorkflow();
  });

  expect(getWorkflowTaskCount).not.toHaveBeenCalled();
  expect(deleteWorkflowRun).not.toHaveBeenCalled();
  expect(wfDel.setDeleteOpen).not.toHaveBeenCalled();
  expect(wfDel.setMigrateLoading).not.toHaveBeenCalled();
});

it("refuses to save an existing workflow when read-only", async () => {
  const onSaveWorkflow = vi.fn();
  const { result } = renderHook(() => {
    const mutationGuard = useWorkflowMutationGuard([]);
    return useWorkflowSaveActions({
      workflow,
      isNewWorkflow: false,
      readOnly: true,
      workflowSteps: [],
      templateStepCount: 0,
      onSaveWorkflow,
      toast: vi.fn(),
      mutationGuard,
    });
  });

  await act(async () => {
    await result.current.handleSaveWorkflow();
  });

  expect(onSaveWorkflow).not.toHaveBeenCalled();
});
