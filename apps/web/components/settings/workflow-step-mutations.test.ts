import { describe, expect, it } from "vitest";

import type { WorkflowStep } from "@/lib/types/http";
import {
  addWorkflowAction,
  moveWorkflowAction,
  removeWorkflowAction,
  updateWorkflowAction,
} from "./workflow-step-mutations";

const baseStep = {
  id: "step-1",
  workflow_id: "workflow-1",
  name: "Review",
  position: 0,
  color: "bg-slate-500",
  created_at: "",
  updated_at: "",
} as WorkflowStep;

describe("workflow action mutations", () => {
  it("adds an action without changing the source step", () => {
    const source: WorkflowStep = {
      ...baseStep,
      events: { on_enter: [{ type: "auto_start_agent" }] },
    };
    const next = addWorkflowAction(source, "on_enter", {
      type: "run_script",
      config: { command: "echo ready" },
    });

    expect(source.events?.on_enter).toHaveLength(1);
    expect(next.events?.on_enter).toHaveLength(2);
    expect(next.events?.on_enter?.[1]).toEqual({
      type: "run_script",
      config: { command: "echo ready" },
    });
  });

  it("updates only the selected action and merges its config", () => {
    const source: WorkflowStep = {
      ...baseStep,
      events: {
        on_exit: [
          { type: "run_script", config: { command: "echo one", timeout_seconds: 10 } },
          { type: "disable_plan_mode" },
        ],
      },
    };
    const next = updateWorkflowAction(source, "on_exit", 0, {
      config: { timeout_seconds: 20 },
    });
    expect(next.events?.on_exit?.[0]).toEqual({
      type: "run_script",
      config: { command: "echo one", timeout_seconds: 20 },
    });
    expect(next.events?.on_exit?.[1]).toEqual({ type: "disable_plan_mode" });
  });

  it("removes and reorders actions while preserving action order per trigger", () => {
    const source: WorkflowStep = {
      ...baseStep,
      events: {
        on_enter: [
          { type: "auto_start_agent" },
          { type: "reset_agent_context" },
          { type: "enable_plan_mode" },
        ],
        on_exit: [{ type: "disable_plan_mode" }],
      },
    };
    const moved = moveWorkflowAction(source, "on_enter", 0, 2);
    const removed = removeWorkflowAction(moved, "on_enter", 1);
    expect(removed.events?.on_enter).toEqual([
      { type: "reset_agent_context" },
      { type: "auto_start_agent" },
    ]);
    expect(removed.events?.on_exit).toEqual([{ type: "disable_plan_mode" }]);
  });
});
