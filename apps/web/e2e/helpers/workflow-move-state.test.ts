import { describe, expect, it } from "vitest";
import { hasPendingWorkflowMove } from "./workflow-move-state";

describe("manual workflow move lifecycle state", () => {
  it("waits while a move has only a pending marker", () => {
    expect(
      hasPendingWorkflowMove({ manual_move_lifecycle_pending: { from_step_id: "source" } }),
    ).toBe(true);
  });

  it("accepts durable completion while pending-marker cleanup has not finished", () => {
    expect(
      hasPendingWorkflowMove({
        manual_move_lifecycle_pending: { from_step_id: "source" },
        manual_move_lifecycle_completed: true,
      }),
    ).toBe(false);
  });

  it("accepts a completed-only marker or fully cleared lifecycle", () => {
    expect(hasPendingWorkflowMove({ manual_move_lifecycle_completed: true })).toBe(false);
    expect(hasPendingWorkflowMove({})).toBe(false);
  });
});
