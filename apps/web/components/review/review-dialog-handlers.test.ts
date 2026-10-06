import { describe, expect, it, vi } from "vitest";
import { createReviewToggleHandler } from "./review-dialog-handlers";
import type { ReviewFile } from "./types";

const FILE_PATH = "src/app.ts";
const readyFile: ReviewFile = {
  path: FILE_PATH,
  diff: "@@ -1 +1 @@\n-old\n+new",
  status: "modified",
  additions: 1,
  deletions: 1,
  staged: false,
  source: "uncommitted",
};

describe("createReviewToggleHandler", () => {
  it.each([
    { diff_state: "pending" as const, display_stale: false },
    { diff_state: "unavailable" as const, display_stale: false },
    { diff_state: "ready" as const, display_stale: true },
  ])(
    "does not mark a $diff_state or retained patch as reviewed",
    ({ diff_state, display_stale }) => {
      const markReviewed = vi.fn();
      const markUnreviewed = vi.fn();
      const toggleReviewed = createReviewToggleHandler(
        [{ ...readyFile, diff_state, display_stale }],
        markReviewed,
        markUnreviewed,
      );

      toggleReviewed(FILE_PATH, true);

      expect(markReviewed).not.toHaveBeenCalled();
      expect(markUnreviewed).not.toHaveBeenCalled();
    },
  );

  it("allows marking current ready detail and clearing a review", () => {
    const markReviewed = vi.fn();
    const markUnreviewed = vi.fn();
    const toggleReviewed = createReviewToggleHandler([readyFile], markReviewed, markUnreviewed);

    toggleReviewed(FILE_PATH, true);
    toggleReviewed(FILE_PATH, false);

    expect(markReviewed).toHaveBeenCalledWith(FILE_PATH, expect.any(String));
    expect(markUnreviewed).toHaveBeenCalledWith(FILE_PATH);
  });
});
