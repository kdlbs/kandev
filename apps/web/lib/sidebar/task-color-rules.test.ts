import { describe, expect, it } from "vitest";
import type { SidebarTaskColorAutomation } from "@/lib/task-color-automation-settings";
import { resolveAutomaticTaskColor } from "./task-color-rules";

const automation = (rules: SidebarTaskColorAutomation["rules"]): SidebarTaskColorAutomation => ({
  enabled: true,
  rules,
});

const fixed = (
  id: string,
  dimension:
    | "task_state"
    | "priority"
    | "origin"
    | "workflow_step"
    | "repository"
    | "workflow"
    | "executor_profile",
  value: unknown,
  color: "red" | "blue" | "gray" | "cyan",
  enabled = true,
) => ({
  id,
  enabled,
  condition: { dimension, value, label: id },
  output: { kind: "fixed" as const, color },
});

describe("resolveAutomaticTaskColor", () => {
  it("returns the first enabled matching rule", () => {
    const result = resolveAutomaticTaskColor(
      automation([
        fixed("state", "task_state", "TODO", "red"),
        fixed("priority", "priority", "high", "blue"),
      ]),
      { state: "TODO", priority: "high", repositories: [] },
    );
    expect(result?.color).toEqual({ token: "red", className: "bg-red-500" });
    expect(result?.source.ruleId).toBe("state");
  });

  it("matches every supported scalar dimension and normalizes a missing origin to Kanban", () => {
    expect(
      resolveAutomaticTaskColor(automation([fixed("priority", "priority", "critical", "blue")]), {
        priority: "critical",
        repositories: [],
      })?.source.ruleId,
    ).toBe("priority");
    expect(
      resolveAutomaticTaskColor(automation([fixed("origin", "origin", "kanban", "cyan")]), {
        repositories: [],
      })?.source.ruleId,
    ).toBe("origin");
    expect(
      resolveAutomaticTaskColor(
        automation([fixed("executor", "executor_profile", "profile-1", "blue")]),
        { primaryExecutorProfileId: "profile-1", repositories: [] },
      )?.source.ruleId,
    ).toBe("executor");
  });

  it("uses the current workflow-step color and gray for unsupported colors", () => {
    const workflowStepRule = {
      id: "step",
      enabled: true,
      condition: {
        dimension: "workflow_step" as const,
        value: { workspace_id: "workspace-a", step_id: "step-1" },
        label: "Review",
      },
      output: { kind: "workflow_step" as const },
    };
    expect(
      resolveAutomaticTaskColor(automation([workflowStepRule]), {
        workspaceId: "workspace-a",
        workflowStepId: "step-1",
        workflowStepColor: "#0af",
        repositories: [],
      }),
    ).toEqual(
      expect.objectContaining({ color: { token: "custom", style: { backgroundColor: "#0af" } } }),
    );
    expect(
      resolveAutomaticTaskColor(automation([workflowStepRule]), {
        workspaceId: "workspace-a",
        workflowStepId: "step-1",
        workflowStepColor: "not-a-color",
        repositories: [],
      })?.color,
    ).toEqual({ token: "gray", className: "bg-slate-500" });
  });

  it("does not match incomplete rules or missing facts", () => {
    expect(
      resolveAutomaticTaskColor(automation([fixed("missing", "task_state", null, "red", false)]), {
        repositories: [],
      }),
    ).toBeNull();
    expect(
      resolveAutomaticTaskColor(automation([fixed("missing", "task_state", "TODO", "red")]), {
        state: "REVIEW",
        repositories: [],
      }),
    ).toBeNull();
  });
});
