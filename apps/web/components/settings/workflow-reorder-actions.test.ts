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
    return useWorkflowStepActions({
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
    });
  });
  return { ...view, setWorkflowSteps, getSteps: () => currentSteps };
}

function deferredPromise<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((resolvePromise) => {
    resolve = resolvePromise;
  });
  return { promise, resolve };
}

it("keeps a reorder optimistic until the authoritative response arrives", async () => {
  const work = step("work", "Work", 0, true);
  const review = step("review", "Review", 1, false);
  const proposed = [
    { ...review, position: 0 },
    { ...work, position: 1 },
  ];
  const authoritative = proposed.map((workflowStep) => ({
    ...workflowStep,
    updated_at: "server-update",
  }));
  const response = deferredPromise<{ steps: WorkflowStep[]; total: number }>();
  vi.mocked(reorderWorkflowStepsAction).mockReturnValue(response.promise);
  const { result, getSteps } = renderReorderActions([work, review]);

  let request!: Promise<void>;
  await act(async () => {
    request = result.current.handleReorderWorkflowSteps([review, work]);
    await vi.waitFor(() => expect(reorderWorkflowStepsAction).toHaveBeenCalledTimes(1));
  });
  expect(getSteps()).toEqual(proposed);

  response.resolve({ steps: authoritative, total: 2 });
  await act(async () => {
    await request;
  });
  expect(getSteps()).toEqual(authoritative);
});

it("rolls an optimistic reorder back when persistence fails", async () => {
  const work = step("work", "Work", 0, true);
  const review = step("review", "Review", 1, false);
  const initial = [work, review];
  const proposed = [
    { ...review, position: 0 },
    { ...work, position: 1 },
  ];
  vi.mocked(reorderWorkflowStepsAction).mockRejectedValue(new Error("reorder failed"));
  const { result, setWorkflowSteps, getSteps } = renderReorderActions(initial);

  await act(async () => {
    await result.current.handleReorderWorkflowSteps([review, work]);
  });

  expect(setWorkflowSteps).toHaveBeenNthCalledWith(1, proposed);
  expect(setWorkflowSteps).toHaveBeenNthCalledWith(2, initial);
  expect(getSteps()).toEqual(initial);
});
