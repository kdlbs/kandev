import { describe, expect, it } from "vitest";
import type { FileInfo } from "@/lib/state/slices/session-runtime/types";
import { splitFilesByChangeLayer } from "./git-change-facets";

describe("splitFilesByChangeLayer", () => {
  it("projects one mixed raw file into staged and unstaged views", () => {
    const mixed = {
      path: "src/mixed.ts",
      status: "modified",
      staged: false,
      additions: 2,
      diff: "combined",
      staged_change: {
        status: "modified",
        additions: 1,
        deletions: 0,
        diff: "staged diff",
      },
      unstaged_change: {
        status: "modified",
        additions: 1,
        deletions: 0,
        diff: "unstaged diff",
      },
    } as FileInfo;

    const result = splitFilesByChangeLayer([mixed]);

    expect(result.stagedFiles).toEqual([
      expect.objectContaining({
        path: "src/mixed.ts",
        staged: true,
        change_layer: "staged",
        additions: 1,
        diff: "staged diff",
      }),
    ]);
    expect(result.unstagedFiles).toEqual([
      expect.objectContaining({
        path: "src/mixed.ts",
        staged: false,
        change_layer: "unstaged",
        additions: 1,
        diff: "unstaged diff",
      }),
    ]);
    expect(mixed.diff).toBe("combined");
  });

  it("keeps legacy files in exactly one section", () => {
    const staged = {
      path: "staged.ts",
      status: "added",
      staged: true,
    } as FileInfo;
    const unstaged = {
      path: "unstaged.ts",
      status: "modified",
      staged: false,
    } as FileInfo;

    expect(splitFilesByChangeLayer([staged, unstaged])).toEqual({
      stagedFiles: [staged],
      unstagedFiles: [unstaged],
    });
  });
});
