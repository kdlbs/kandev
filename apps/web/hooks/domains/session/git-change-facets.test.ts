import { describe, expect, it } from "vitest";
import type { FileInfo } from "@/lib/state/slices/session-runtime/types";
import { splitFilesByChangeLayer } from "./git-change-facets";

const MIXED_PATH = "src/mixed.ts";

describe("splitFilesByChangeLayer", () => {
  it("uses independent link metadata for mixed layers", () => {
    const file = {
      path: "link",
      status: "modified" as const,
      staged: false,
      is_symlink: false,
      staged_change: { status: "modified" as const, is_symlink: true },
      unstaged_change: { status: "modified" as const, is_symlink: false },
    };
    const result = splitFilesByChangeLayer([file]);
    expect(result.stagedFiles[0]).toHaveProperty("is_symlink", true);
    expect(result.unstagedFiles[0]).toHaveProperty("is_symlink", false);
  });
  it("projects one mixed raw file into staged and unstaged views", () => {
    const mixed = {
      path: MIXED_PATH,
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
        path: MIXED_PATH,
        staged: true,
        change_layer: "staged",
        additions: 1,
        diff: "staged diff",
      }),
    ]);
    expect(result.unstagedFiles).toEqual([
      expect.objectContaining({
        path: MIXED_PATH,
        staged: false,
        change_layer: "unstaged",
        additions: 1,
        diff: "unstaged diff",
      }),
    ]);
    expect(mixed.diff).toBe("combined");
  });

  it("preserves stale freshness on separately displayed mixed facets", () => {
    const mixed = {
      path: MIXED_PATH,
      status: "modified",
      staged: false,
      diff_state: "pending",
      staged_change: {
        status: "modified",
        additions: 1,
        diff: "old staged patch",
        diff_state: "pending",
        display_stale: true,
      },
      unstaged_change: {
        status: "modified",
        additions: 2,
        diff: "old unstaged patch",
        diff_state: "unavailable",
        display_stale: true,
      },
    } as FileInfo;

    const result = splitFilesByChangeLayer([mixed]);

    expect(result.stagedFiles[0]).toMatchObject({ diff_state: "pending", display_stale: true });
    expect(result.unstagedFiles[0]).toMatchObject({
      diff_state: "unavailable",
      display_stale: true,
    });
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
