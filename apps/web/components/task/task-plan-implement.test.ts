import { describe, expect, it } from "vitest";
import type { TaskPlan } from "@/lib/types/http";
import { shouldShowPlanToolbarImplement } from "./task-plan-implement";

function plan(overrides: Partial<TaskPlan> = {}): TaskPlan {
  return {
    id: "plan-1",
    task_id: "task-1",
    title: "Plan",
    content: "Persisted plan",
    created_by: "user",
    created_at: "2026-07-09T12:00:00Z",
    updated_at: "2026-07-09T12:00:00Z",
    ...overrides,
  };
}

describe("shouldShowPlanToolbarImplement", () => {
  it("hides the button when the editor draft is empty", () => {
    expect(shouldShowPlanToolbarImplement({ draftContent: "  \n", plan: null })).toBe(false);
  });

  it("shows the button for non-empty unimplemented plan content", () => {
    expect(shouldShowPlanToolbarImplement({ draftContent: "Ship it", plan: plan() })).toBe(true);
  });

  it("hides the button once implementation has started for the task plan", () => {
    expect(
      shouldShowPlanToolbarImplement({
        draftContent: "Ship it",
        plan: plan({ implementation_started_at: "2026-07-09T12:30:00Z" }),
      }),
    ).toBe(false);
  });
});
