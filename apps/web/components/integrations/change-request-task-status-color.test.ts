import { describe, expect, it } from "vitest";
import type { ReviewTaskStatus } from "@/lib/plugins/types";
import {
  aggregateReviewTaskStatusColor,
  CHANGE_REQUEST_STATUS_COLORS,
} from "./change-request-task-status-color";

function status(
  state: ReviewTaskStatus["state"],
  pipelineState: ReviewTaskStatus["pipelineState"] = "neutral",
): ReviewTaskStatus {
  return { number: 1, state, pipelineState, checks: [] };
}

describe("aggregateReviewTaskStatusColor", () => {
  it("keeps an all-merged task purple", () => {
    expect(aggregateReviewTaskStatusColor([status("merged")])).toBe(
      CHANGE_REQUEST_STATUS_COLORS.merged,
    );
  });

  it("prefers an active pull request over a merged sibling", () => {
    expect(aggregateReviewTaskStatusColor([status("merged"), status("open", "failure")])).toBe(
      CHANGE_REQUEST_STATUS_COLORS.danger,
    );
  });
});
