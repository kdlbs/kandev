import { describe, expect, it } from "vitest";
import { djb2Hash } from "./hash";

/**
 * Shared parity fixtures. The identical table lives in
 * `apps/backend/internal/utility/hash/djb2_test.go`: the backend stamps
 * `task_review_findings.file_diff_hash` and the client recomputes it to decide
 * whether a diff moved under a review finding, so both implementations must
 * agree exactly. Changing one without the other breaks stale detection.
 */
const fixtures: Array<{ name: string; input: string; want: string }> = [
  { name: "empty", input: "", want: "1505" },
  { name: "single ascii char", input: "a", want: "2b606" },
  { name: "short ascii", input: "abc", want: "b885c8b" },
  {
    name: "unified diff body",
    input: "diff --git a/main.go b/main.go\n@@ -1,3 +1,4 @@\n+added line\n",
    want: "8c797eb",
  },
  { name: "latin-1 supplement", input: "café naïve über", want: "e4f5cae6" },
  { name: "astral plane emoji", input: "🚀 rocket", want: "9be64c4a" },
  { name: "cjk", input: "你好世界", want: "a96ad5c4" },
  { name: "long ascii", input: "The quick brown fox jumps over the lazy dog", want: "34cc38de" },
];

describe("djb2Hash", () => {
  it.each(fixtures)("matches the Go implementation for $name", ({ input, want }) => {
    expect(djb2Hash(input)).toBe(want);
  });

  it("distinguishes inputs differing only by trailing whitespace", () => {
    const input = "diff --git a/x b/x\n-old\n+new\n";
    expect(djb2Hash(input)).not.toBe(djb2Hash(`${input} `));
  });
});
