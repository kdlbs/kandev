import { describe, expect, it } from "vitest";
import { renderHook } from "@testing-library/react";
import { hashDiff, reviewFileKey, type ReviewFile } from "@/components/review/types";
import { useSelectedFileKey } from "./changes-panel-banner";
import {
  computeChangesReviewSets,
  filterVisibleFiles,
  reviewDiffHashForKey,
  useVisibleDiffState,
} from "./task-changes-panel-state";

const MIXED_PATH = "mixed.ts";
const STAGED_DIFF = "staged diff";
const UNSTAGED_DIFF = "unstaged diff";

function mixedFile(): ReviewFile {
  return {
    path: MIXED_PATH,
    source: "uncommitted",
    status: "modified",
    staged: false,
    additions: 2,
    deletions: 0,
    diff: "combined",
    staged_change: {
      status: "modified",
      additions: 1,
      deletions: 0,
      diff: STAGED_DIFF,
    },
    unstaged_change: {
      status: "modified",
      additions: 1,
      deletions: 0,
      diff: UNSTAGED_DIFF,
    },
  };
}

function projectLayer(file: ReviewFile, changeLayer: "staged" | "unstaged"): ReviewFile {
  const [projected] = filterVisibleFiles([file], {
    mode: "file",
    filePath: MIXED_PATH,
    fileRepositoryName: undefined,
    sourceFilter: "uncommitted",
    changeLayer,
  });
  return projected;
}

describe("filterVisibleFiles mixed layers", () => {
  it("projects the requested layer for a mixed uncommitted file", () => {
    expect(projectLayer(mixedFile(), "staged")).toEqual(
      expect.objectContaining({
        diff: STAGED_DIFF,
        staged: true,
        change_layer: "staged",
        additions: 1,
      }),
    );
  });

  it("keeps staged and unstaged review identities independent", () => {
    const mixed = mixedFile();
    const staged = projectLayer(mixed, "staged");
    const unstaged = projectLayer(mixed, "unstaged");

    expect(reviewFileKey(staged)).not.toBe(reviewFileKey(unstaged));
  });

  it("selects the layer-qualified review row", () => {
    const { result } = renderHook(() =>
      useSelectedFileKey("file", MIXED_PATH, undefined, "staged"),
    );

    expect(result.current).toBe(reviewFileKey({ path: MIXED_PATH, change_layer: "staged" }));
  });
});

describe("mixed layer review state", () => {
  it("restores review state against the selected layer diff", () => {
    const mixed = mixedFile();
    const staged = projectLayer(mixed, "staged");
    const stagedKey = reviewFileKey(staged);

    const state = computeChangesReviewSets(
      [mixed],
      new Map([[stagedKey, { reviewed: true, diffHash: hashDiff(staged.diff) }]]),
    );

    expect(state.reviewedFiles).toEqual(new Set([stagedKey]));
    expect(state.staleFiles).toEqual(new Set());
  });

  it("persists the selected layer diff hash", () => {
    const mixed = mixedFile();
    const staged = projectLayer(mixed, "staged");

    expect(reviewDiffHashForKey([mixed], reviewFileKey(staged))).toBe(hashDiff(staged.diff));
  });

  it("keys the selected layer ref by its review identity", () => {
    const mixed = mixedFile();
    const ref = { current: null };
    const fileRefs = new Map([[reviewFileKey(mixed), ref]]);

    const { result } = renderHook(() =>
      useVisibleDiffState({
        allFiles: [mixed],
        rawPRFiles: [],
        mode: "file",
        filePath: MIXED_PATH,
        fileRepositoryName: undefined,
        sourceFilter: "uncommitted",
        prKey: undefined,
        changeLayer: "staged",
        fileRefs,
        reviewedFiles: new Set(),
        staleFiles: new Set(),
      }),
    );
    const [visible] = result.current.visibleFiles;

    expect(result.current.visibleFileRefs.get(reviewFileKey(visible))).toBe(ref);
  });
});
