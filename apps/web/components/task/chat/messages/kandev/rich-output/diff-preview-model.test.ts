import { expect, it } from "vitest";
import { parsePreviewDiff } from "./diff-preview-model";

it("aligns unequal changes and preserves source headers, metadata and CRLF", () => {
  const patch = [
    "--- timestamp A",
    "+++ timestamp B",
    "@@ -1,2 +1 @@",
    "--- old",
    "-extra",
    "+++ new",
    "--- c",
    "+++ d",
    "@@ -0,0 +1 @@",
    "+added",
  ];
  const result = parsePreviewDiff(patch.join("\r\n"));
  expect(result?.lines).toEqual(patch);
  expect(result?.split).toEqual([[0], [1], [2], [3, 5], [4, null], [6], [7], [8], [null, 9]]);
});

it("leaves unsupported text in the ordinary preview", () => {
  expect(parsePreviewDiff("--- plain text")).toBeNull();
});

it("pairs replacements across missing-newline markers and retains blank context", () => {
  const marker = "\\ No newline at end of file";
  const patch = ["@@ -1,2 +1,2 @@", "", "-old", marker, "+new", marker];
  expect(parsePreviewDiff(patch.join("\n"))?.split).toEqual([[0], [1, 1], [2, 4], [3, 5]]);
});
