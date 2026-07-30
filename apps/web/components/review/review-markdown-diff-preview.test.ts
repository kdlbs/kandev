import { describe, expect, it } from "vitest";
import type { ReviewFile } from "./types";
import { extractReviewMarkdownPreview } from "./review-markdown-diff-preview";

function reviewFile(
  overrides: Partial<ReviewFile>,
): Pick<ReviewFile, "diff" | "status" | "diff_skip_reason"> {
  return {
    diff: "",
    status: "modified",
    ...overrides,
  };
}

describe("extractReviewMarkdownPreview", () => {
  it("renders a complete added Markdown file as one document", () => {
    const preview = extractReviewMarkdownPreview(
      reviewFile({
        status: "added",
        diff: [
          "diff --git a/docs/guide.md b/docs/guide.md",
          "new file mode 100644",
          "--- /dev/null",
          "+++ b/docs/guide.md",
          "@@ -0,0 +1,4 @@",
          "+# Guide",
          "+",
          "+Use the new flow.",
          "+",
        ].join("\n"),
      }),
    );

    expect(preview).toEqual({
      fragments: ["# Guide\n\nUse the new flow.\n"],
      isComplete: true,
      isPartial: false,
    });
  });

  it("keeps modified Markdown hunks separate and removes deleted lines", () => {
    const preview = extractReviewMarkdownPreview(
      reviewFile({
        diff: [
          "diff --git a/docs/guide.md b/docs/guide.md",
          "--- a/docs/guide.md",
          "+++ b/docs/guide.md",
          "@@ -1,3 +1,3 @@",
          " # Guide",
          "-Old introduction.",
          "+New introduction.",
          "@@ -20,3 +20,3 @@",
          " ## Usage",
          "-Run old-command.",
          "+Run new-command.",
          "\\ No newline at end of file",
        ].join("\n"),
      }),
    );

    expect(preview).toEqual({
      fragments: ["# Guide\nNew introduction.", "## Usage\nRun new-command."],
      isComplete: false,
      isPartial: true,
    });
  });

  it("marks truncated renderable content as partial", () => {
    const preview = extractReviewMarkdownPreview(
      reviewFile({
        status: "added",
        diff_skip_reason: "truncated",
        diff: "@@ -0,0 +1,1 @@\n+# Partial guide",
      }),
    );

    expect(preview).toEqual({
      fragments: ["# Partial guide"],
      isComplete: false,
      isPartial: true,
    });
  });

  it("does not expose a preview for a deleted file with no new-side Markdown", () => {
    const preview = extractReviewMarkdownPreview(
      reviewFile({
        status: "deleted",
        diff: "@@ -1,2 +0,0 @@\n-# Removed\n-Old content",
      }),
    );

    expect(preview.fragments).toEqual([]);
  });
});
