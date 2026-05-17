import { describe, it, expect } from "vitest";

import {
  formatLineRange,
  normalizeDiffString,
  transformFileMutation,
  transformGitDiff,
  commentsToAnnotations,
  extractCodeFromDiff,
  extractCodeFromContent,
  computeLineDiffStats,
  type FileMutation,
} from "./adapter";
import type { DiffComment } from "./types";

const FILE = "src/foo.ts";
const SIMPLE_PATCH_BODY = "@@ -1,1 +1,1 @@\n-old\n+new";
const DIFF_GIT_HEADER = `diff --git a/${FILE} b/${FILE}`;
const OLD_HEADER = `--- a/${FILE}`;
const NEW_HEADER = `+++ b/${FILE}`;

const baseComment = (overrides: Partial<DiffComment> = {}): DiffComment => ({
  id: "c1",
  sessionId: "s1",
  source: "diff",
  filePath: FILE,
  startLine: 10,
  endLine: 10,
  side: "additions",
  codeContent: "x",
  text: "looks good",
  createdAt: "2026-05-16T00:00:00Z",
  status: "pending",
  ...overrides,
});

describe("formatLineRange", () => {
  it("renders a single-line range as Lx", () => {
    expect(formatLineRange(10, 10)).toBe("L10");
  });

  it("renders a multi-line range as Lx-y", () => {
    expect(formatLineRange(10, 15)).toBe("L10-15");
  });
});

describe("normalizeDiffString", () => {
  it("returns empty string when diff is empty", () => {
    expect(normalizeDiffString("", FILE)).toBe("");
  });

  it("re-emits canonical headers and trailing newline even when input already starts with diff --git", () => {
    const diff = `diff --git a/src/foo.ts b/src/foo.ts
--- a/src/foo.ts
+++ b/src/foo.ts
@@ -1,1 +1,1 @@
-old
+new`;
    // We always re-emit headers + ensure trailing newline so @pierre/diffs
    // never sees malformed input regardless of upstream formatting.
    expect(normalizeDiffString(`  ${diff}  `, FILE)).toBe(diff + "\n");
  });

  it("prepends only the diff --git header when --- and +++ are present", () => {
    const diff = `--- a/src/foo.ts
+++ b/src/foo.ts
@@ -1,1 +1,1 @@
-old
+new`;
    const result = normalizeDiffString(diff, FILE);
    expect(result.startsWith(`${DIFF_GIT_HEADER}\n`)).toBe(true);
    expect(result).toContain(OLD_HEADER);
    expect(result).toContain(NEW_HEADER);
  });

  it("prepends the full header trio when no headers are present", () => {
    const diff = `@@ -1,1 +1,1 @@
-old
+new`;
    const result = normalizeDiffString(diff, FILE);
    const lines = result.split("\n");
    expect(lines[0]).toBe(DIFF_GIT_HEADER);
    expect(lines[1]).toBe(OLD_HEADER);
    expect(lines[2]).toBe(NEW_HEADER);
    expect(lines[3]).toBe("@@ -1,1 +1,1 @@");
  });

  it("emits --- /dev/null and new-file-mode for an `@@ -0,0 +N,M @@` hunk", () => {
    // A new file from GitHub mock looks like this (no headers, just the hunk).
    // @pierre/diffs 1.1.x throws "deletionLine and additionLine are null" if the
    // diff is fed --- a/file / +++ b/file because it gets classified as a
    // modification with no old content to read. The /dev/null marker keeps the
    // renderer in the "new file" code path.
    const diff = `@@ -0,0 +1,2 @@
+line one
+line two`;
    const result = normalizeDiffString(diff, FILE);
    const lines = result.split("\n");
    expect(lines[0]).toBe(DIFF_GIT_HEADER);
    expect(lines[1]).toBe("new file mode 100644");
    expect(lines[2]).toBe("--- /dev/null");
    expect(lines[3]).toBe("+++ b/src/foo.ts");
  });

  it("detects new file from a `new file mode` header even without an @@ hunk", () => {
    // Header-only new-file patches (mode-only / binary / rename-no-delta)
    // need the same /dev/null treatment — otherwise the renderer treats
    // them as modifications and pairs missing old content with the patch.
    const diff = `new file mode 100644
index 0000000..e69de29`;
    const result = normalizeDiffString(diff, FILE);
    const lines = result.split("\n");
    expect(lines[0]).toBe(DIFF_GIT_HEADER);
    expect(lines[1]).toBe("new file mode 100644");
    expect(lines[2]).toBe("--- /dev/null");
    expect(lines[3]).toBe(NEW_HEADER);
  });

  it("strips the trailing header in a header-only rename diff (no @@ body)", () => {
    // 100%-similarity renames have no hunk body, so the *last* header line
    // (`rename to ...`) has no trailing \n after trim. The strip regex
    // needs to see a terminator, which is why we append `\n` before
    // matching — otherwise `rename to ...` leaks into `body` and breaks
    // the reconstructed patch.
    const diff = `diff --git a/old.ts b/${FILE}
similarity index 100%
rename from old.ts
rename to ${FILE}`;
    const result = normalizeDiffString(diff, FILE);
    expect(result).not.toContain("rename to");
    expect(result).not.toContain("similarity index");
    expect(result).not.toContain("a/old.ts");
  });

  it("strips rename headers so the canonical --- / +++ pair survives", () => {
    // `git diff --find-renames` produces `similarity index`, `rename from`,
    // `rename to` lines that our older regex left in place — the duplicated
    // header pair confused @pierre/diffs.
    const diff = `diff --git a/old.ts b/${FILE}
similarity index 95%
rename from old.ts
rename to ${FILE}
index abcdef0..1234567 100644
--- a/old.ts
+++ b/${FILE}
@@ -1,1 +1,1 @@
-old
+new`;
    const result = normalizeDiffString(diff, FILE);
    // No leftover rename / similarity / index header should remain.
    expect(result).not.toContain("similarity index");
    expect(result).not.toContain("rename from");
    expect(result).not.toContain("rename to");
    expect(result).not.toContain("a/old.ts");
    expect(result).toContain(DIFF_GIT_HEADER);
    expect(result).toContain(OLD_HEADER);
    expect(result).toContain(NEW_HEADER);
    expect(result).toContain("@@ -1,1 +1,1 @@");
  });

  it("emits +++ /dev/null and deleted-file-mode for an `@@ -N,M +0,0 @@` hunk", () => {
    const diff = `@@ -1,2 +0,0 @@
-line one
-line two`;
    const result = normalizeDiffString(diff, FILE);
    const lines = result.split("\n");
    expect(lines[0]).toBe(DIFF_GIT_HEADER);
    expect(lines[1]).toBe("deleted file mode 100644");
    expect(lines[2]).toBe("--- a/src/foo.ts");
    expect(lines[3]).toBe("+++ /dev/null");
  });
});

describe("transformFileMutation", () => {
  it("uses old_content and new_content for replace mutations", () => {
    const mutation: FileMutation = {
      type: "replace",
      old_content: "old text",
      new_content: "new text",
    };
    const result = transformFileMutation(FILE, mutation);
    expect(result).toEqual({
      filePath: FILE,
      oldContent: "old text",
      newContent: "new text",
      diff: undefined,
      additions: 0,
      deletions: 0,
    });
  });

  it("falls back to `content` when new_content is missing", () => {
    const mutation: FileMutation = { type: "create", content: "hello world" };
    const result = transformFileMutation(FILE, mutation);
    expect(result.newContent).toBe("hello world");
  });

  it("normalizes an explicit diff string", () => {
    const mutation: FileMutation = {
      type: "patch",
      diff: SIMPLE_PATCH_BODY,
    };
    const result = transformFileMutation(FILE, mutation);
    expect(result.diff).toContain(DIFF_GIT_HEADER);
    expect(result.diff).toContain("--- a/src/foo.ts");
    expect(result.diff).toContain("+++ b/src/foo.ts");
  });

  it("generates an all-additions diff for create with content but no diff", () => {
    const mutation: FileMutation = {
      type: "create",
      content: "line one\nline two",
    };
    const result = transformFileMutation("src/new.ts", mutation);
    expect(result.diff).toBe(
      [
        "diff --git a/src/new.ts b/src/new.ts",
        "new file mode 100644",
        "--- /dev/null",
        "+++ b/src/new.ts",
        "@@ -0,0 +1,2 @@",
        "+line one",
        "+line two",
      ].join("\n"),
    );
  });

  it("returns undefined diff for create without content or diff", () => {
    const mutation: FileMutation = { type: "create" };
    const result = transformFileMutation("src/empty.ts", mutation);
    expect(result.diff).toBeUndefined();
  });

  it("prefers new_path over the supplied filePath when renaming", () => {
    const mutation: FileMutation = {
      type: "rename",
      new_path: "src/renamed.ts",
      diff: "@@ -1,1 +1,1 @@\n-a\n+b",
    };
    const result = transformFileMutation("src/old.ts", mutation);
    expect(result.filePath).toBe("src/renamed.ts");
    expect(result.diff).toContain("a/src/renamed.ts b/src/renamed.ts");
  });

  it("defaults empty content fields to empty strings (not undefined)", () => {
    const result = transformFileMutation(FILE, { type: "delete" });
    expect(result.oldContent).toBe("");
    expect(result.newContent).toBe("");
  });
});

describe("transformGitDiff", () => {
  it("normalizes the diff and zeroes additions/deletions", () => {
    const result = transformGitDiff(FILE, "@@ -1,1 +1,1 @@\n-a\n+b", "M");
    expect(result.filePath).toBe(FILE);
    expect(result.oldContent).toBe("");
    expect(result.newContent).toBe("");
    expect(result.diff).toContain(DIFF_GIT_HEADER);
    expect(result.additions).toBe(0);
    expect(result.deletions).toBe(0);
  });
});

describe("commentsToAnnotations", () => {
  it("anchors each annotation at endLine and preserves side", () => {
    const comments: DiffComment[] = [
      baseComment({ id: "c1", startLine: 10, endLine: 12, side: "additions" }),
      baseComment({ id: "c2", startLine: 5, endLine: 5, side: "deletions" }),
    ];
    const annotations = commentsToAnnotations(comments);
    expect(annotations).toHaveLength(2);
    expect(annotations[0]).toMatchObject({
      side: "additions",
      lineNumber: 12,
      metadata: { isEditing: false },
    });
    expect(annotations[0].metadata.comment.id).toBe("c1");
    expect(annotations[1]).toMatchObject({
      side: "deletions",
      lineNumber: 5,
    });
  });

  it("returns an empty array for empty input", () => {
    expect(commentsToAnnotations([])).toEqual([]);
  });
});

describe("extractCodeFromDiff", () => {
  const diff = `diff --git a/src/foo.ts b/src/foo.ts
--- a/src/foo.ts
+++ b/src/foo.ts
@@ -10,4 +10,5 @@
 context line
-removed line
+added line one
+added line two
 trailing context`;

  it("extracts added lines on the additions side", () => {
    // Hunk starts at +10. Context line is +10, then two additions at +11 and +12.
    const result = extractCodeFromDiff(diff, 11, 12, "additions");
    expect(result).toBe("added line one\nadded line two");
  });

  it("extracts the removed line on the deletions side", () => {
    // Hunk starts at -10. Context line is -10, removed line is -11.
    const result = extractCodeFromDiff(diff, 11, 11, "deletions");
    expect(result).toBe("removed line");
  });

  it("includes context lines that fall within range on the additions side", () => {
    // +10 is the first context line; both addition lines are +11 and +12.
    const result = extractCodeFromDiff(diff, 10, 12, "additions");
    expect(result).toBe("context line\nadded line one\nadded line two");
  });

  it("returns an empty string when the range matches nothing", () => {
    expect(extractCodeFromDiff(diff, 100, 200, "additions")).toBe("");
  });
});

describe("extractCodeFromContent", () => {
  it("extracts a 1-indexed inclusive line range", () => {
    const content = "a\nb\nc\nd\ne";
    expect(extractCodeFromContent(content, 2, 4)).toBe("b\nc\nd");
  });

  it("returns the single line when start equals end", () => {
    const content = "a\nb\nc";
    expect(extractCodeFromContent(content, 2, 2)).toBe("b");
  });
});

describe("computeLineDiffStats", () => {
  it("returns zero stats for identical content", () => {
    expect(computeLineDiffStats("a\nb\nc", "a\nb\nc")).toEqual({
      additions: 0,
      deletions: 0,
    });
  });

  it("counts an appended line as a single addition", () => {
    expect(computeLineDiffStats("a\nb", "a\nb\nc")).toEqual({
      additions: 1,
      deletions: 0,
    });
  });

  it("counts a removed trailing line as a single deletion", () => {
    expect(computeLineDiffStats("a\nb\nc", "a\nb")).toEqual({
      additions: 0,
      deletions: 1,
    });
  });

  it("counts a changed line as one addition and one deletion", () => {
    expect(computeLineDiffStats("a\nb\nc", "a\nX\nc")).toEqual({
      additions: 1,
      deletions: 1,
    });
  });
});
