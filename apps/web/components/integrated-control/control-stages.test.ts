import { describe, expect, it } from "vitest";
import type { NativeDashboard } from "@/lib/types/control-native";
import { dashboardStageCounts, taskStage, workflowStepCounts } from "./control-stages";

const dashboard = (overrides: Partial<NativeDashboard> = {}): NativeDashboard => ({
  workspace_id: "workspace-a",
  generated_at: "2026-11-09T12:00:00Z",
  since: "2026-11-08T12:00:00Z",
  basis: "recorded_task_session_queue_transition_state",
  inference_state: "unknown",
  tasks: {},
  sessions: {},
  waiting_input_tasks: 0,
  queued_messages: 0,
  transitions_24h: 0,
  completed_tasks_24h: 0,
  ...overrides,
});

describe("Native task stage derivation", () => {
  it("uses recorded stage type and state without guessing from step names", () => {
    const data = dashboard({
      tasks: { IN_PROGRESS: 99 },
      task_stages: [
        { state: "IN_PROGRESS", step_id: "work", step_name: "Build", stage_type: "work", count: 2 },
        {
          state: "IN_PROGRESS",
          step_id: "approval",
          step_name: "Approve",
          stage_type: "approval",
          count: 1,
        },
        {
          state: "BLOCKED",
          step_id: "review",
          step_name: "Review",
          stage_type: "review",
          count: 3,
        },
        { state: "COMPLETED", step_id: "done", step_name: "Done", stage_type: "custom", count: 4 },
        { state: "FAILED", step_id: "failed", step_name: "Failed", stage_type: "work", count: 5 },
        {
          state: "IN_PROGRESS",
          step_id: "custom",
          step_name: "Review-ish",
          stage_type: "custom",
          count: 6,
        },
      ],
    });

    expect(dashboardStageCounts(data)).toEqual({
      work: 2,
      review: 0,
      held: 4,
      completed: 4,
      unknown: 6,
    });
    expect(workflowStepCounts(data, "unknown")).toEqual([
      { id: "custom", name: "Review-ish", count: 6 },
    ]);
  });

  it("marks cancelled and failed tasks outside the active stages", () => {
    expect(taskStage({ state: "CANCELLED", stage_type: "work" })).toBeNull();
    expect(taskStage({ state: "FAILED", stage_type: "approval" })).toBeNull();
    expect(taskStage({ state: "BLOCKED", stage_type: "approval" })).toBe("held");
  });

  it("uses state counts only when the backend omitted stage groups", () => {
    expect(dashboardStageCounts(dashboard({ tasks: { BLOCKED: 2, COMPLETED: 1 } }))).toEqual({
      work: 0,
      review: 0,
      held: 2,
      completed: 1,
      unknown: 0,
    });
    expect(dashboardStageCounts(dashboard({ tasks: { IN_PROGRESS: 2 }, task_stages: [] }))).toEqual(
      {
        work: 0,
        review: 0,
        held: 0,
        completed: 0,
        unknown: 0,
      },
    );
  });
});
