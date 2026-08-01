import { describe, expect, it } from "vitest";

import { lineInRanges, parseAddedLineRanges } from "./diff-ranges.mjs";

const OLD_A = "--- a/a.tsx";
const NEW_A = "+++ b/a.tsx";

describe("parseAddedLineRanges", () => {
  it("reads a single-line hunk with no explicit count", () => {
    const diff = [OLD_A, NEW_A, "@@ -4 +4 @@", "-old", "+new"].join("\n");
    expect(parseAddedLineRanges(diff).get("a.tsx")).toEqual([[4, 4]]);
  });

  it("reads a multi-line hunk", () => {
    const diff = [OLD_A, NEW_A, "@@ -10,0 +11,3 @@", "+x", "+y", "+z"].join("\n");
    expect(parseAddedLineRanges(diff).get("a.tsx")).toEqual([[11, 13]]);
  });

  it("ignores deletion-only hunks, which add no attributable line", () => {
    const diff = [OLD_A, NEW_A, "@@ -5,2 +4,0 @@", "-gone", "-also"].join("\n");
    expect(parseAddedLineRanges(diff).get("a.tsx")).toEqual([]);
  });

  it("ignores deleted files", () => {
    const diff = ["--- a/gone.tsx", "+++ /dev/null", "@@ -1,2 +0,0 @@", "-a", "-b"].join("\n");
    expect(parseAddedLineRanges(diff).has("gone.tsx")).toBe(false);
  });

  it("keeps hunks separate across several files", () => {
    const diff = [
      "diff --git a/a.tsx b/a.tsx",
      OLD_A,
      NEW_A,
      "@@ -1 +1 @@",
      "+a",
      "diff --git a/b.tsx b/b.tsx",
      "--- a/b.tsx",
      "+++ b/b.tsx",
      "@@ -20,0 +21,2 @@",
      "+b",
      "+c",
    ].join("\n");
    const ranges = parseAddedLineRanges(diff);
    expect(ranges.get("a.tsx")).toEqual([[1, 1]]);
    expect(ranges.get("b.tsx")).toEqual([[21, 22]]);
  });

  it("collects multiple hunks in one file", () => {
    const diff = [OLD_A, NEW_A, "@@ -1 +1 @@", "+a", "@@ -50,0 +51,2 @@", "+b", "+c"].join("\n");
    expect(parseAddedLineRanges(diff).get("a.tsx")).toEqual([
      [1, 1],
      [51, 52],
    ]);
  });

  it("does not mistake a '+++' inside added content for a file header", () => {
    // A line of added source that begins with "+++" arrives as "++++...".
    const diff = [OLD_A, NEW_A, "@@ -1,0 +1,1 @@", "++++ not a header"].join("\n");
    const ranges = parseAddedLineRanges(diff);
    expect([...ranges.keys()]).toEqual(["a.tsx"]);
    expect(ranges.get("a.tsx")).toEqual([[1, 1]]);
  });

  it("returns nothing for an empty diff", () => {
    expect(parseAddedLineRanges("").size).toBe(0);
  });

  it("ignores a pure rename, which emits no headers or hunks", () => {
    // A plain `git mv` must cost nothing: attributing the moved file's existing
    // lines would demand a full migration for a file nobody edited.
    const diff = [
      "diff --git a/old.tsx b/new.tsx",
      "similarity index 100%",
      "rename from old.tsx",
      "rename to new.tsx",
    ].join("\n");
    expect(parseAddedLineRanges(diff).size).toBe(0);
  });

  it("does not attribute a later hunk to the file before a headerless entry", () => {
    // A binary entry emits no `+++`, so without resetting on `diff --git` the
    // previous file would stay selected and absorb the next hunk.
    const diff = [
      "diff --git a/a.tsx b/a.tsx",
      OLD_A,
      NEW_A,
      "@@ -1 +1 @@",
      "+a",
      "diff --git a/img.png b/img.png",
      "Binary files a/img.png and b/img.png differ",
      "@@ -99,0 +99,3 @@",
      "+not really a hunk",
    ].join("\n");
    const ranges = parseAddedLineRanges(diff);
    expect(ranges.get("a.tsx")).toEqual([[1, 1]]);
    expect(ranges.has("img.png")).toBe(false);
  });

  it("keeps a rename-plus-edit scoped to the edited lines only", () => {
    const diff = [
      "diff --git a/old.tsx b/new.tsx",
      "similarity index 92%",
      "rename from old.tsx",
      "rename to new.tsx",
      "--- a/old.tsx",
      "+++ b/new.tsx",
      "@@ -2,0 +3 @@",
      "+const added = 1;",
    ].join("\n");
    expect(parseAddedLineRanges(diff).get("new.tsx")).toEqual([[3, 3]]);
  });
});

describe("lineInRanges", () => {
  const ranges: Array<[number, number]> = [
    [3, 5],
    [10, 10],
  ];

  it("matches inclusive bounds", () => {
    expect(lineInRanges(ranges, 3)).toBe(true);
    expect(lineInRanges(ranges, 5)).toBe(true);
    expect(lineInRanges(ranges, 10)).toBe(true);
  });

  it("rejects lines outside every range", () => {
    expect(lineInRanges(ranges, 2)).toBe(false);
    expect(lineInRanges(ranges, 6)).toBe(false);
    expect(lineInRanges(ranges, 11)).toBe(false);
  });

  it("treats a missing range list as no match", () => {
    // Reached when a modified file produced no attributable added lines.
    expect(lineInRanges(undefined as unknown as Array<[number, number]>, 1)).toBe(false);
  });
});
