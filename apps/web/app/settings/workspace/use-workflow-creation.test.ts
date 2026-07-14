import { act, renderHook } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import {
  createWorkflowAction,
  createWorkflowStepAction,
  deleteWorkflowAction,
  listWorkflowStepsAction,
} from "@/app/actions/workspaces";
import type { Workflow, WorkflowStep, WorkflowTemplate, Workspace } from "@/lib/types/http";
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

const template = {
  id: "template-1",
  name: "Template",
  default_steps: [{ name: "Template Step", position: 0 }],
} as WorkflowTemplate;

function renderCreationHook(workflowTemplates: WorkflowTemplate[] = []) {
  const setWorkflowItems = vi.fn();
  const setSavedWorkflowItems = vi.fn();
  const toast = vi.fn();
  const view = renderHook(() =>
    useWorkflowCreation({
      workspace,
      workflowTemplates,
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

  it("loads and exposes the persisted steps for a template workflow", async () => {
    const templateStep = createdStep(0);
    vi.mocked(createWorkflowAction).mockResolvedValue(workflow);
    vi.mocked(listWorkflowStepsAction).mockResolvedValue({ steps: [templateStep], total: 1 });
    const { result, setWorkflowItems } = renderCreationHook([template]);

    await act(async () => {
      result.current.setSelectedTemplateId(template.id);
    });
    await act(async () => {
      await result.current.handleCreateWorkflow();
    });

    expect(listWorkflowStepsAction).toHaveBeenCalledWith(workflow.id);
    expect(result.current.initialStepsByWorkflowId.get(workflow.id)).toEqual([templateStep]);
    expect(setWorkflowItems).toHaveBeenCalledOnce();
  });

  it("deletes a partially created template workflow when loading its steps fails", async () => {
    vi.mocked(createWorkflowAction).mockResolvedValue(workflow);
    vi.mocked(listWorkflowStepsAction).mockRejectedValueOnce(new Error("step fetch failed"));
    const { result, setWorkflowItems, toast } = renderCreationHook([template]);

    await act(async () => {
      result.current.setSelectedTemplateId(template.id);
    });
    await act(async () => {
      await result.current.handleCreateWorkflow();
    });

    expect(deleteWorkflowAction).toHaveBeenCalledWith(workflow.id);
    expect(setWorkflowItems).not.toHaveBeenCalled();
    expect(toast).toHaveBeenCalledWith(expect.objectContaining({ variant: "error" }));
  });

  it("keeps a partially initialized workflow visible when rollback fails", async () => {
    vi.mocked(createWorkflowAction).mockResolvedValue(workflow);
    vi.mocked(createWorkflowStepAction).mockRejectedValueOnce(new Error("step failed"));
    vi.mocked(deleteWorkflowAction).mockRejectedValueOnce(new Error("cleanup failed"));
    const { result, setWorkflowItems, setSavedWorkflowItems, toast } = renderCreationHook();

    await act(async () => {
      await result.current.handleCreateWorkflow();
    });

    expect(setWorkflowItems).toHaveBeenCalledOnce();
    expect(setSavedWorkflowItems).toHaveBeenCalledOnce();
    expect(toast).toHaveBeenCalledWith(
      expect.objectContaining({ title: "Workflow created with setup errors", variant: "error" }),
    );
  });
});
