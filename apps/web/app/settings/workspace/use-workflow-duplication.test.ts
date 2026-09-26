import { act, renderHook } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { listWorkflowStepsAction } from "@/app/actions/workspaces";
import type { Workflow, WorkflowStep } from "@/lib/types/http";
import { useWorkflowDuplication } from "./use-workflow-duplication";

vi.mock("@/app/actions/workspaces", () => ({
  listWorkflowStepsAction: vi.fn(),
}));

const workflow = {
  id: "workflow-1",
  workspace_id: "workspace-1",
  name: "Workflow",
  created_at: "saved",
  updated_at: "saved",
} as Workflow;

const steps = [
  {
    id: "step-1",
    workflow_id: workflow.id,
    name: "Step",
    position: 0,
    created_at: "saved",
    updated_at: "saved",
  },
] as WorkflowStep[];

function renderDuplicationHook(
  overrides: Partial<Parameters<typeof useWorkflowDuplication>[0]> = {},
) {
  const onDuplicateWorkflow = vi.fn();
  const toast = vi.fn();
  const view = renderHook(() =>
    useWorkflowDuplication({
      workflow,
      hasUnsavedChanges: false,
      mutationPending: false,
      onDuplicateWorkflow,
      toast,
      ...overrides,
    }),
  );
  return { ...view, onDuplicateWorkflow, toast };
}

beforeEach(() => {
  vi.clearAllMocks();
  vi.mocked(listWorkflowStepsAction).mockResolvedValue({ steps, total: steps.length });
});

describe("useWorkflowDuplication", () => {
  it("loads saved steps and delegates them to the draft creator", async () => {
    const { result, onDuplicateWorkflow } = renderDuplicationHook();

    await act(async () => {
      await result.current.handleDuplicateWorkflow();
    });

    expect(listWorkflowStepsAction).toHaveBeenCalledWith(workflow.id);
    expect(onDuplicateWorkflow).toHaveBeenCalledWith(steps);
    expect(result.current.duplicateLoading).toBe(false);
  });

  it("blocks dirty sources and explains that they must be saved first", () => {
    const { result } = renderDuplicationHook({ hasUnsavedChanges: true });

    expect(result.current.duplicateDisabled).toBe(true);
    expect(result.current.duplicateDisabledReason).toBe("Save the workflow before duplicating it.");
    expect(listWorkflowStepsAction).not.toHaveBeenCalled();
  });

  it("keeps sync-managed sources copyable", () => {
    const { result } = renderDuplicationHook({
      workflow: { ...workflow, source: "github" } as Workflow,
    });

    expect(result.current.duplicateDisabled).toBe(false);
  });

  it("shows a load failure without creating a draft", async () => {
    const failure = new Error("step load failed");
    vi.mocked(listWorkflowStepsAction).mockRejectedValueOnce(failure);
    const { result, onDuplicateWorkflow, toast } = renderDuplicationHook();

    await act(async () => {
      await result.current.handleDuplicateWorkflow();
    });

    expect(onDuplicateWorkflow).not.toHaveBeenCalled();
    expect(toast).toHaveBeenCalledWith({
      title: "Failed to duplicate workflow",
      description: "step load failed",
      variant: "error",
    });
    expect(result.current.duplicateLoading).toBe(false);
  });

  it("ignores a second activation while the saved steps are loading", async () => {
    let resolveSteps!: (value: { steps: WorkflowStep[]; total: number }) => void;
    vi.mocked(listWorkflowStepsAction).mockReturnValueOnce(
      new Promise((resolve) => {
        resolveSteps = resolve;
      }),
    );
    const { result, onDuplicateWorkflow } = renderDuplicationHook();

    let firstRequest: Promise<void>;
    let secondRequest: Promise<void>;
    await act(async () => {
      firstRequest = result.current.handleDuplicateWorkflow();
      secondRequest = result.current.handleDuplicateWorkflow();
      await Promise.resolve();
    });

    expect(listWorkflowStepsAction).toHaveBeenCalledOnce();
    resolveSteps({ steps, total: steps.length });
    await act(async () => {
      await Promise.all([firstRequest!, secondRequest!]);
    });

    expect(onDuplicateWorkflow).toHaveBeenCalledOnce();
  });
});
