import { describe, expect, it } from "vitest";
import { buildReviewSources } from "./use-review-sources";

describe("buildReviewSources mixed facets", () => {
  it("preserves staged and unstaged facets on an uncommitted source", () => {
    const result = buildReviewSources({
      gitStatus: {
        files: {
          "src/mixed.ts": {
            diff: "combined",
            status: "modified",
            staged: false,
            staged_change: {
              status: "modified",
              additions: 1,
              deletions: 0,
              diff: "staged diff",
            },
            unstaged_change: {
              status: "modified",
              additions: 1,
              deletions: 0,
              diff: "unstaged diff",
            },
          } as never,
        },
      },
      statusByRepo: undefined,
      cumulativeDiff: null,
      prDiffFiles: undefined,
    });

    expect(result.allFiles[0]).toMatchObject({
      staged_change: { additions: 1, diff: "staged diff" },
      unstaged_change: { additions: 1, diff: "unstaged diff" },
    });
  });
});
