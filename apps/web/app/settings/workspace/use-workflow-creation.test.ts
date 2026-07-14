import { act, renderHook } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import {
  createWorkflowAction,
  createWorkflowStepAction,
  deleteWorkflowAction,
} from "@/app/actions/workspaces";
import type { Workflow, WorkflowStep, Workspace } from "@/lib/types/http";
import { useWorkflowCreation } from "./use-workflow-creation";

vi.mock("@/app/actions/workspaces", () => ({
  createWorkflowAction: vi.fn(),
  createWorkflowStepAction: vi.fn(),
  deleteWorkflowAction: vi.fn(),
  listWorkflowStepsAction: vi.fn(),
}));

const workspace = { id: "workspace-1", name: "Workspace" } as Workspace;
const workflow = {
  id: "workflow-1",
  workspace_id: workspace.id,
  name: "Custom Workflow",
} as Workflow;

function createdStep(position: number): WorkflowStep {
  return {
    id: `step-${position}`,
    workflow_id: workflow.id,
    name: `Step ${position}`,
    position,
  } as WorkflowStep;
}

function renderCreationHook() {
  const setWorkflowItems = vi.fn();
  const setSavedWorkflowItems = vi.fn();
  const toast = vi.fn();
  const view = renderHook(() =>
    useWorkflowCreation({
      workspace,
      workflowTemplates: [],
      setWorkflowItems,
      setSavedWorkflowItems,
      toast,
    }),
  );
  return { ...view, setWorkflowItems, setSavedWorkflowItems, toast };
}

beforeEach(() => {
  vi.clearAllMocks();
  vi.mocked(deleteWorkflowAction).mockResolvedValue(undefined);
});

describe("useWorkflowCreation", () => {
  it("persists a custom workflow and its default steps before exposing it", async () => {
    vi.mocked(createWorkflowAction).mockResolvedValue(workflow);
    vi.mocked(createWorkflowStepAction).mockImplementation(async ({ position }) =>
      createdStep(position),
    );
    const { result, setWorkflowItems, setSavedWorkflowItems } = renderCreationHook();

    await act(async () => {
      result.current.setNewWorkflowName("Custom Workflow");
      result.current.setSelectedTemplateId(null);
    });
    await act(async () => {
      await result.current.handleCreateWorkflow();
    });

    expect(createWorkflowStepAction).toHaveBeenCalledTimes(4);
    expect(result.current.initialStepsByWorkflowId.get(workflow.id)).toHaveLength(4);
    expect(setWorkflowItems).toHaveBeenCalledOnce();
    expect(setSavedWorkflowItems).toHaveBeenCalledOnce();
    expect(deleteWorkflowAction).not.toHaveBeenCalled();
  });

  it("deletes a partially created workflow when default step creation fails", async () => {
    vi.mocked(createWorkflowAction).mockResolvedValue(workflow);
    vi.mocked(createWorkflowStepAction).mockRejectedValueOnce(new Error("step failed"));
    const { result, setWorkflowItems, toast } = renderCreationHook();

    await act(async () => {
      await result.current.handleCreateWorkflow();
    });

    expect(deleteWorkflowAction).toHaveBeenCalledWith(workflow.id);
    expect(setWorkflowItems).not.toHaveBeenCalled();
    expect(toast).toHaveBeenCalledWith(expect.objectContaining({ variant: "error" }));
  });
});
