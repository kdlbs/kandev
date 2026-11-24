import { describe, expect, it } from "vitest";
import type { Automation, AutomationRun, RunStatus } from "@/lib/types/automation";
import { automationState } from "./automation-rows";
import { automationRunPresentation } from "./automation-run-presentation";

const CREATED_AT = "2026-11-05T10:00:00Z";

function run(status: RunStatus): AutomationRun {
  return {
    id: "newest",
    automation_id: "a",
    trigger_id: "t",
    trigger_type: "manual",
    task_id: "",
    status,
    dedup_key: "",
    trigger_data: {},
    error_message: "",
    created_at: CREATED_AT,
  };
}

// @covers AC-OFFICE-AUTOMATION-RUNS-001.19 AC-OFFICE-AUTOMATION-RUNS-001.20
// @covers AC-OFFICE-AUTOMATION-RUNS-001.21
describe("automationRunPresentation", () => {
  it("failure is not idle success", () => {
    const result = automationRunPresentation("idle", run("failed"));
    expect(result.dotClass).toBe("bg-red-500");
    expect(result.outcomeLabelKey).toBe("automations:runFailed");
    expect(result.showOutcome).toBe(true);
    expect(result.labelKeys).toEqual(["automations:runFailed"]);
  });
  it("running wins over a newer skipped firing", () => {
    const result = automationRunPresentation("running", run("skipped"));
    expect(result.dotClass).toBe("bg-blue-500");
    expect(result.labelKeys).toEqual(["automations:stateRunning", "automations:runSkipped"]);
    expect(result.showOutcome).toBe(true);
  });
  it.each([false, true])(
    "disabled automation and disabled schedule retain paused color (enabled=%s)",
    (enabled) => {
      const automation: Automation = {
        id: "a",
        workspace_id: "ws",
        name: "Schedule",
        description: "",
        workflow_id: "",
        workflow_step_id: "",
        agent_profile_id: "",
        executor_profile_id: "",
        repository_ids: [],
        prompt: "",
        task_title_template: "",
        enabled,
        max_concurrent_runs: 1,
        last_triggered_at: null,
        created_at: CREATED_AT,
        updated_at: CREATED_AT,
        triggers: [
          {
            id: "t",
            automation_id: "a",
            type: "scheduled",
            enabled: false,
            config: { cron_expression: "0 0 * * *" },
            last_evaluated_at: null,
            created_at: CREATED_AT,
            updated_at: CREATED_AT,
          },
        ],
      };
      const state = automationState(automation, 0);
      const result = automationRunPresentation(state, run("failed"));
      expect(result.dotClass).toBe("bg-muted-foreground/50");
      expect(result.labelKeys).toEqual(["automations:statePaused", "automations:runFailed"]);
      expect(result.showOutcome).toBe(true);
    },
  );
  it("a newer success clears failure", () => {
    expect(automationRunPresentation("idle", run("failed")).showOutcome).toBe(true);
    const recovered = automationRunPresentation("idle", run("succeeded"));
    expect(recovered.dotClass).toBe("bg-emerald-500");
    expect(recovered.showOutcome).toBe(false);
    expect(recovered.labelKeys).toEqual(["automations:runSucceeded"]);
  });
  it.each(["idle", "paused"] as const)("never-run %s has no outcome metadata", (state) => {
    const result = automationRunPresentation(state, null);
    expect(result.outcomeLabelKey).toBeNull();
    expect(result.showOutcome).toBe(false);
    expect(result.labelKeys).toEqual([
      state === "idle" ? "automations:stateIdle" : "automations:statePaused",
    ]);
  });
  it.each(["triggered", "task_created"] as const)(
    "open newest %s is announced only once",
    (status) => {
      const result = automationRunPresentation("running", run(status));
      expect(result.showOutcome).toBe(false);
      expect(result.labelKeys).toEqual(["automations:stateRunning"]);
    },
  );
  it.each([
    "retry_scheduling_failed",
    "cancelled",
    "scheduled_retry",
    "skipped",
    "archived",
  ] as const)("%s remains visible rather than being called idle", (status) => {
    const presentation = automationRunPresentation("idle", run(status));
    expect(presentation.showOutcome).toBe(true);
    expect(presentation.labelKeys).not.toContain("automations:stateIdle");
  });
});
