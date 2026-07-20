import { describe, expect, it } from "vitest";
import type { Workflow, WorkflowStep } from "@/lib/types/http";
import {
  isWorkflowFieldDirty,
  isWorkflowStepDirty,
  isWorkflowStepValueDirty,
} from "./workflow-dirty-state";

const workflow = {
  id: "workflow-1",
  workspace_id: "workspace-1",
  name: "Workflow",
  description: "",
  created_at: "",
  updated_at: "",
} as Workflow;

const step = {
  id: "step-1",
  workflow_id: workflow.id,
  name: "Todo",
  color: "bg-slate-500",
  position: 0,
  allow_manual_move: true,
  created_at: "",
  updated_at: "",
} as WorkflowStep;

describe("workflow dirty state", () => {
  it("marks fields on a new workflow dirty", () => {
    expect(isWorkflowFieldDirty(workflow, undefined, "name")).toBe(true);
  });

  it("marks only a changed workflow field dirty", () => {
    const draft = { ...workflow, name: "Renamed" };

    expect(isWorkflowFieldDirty(draft, workflow, "name")).toBe(true);
    expect(isWorkflowFieldDirty(draft, workflow, "agent_profile_id")).toBe(false);
  });

  it("marks new and changed steps dirty", () => {
    expect(isWorkflowStepDirty(step, undefined)).toBe(true);
    expect(isWorkflowStepDirty({ ...step, name: "Doing" }, step)).toBe(true);
    expect(isWorkflowStepDirty(step, step)).toBe(false);
  });

  it("compares only the selected step value", () => {
    const draft = { ...step, name: "Doing", wip_limit: 2 };

    expect(isWorkflowStepValueDirty(draft, step, (item) => item.name)).toBe(true);
    expect(isWorkflowStepValueDirty(draft, step, (item) => item.color)).toBe(false);
  });
});
