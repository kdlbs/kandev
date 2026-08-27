import { describe, expect, it } from "vitest";
import type { ReviewFile } from "@/components/review/types";
import { filterVisibleFiles } from "./task-changes-panel-state";

describe("filterVisibleFiles mixed layers", () => {
  it("projects the requested layer for a mixed uncommitted file", () => {
    const mixed = {
      path: "mixed.ts",
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
        diff: "staged diff",
      },
      unstaged_change: {
        status: "modified",
        additions: 1,
        deletions: 0,
        diff: "unstaged diff",
      },
    } as ReviewFile;
    const options = {
      mode: "file",
      filePath: "mixed.ts",
      fileRepositoryName: undefined,
      sourceFilter: "uncommitted",
      changeLayer: "staged",
    } as Parameters<typeof filterVisibleFiles>[1];

    expect(filterVisibleFiles([mixed], options)).toEqual([
      expect.objectContaining({
        diff: "staged diff",
        staged: true,
        change_layer: "staged",
        additions: 1,
      }),
    ]);
  });
});
