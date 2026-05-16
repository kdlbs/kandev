import { describe, it, expect } from "vitest";
import { parsePatchFiles } from "@pierre/diffs";
import type { FileDiffMetadata, DiffLineAnnotation } from "@pierre/diffs";

import { buildHunkAnnotations, type HunkOutputs } from "./use-diff-viewer-state";
import type { RevertBlockInfo } from "./diff-viewer";
import type { AnnotationMetadata } from "./use-diff-annotation-renderer";

const FILE = "src/foo.ts";
const DIFF_HEADER = [`diff --git a/${FILE} b/${FILE}`, `--- a/${FILE}`, `+++ b/${FILE}`];

function freshOut(): HunkOutputs {
  return {
    result: [] as DiffLineAnnotation<AnnotationMetadata>[],
    lineMap: new Map<string, string>(),
    revertMap: new Map<string, RevertBlockInfo>(),
  };
}

function parseSinglePatch(diff: string): FileDiffMetadata {
  const patches = parsePatchFiles(diff);
  const file = patches[0]?.files[0];
  if (!file) throw new Error("parsePatchFiles returned no files");
  return file;
}

describe("buildHunkAnnotations", () => {
  it("reconstructs revert oldLines by slicing deletionLines via deletionLineIndex", () => {
    // Mixed change: 1 context, 2 deletions, 2 additions, 1 trailing context.
    const diff = [
      ...DIFF_HEADER,
      "@@ -1,5 +1,5 @@",
      " keep",
      "-remove one",
      "-remove two",
      "+add one",
      "+add two",
      " end",
      "",
    ].join("\n");

    const out = freshOut();
    buildHunkAnnotations(parseSinglePatch(diff), out);

    expect(out.revertMap.size).toBe(1);
    const info = [...out.revertMap.values()][0];
    // addStart begins after the leading 1 line of context, so line 2.
    expect(info.addStart).toBe(2);
    expect(info.addCount).toBe(2);
    // The library hands us *line text* via deletionLines + deletionLineIndex;
    // the hunk-walker is responsible for picking the right slice and stripping
    // trailing newlines. If the addressing scheme drifts in a future bump this
    // assertion will fail rather than silently delivering wrong "old lines".
    expect(info.oldLines).toEqual(["remove one", "remove two"]);
  });

  it("emits a hunk-actions annotation anchored at the previous context line", () => {
    const diff = [...DIFF_HEADER, "@@ -1,3 +1,3 @@", " keep", "-old", "+new", " tail", ""].join(
      "\n",
    );

    const out = freshOut();
    buildHunkAnnotations(parseSinglePatch(diff), out);

    expect(out.result).toHaveLength(1);
    const ann = out.result[0];
    // additions side because aLen > 0; anchored on the previous context line (line 1).
    expect(ann.side).toBe("additions");
    expect(ann.lineNumber).toBe(1);
    expect(ann.metadata).toMatchObject({ type: "hunk-actions" });
  });

  it("maps each added/deleted line to its changeBlock id in lineMap", () => {
    const diff = [...DIFF_HEADER, "@@ -1,3 +1,3 @@", " keep", "-old", "+new", " tail", ""].join(
      "\n",
    );

    const out = freshOut();
    buildHunkAnnotations(parseSinglePatch(diff), out);

    const cbId = [...out.revertMap.keys()][0];
    // Line 2 is the modified line on both sides (after the 1-line context prefix).
    expect(out.lineMap.get("additions:2")).toBe(cbId);
    expect(out.lineMap.get("deletions:2")).toBe(cbId);
  });

  it("handles a pure-deletion change (aLen === 0): empty addCount, side='deletions'", () => {
    const diff = [...DIFF_HEADER, "@@ -1,3 +1,2 @@", " keep", "-removed", " tail", ""].join("\n");

    const out = freshOut();
    buildHunkAnnotations(parseSinglePatch(diff), out);

    expect(out.revertMap.size).toBe(1);
    const info = [...out.revertMap.values()][0];
    expect(info.addCount).toBe(0);
    expect(info.oldLines).toEqual(["removed"]);
    expect(out.result[0].side).toBe("deletions");
  });

  it("handles a pure-addition change (dLen === 0): empty oldLines, side='additions'", () => {
    const diff = [...DIFF_HEADER, "@@ -1,2 +1,3 @@", " keep", "+added", " tail", ""].join("\n");

    const out = freshOut();
    buildHunkAnnotations(parseSinglePatch(diff), out);

    expect(out.revertMap.size).toBe(1);
    const info = [...out.revertMap.values()][0];
    expect(info.addCount).toBe(1);
    expect(info.oldLines).toEqual([]);
    expect(out.result[0].side).toBe("additions");
  });
});
