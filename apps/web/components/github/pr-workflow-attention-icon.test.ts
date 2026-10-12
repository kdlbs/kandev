import { describe, expect, it } from "vitest";
import { getPRStatusColor, isPRReadyToMerge } from "./pr-task-icon";
import { makeTestPR as makePR } from "./pr-status-chip.test-fixtures";

const YELLOW_500 = "text-yellow-500";

describe("workflow attention task icon", () => {
  const attention = {
    state: "approval_required" as const,
    head_sha: "head-1",
    observed_at: "2026-09-10T10:00:00Z",
    stale: true,
    runs: [],
  };

  it("blocks readiness while current-head workflow attention is active", () => {
    expect(
      isPRReadyToMerge(
        makePR({
          head_sha: "head-1",
          review_state: "approved",
          checks_state: "success",
          mergeable_state: "clean",
          workflow_attention: attention,
        }),
      ),
    ).toBe(false);
  });

  it("uses a warning color instead of ready green", () => {
    expect(
      getPRStatusColor(
        makePR({
          head_sha: "head-1",
          review_state: "approved",
          checks_state: "success",
          mergeable_state: "clean",
          workflow_attention: { ...attention, stale: false },
        }),
      ),
    ).toBe(YELLOW_500);
  });
});
