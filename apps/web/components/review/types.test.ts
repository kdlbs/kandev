import { describe, expect, it } from "vitest";
import { hasTextualDiff, reviewDiffUnavailableLabel, type ReviewFile } from "./types";

function patchlessFile(overrides: Partial<ReviewFile>): ReviewFile {
  return {
    path: "src/new-name.ts",
    diff: "",
    status: "renamed",
    additions: 0,
    deletions: 0,
    staged: false,
    source: "pr",
    ...overrides,
  };
}

describe("reviewDiffUnavailableLabel", () => {
  it("describes a pure move using its previous path", () => {
    const file = patchlessFile({ old_path: "src/old-name.ts" });
    expect(reviewDiffUnavailableLabel(file)).toBe("Moved from src/old-name.ts; no textual changes");
  });

  it("prefers an explicit diff skip reason over move text", () => {
    const file = patchlessFile({
      old_path: "src/old-name.ts",
      diff_skip_reason: "binary",
    });
    expect(reviewDiffUnavailableLabel(file)).toBe("Binary file — not diffable");
  });

  it.each([
    ["added", "Added file has no textual diff"],
    ["untracked", "Untracked file has no textual diff"],
    ["deleted", "Deleted file has no textual diff"],
  ] as const)("describes a patchless %s file", (status, expected) => {
    const file = patchlessFile({ status, old_path: undefined });
    expect(reviewDiffUnavailableLabel(file)).toBe(expected);
  });
});

describe("hasTextualDiff", () => {
  it.each<[string, Partial<ReviewFile>]>([
    ["empty input", { diff: "", status: "modified" }],
    [
      "pure rename metadata",
      {
        diff: [
          "diff --git a/src/old.ts b/src/new.ts",
          "similarity index 100%",
          "rename from src/old.ts",
          "rename to src/new.ts",
        ].join("\n"),
        status: "renamed",
      },
    ],
    [
      "empty added-file metadata",
      {
        diff: [
          "diff --git a/empty.txt b/empty.txt",
          "new file mode 100644",
          "index 0000000..e69de29",
        ].join("\n"),
        status: "added",
      },
    ],
    [
      "zero-stat rename with a synthetic added-file hunk",
      {
        diff: [
          "diff --git a/src/new.ts b/src/new.ts",
          "new file mode 100644",
          "--- /dev/null",
          "+++ b/src/new.ts",
          "@@ -0,0 +1,2 @@",
          "+first line",
          "+second line",
        ].join("\n"),
        status: "renamed",
        additions: 0,
        deletions: 0,
      },
    ],
    [
      "empty untracked zero-count hunk",
      {
        diff: [
          "diff --git a/empty.txt b/empty.txt",
          "new file mode 100644",
          "--- /dev/null",
          "+++ b/empty.txt",
          "@@ -0,0 +1,0 @@",
        ].join("\n"),
        status: "untracked",
        additions: 0,
        deletions: 0,
      },
    ],
  ])("returns false for %s", (_name, overrides) => {
    expect(hasTextualDiff(patchlessFile(overrides))).toBe(false);
  });

  it.each([
    ["modified file", "modified", 1, 1],
    ["renamed file with a real delta", "renamed", 1, 1],
  ] as const)("returns true for a %s hunk", (_name, status, additions, deletions) => {
    const file = patchlessFile({
      diff: [
        "diff --git a/src/app.ts b/src/app.ts",
        "--- a/src/app.ts",
        "+++ b/src/app.ts",
        "@@ -1 +1 @@",
        "-before",
        "+after",
      ].join("\n"),
      status,
      additions,
      deletions,
    });
    expect(hasTextualDiff(file)).toBe(true);
  });
});
