import { describe, expect, it } from "vitest";
import {
  buildWalkthroughRangeDecorations,
  clampWalkthroughRangeToLineCount,
  getWalkthroughEditorRange,
} from "./use-monaco-walkthrough-range";

describe("getWalkthroughEditorRange", () => {
  it("returns the active walkthrough range for a matching editor file", () => {
    expect(
      getWalkthroughEditorRange(
        { path: "/tmp/worktree/src/app.ts" },
        { file: "src/app.ts", line: 8, line_end: 10, text: "Explain this" },
      ),
    ).toEqual({ startLine: 8, endLine: 10 });
  });

  it("returns null for a different file", () => {
    expect(
      getWalkthroughEditorRange(
        { path: "src/other.ts" },
        { file: "src/app.ts", line: 8, text: "Explain this" },
      ),
    ).toBeNull();
  });
});

describe("buildWalkthroughRangeDecorations", () => {
  it("builds a decoration for every line in the walkthrough range", () => {
    const decorations = buildWalkthroughRangeDecorations({ startLine: 2, endLine: 3 });

    expect(decorations).toHaveLength(2);
    expect(decorations[0].range).toMatchObject({ startLineNumber: 2, endLineNumber: 2 });
    expect(decorations[1].range).toMatchObject({ startLineNumber: 3, endLineNumber: 3 });
    expect(decorations[0].options.className).toBe("monaco-walkthrough-line");
  });
});

describe("clampWalkthroughRangeToLineCount", () => {
  it("clamps stale walkthrough ranges to the current Monaco model line count", () => {
    expect(clampWalkthroughRangeToLineCount({ startLine: 20, endLine: 24 }, 12)).toEqual({
      startLine: 12,
      endLine: 12,
    });
  });
});
