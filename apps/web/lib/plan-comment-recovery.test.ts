import { describe, expect, it } from "vitest";
import { planCommentRecovery, planCommentRecoveryDelay } from "./plan-comment-recovery";

describe("plan comment recovery admission", () => {
  it.each(["idle", "running", "retrying", "complete", "waiting_for_plan", "failed"] as const)(
    "does not block empty %s state",
    (status) => {
      expect(planCommentRecovery({ status, pendingCount: 0, failure: "transient" })).toMatchObject({
        isBlocking: false,
        needsAttention: false,
      });
    },
  );
  it("blocks mixed feedback until the remaining identified row is acknowledged", () => {
    expect(
      planCommentRecovery({ status: "retrying", pendingCount: 1, failure: "transient" }),
    ).toMatchObject({ isBlocking: true, needsAttention: false });
  });
  it("caps retries after the three-attempt burst", () => {
    expect([1, 2, 3, 4, 5, 6, 99].map(planCommentRecoveryDelay)).toEqual([
      1000, 2000, 30000, 60000, 120000, 120000, 120000,
    ]);
  });
});
