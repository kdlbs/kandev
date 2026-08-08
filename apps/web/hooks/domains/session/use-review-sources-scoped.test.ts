import { describe, expect, it } from "vitest";
import { buildReviewSources } from "./use-review-sources";

const CHILD_SCOPE = "vendor/lib";
const README_PATH = "README.md";

describe("buildReviewSources scoped metadata", () => {
  it("preserves submodule metadata on PR-only files", () => {
    const result = buildReviewSources({
      gitStatus: undefined,
      statusByRepo: undefined,
      cumulativeDiff: null,
      prDiffFiles: [
        {
          filename: README_PATH,
          status: "modified",
          patch: "@@ -1 +1 @@\n-old\n+new\n",
          additions: 1,
          deletions: 1,
          is_submodule: true,
        },
      ],
      prRepoName: CHILD_SCOPE,
    });

    expect(result.allFiles).toEqual([
      expect.objectContaining({
        path: README_PATH,
        repository_name: CHILD_SCOPE,
        source: "pr",
        is_submodule: true,
      }),
    ]);
  });

  it("keeps fallback root status distinct from a same-named child file", () => {
    const result = buildReviewSources({
      gitStatus: {
        files: {
          [README_PATH]: {
            diff: "@@root@@",
            status: "modified",
            additions: 1,
            deletions: 0,
          },
        },
      },
      statusByRepo: undefined,
      cumulativeDiff: {
        files: {
          [`${CHILD_SCOPE}\u0000${README_PATH}`]: {
            path: README_PATH,
            repository_name: CHILD_SCOPE,
            diff: "@@child@@",
            status: "modified",
            additions: 1,
            deletions: 0,
          },
        },
      },
      prDiffFiles: undefined,
      useRepositoryKeys: true,
    });

    expect(result.allFiles.map((file) => [file.repository_name, file.path])).toEqual([
      ["", README_PATH],
      [CHILD_SCOPE, README_PATH],
    ]);
  });
});
