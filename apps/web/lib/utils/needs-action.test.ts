import { describe, expect, it } from "vitest";
import { needsAction } from "./needs-action";

describe("needsAction", () => {
  it("uses the task status summary when legacy pending data is stale", () => {
    expect(
      needsAction({
        state: "REVIEW",
        taskPendingAction: "clarification",
        statusSummary: { pending_action: "permission" },
      }),
    ).toBe(true);
  });

  it("recognizes a pending action when the task remains in REVIEW", () => {
    expect(
      needsAction({
        state: "REVIEW",
        statusSummary: { pending_action: "clarification" },
      }),
    ).toBe(true);
  });

  it("does not revive a legacy action after the status summary clears it", () => {
    expect(
      needsAction({
        state: "REVIEW",
        taskPendingAction: "clarification",
        statusSummary: {},
      }),
    ).toBe(false);
  });
});
