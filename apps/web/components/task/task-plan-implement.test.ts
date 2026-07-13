import { describe, expect, it } from "vitest";
import type { TaskPlan } from "@/lib/types/http";
import { getPlanToolbarImplementState } from "./task-plan-implement";

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

describe("getPlanToolbarImplementState", () => {
  it("hides the button when the editor draft is empty", () => {
    expect(getPlanToolbarImplementState({ draftContent: "  \n", plan: null })).toEqual({
      visible: false,
      disabled: false,
    });
  });

  it("enables the button for non-empty unimplemented plan content", () => {
    expect(getPlanToolbarImplementState({ draftContent: "Ship it", plan: plan() })).toEqual({
      visible: true,
      disabled: false,
    });
  });

  it("keeps the button visible and disabled once implementation has started", () => {
    expect(
      getPlanToolbarImplementState({
        draftContent: "Ship it",
        plan: plan({ implementation_started_at: "2026-07-09T12:30:00Z" }),
      }),
    ).toEqual({
      visible: true,
      disabled: true,
      disabledReason: "This plan has already been sent for implementation.",
    });
  });

  it("keeps the disabled marker affordance visible even when the draft is empty", () => {
    expect(
      getPlanToolbarImplementState({
        draftContent: "  \n",
        plan: plan({ implementation_started_at: "2026-07-09T12:30:00Z" }),
      }),
    ).toEqual({
      visible: true,
      disabled: true,
      disabledReason: "This plan has already been sent for implementation.",
    });
  });
});
