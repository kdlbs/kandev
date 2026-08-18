import { act, renderHook } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { Workflow, WorkflowStep, WorkflowTemplate, Workspace } from "@/lib/types/http";
import { createDraftWorkflowSteps, useWorkflowCreation } from "./use-workflow-creation";

const workspace = { id: "workspace-1", name: "Workspace" } as Workspace;
const template = {
  id: "template-1",
  name: "Template",
  description: "Template description",
  default_steps: [
    {
      name: "Template Step",
      position: 0,
      color: "bg-blue-500",
      cancel_triggers_turn_complete: true,
    },
  ],
} as WorkflowTemplate;

function renderCreationHook(
  workflowTemplates: WorkflowTemplate[] = [],
  initialWorkflows: Workflow[] = [],
) {
  let workflows: Workflow[] = initialWorkflows;
  const setWorkflowItems = vi.fn((update: React.SetStateAction<Workflow[]>) => {
    workflows = typeof update === "function" ? update(workflows) : update;
  });
  const view = renderHook(() =>
    useWorkflowCreation({
      workspace,
      workflowItems: workflows,
      workflowTemplates,
      setWorkflowItems,
    }),
  );
  return { ...view, setWorkflowItems, getWorkflows: () => workflows };
}

beforeEach(() => {
  vi.clearAllMocks();
  vi.spyOn(crypto, "randomUUID").mockReturnValue("00000000-0000-4000-8000-000000000001");
});

afterEach(() => {
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

describe("useWorkflowCreation", () => {
  it("remaps template transition references to client step identities", () => {
    const steps = createDraftWorkflowSteps("temp-workflow-1", [
      {
        id: "todo",
        name: "Todo",
        position: 0,
        events: {
          on_turn_complete: [{ type: "move_to_step", config: { step_id: "done" } }],
        },
      },
      { id: "done", name: "Done", position: 1, pull_from_step_id: "todo" },
    ]);

    expect(steps[0].events).toEqual({
      on_turn_complete: [
        {
          type: "move_to_step",
          config: { step_id: "temp-template-step-temp-workflow-1-1" },
        },
      ],
    });
    expect(steps[1].pull_from_step_id).toBe("temp-template-step-temp-workflow-1-0");
  });

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

  it("creates a workflow when crypto.randomUUID is unavailable", () => {
    vi.stubGlobal("crypto", {});
    const { result, getWorkflows } = renderCreationHook();

    act(() => {
      result.current.setNewWorkflowName("HTTP Workflow");
      result.current.setSelectedTemplateId(null);
    });
    act(() => result.current.handleCreateWorkflow());

    const [workflow] = getWorkflows();
    expect(workflow.id).toMatch(/^temp-workflow-[0-9a-f-]{36}$/);
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
      cancel_triggers_turn_complete: true,
    });
  });
});

describe("useWorkflowCreation duplication", () => {
  it("inserts a duplicated workflow after its source and keeps its steps local", () => {
    const source = {
      id: "workflow-source",
      workspace_id: workspace.id,
      name: "Source Workflow",
      description: "Source description",
      created_at: "saved",
      updated_at: "saved",
    } as Workflow;
    const sourceSteps = [
      {
        id: "step-source",
        workflow_id: source.id,
        name: "Source Step",
        position: 0,
        color: "bg-blue-500",
        created_at: "saved",
        updated_at: "saved",
      },
    ] as WorkflowStep[];
    const { result, getWorkflows } = renderCreationHook([], [source]);
    const duplicate = (
      result.current as typeof result.current & {
        handleDuplicateWorkflow: (workflow: Workflow, steps: WorkflowStep[]) => void;
      }
    ).handleDuplicateWorkflow;

    expect(duplicate).toEqual(expect.any(Function));
    if (typeof duplicate !== "function") return;

    act(() => duplicate(source, sourceSteps));

    expect(getWorkflows().map((workflow) => workflow.name)).toEqual([
      "Source Workflow",
      "Source Workflow (copy)",
    ]);
    const copied = getWorkflows()[1];
    expect(copied.id).toMatch(/^temp-workflow-/);
    expect(result.current.initialStepsByWorkflowId.get(copied.id)).toEqual([
      expect.objectContaining({ name: "Source Step", workflow_id: copied.id }),
    ]);
  });
});
