import { act, renderHook } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { Workflow, WorkflowTemplate, Workspace } from "@/lib/types/http";
import { useWorkflowCreation } from "./use-workflow-creation";

const workspace = { id: "workspace-1", name: "Workspace" } as Workspace;
const template = {
  id: "template-1",
  name: "Template",
  description: "Template description",
  default_steps: [{ name: "Template Step", position: 0, color: "bg-blue-500" }],
} as WorkflowTemplate;

function renderCreationHook(workflowTemplates: WorkflowTemplate[] = []) {
  let workflows: Workflow[] = [];
  const setWorkflowItems = vi.fn((update: React.SetStateAction<Workflow[]>) => {
    workflows = typeof update === "function" ? update(workflows) : update;
  });
  const view = renderHook(() =>
    useWorkflowCreation({ workspace, workflowTemplates, setWorkflowItems }),
  );
  return { ...view, setWorkflowItems, getWorkflows: () => workflows };
}

beforeEach(() => {
  vi.clearAllMocks();
  vi.spyOn(crypto, "randomUUID").mockReturnValue("00000000-0000-4000-8000-000000000001");
});

describe("useWorkflowCreation", () => {
  it("creates a custom workflow and default steps locally", () => {
    const { result, getWorkflows } = renderCreationHook();

    act(() => {
      result.current.setNewWorkflowName("Custom Workflow");
      result.current.setSelectedTemplateId(null);
    });
    act(() => result.current.handleCreateWorkflow());

    const [workflow] = getWorkflows();
    expect(workflow).toMatchObject({ name: "Custom Workflow" });
    expect(workflow.id).toMatch(/^temp-workflow-/);
    expect(result.current.initialStepsByWorkflowId.get(workflow.id)).toHaveLength(4);
  });

  it("uses template fields without persisting from the dialog", () => {
    const { result, getWorkflows } = renderCreationHook([template]);

    act(() => result.current.setSelectedTemplateId(template.id));
    act(() => result.current.handleCreateWorkflow());

    const [workflow] = getWorkflows();
    expect(workflow).toMatchObject({
      name: "Template",
      description: "Template description",
      workflow_template_id: template.id,
    });
    expect(result.current.initialStepsByWorkflowId.get(workflow.id)?.[0]).toMatchObject({
      name: "Template Step",
      color: "bg-blue-500",
    });
  });
});
