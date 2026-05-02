import { describe, it, expect } from "vitest";
import { mapPRFilesToChangedFiles } from "./changes-panel-helpers";
import type { PRDiffFile } from "@/lib/types/github";

function diffFile(overrides: Partial<PRDiffFile>): PRDiffFile {
  return {
    filename: "src/app.ts",
    status: "modified",
    additions: 1,
    deletions: 0,
    old_path: undefined,
    ...overrides,
  } as PRDiffFile;
}

describe("mapPRFilesToChangedFiles", () => {
  it("maps GitHub status strings to FileInfo statuses", () => {
    const out = mapPRFilesToChangedFiles([
      diffFile({ filename: "a.ts", status: "added" }),
      diffFile({ filename: "b.ts", status: "removed" }),
      diffFile({ filename: "c.ts", status: "renamed", old_path: "old/c.ts" }),
      diffFile({ filename: "d.ts", status: "modified" }),
      // Anything not in the explicit set should fall through to "modified".
      diffFile({ filename: "e.ts", status: "weird" as PRDiffFile["status"] }),
    ]);
    expect(out.map((f) => f.status)).toEqual([
      "added",
      "deleted",
      "renamed",
      "modified",
      "modified",
    ]);
  });

  it("forwards path, additions, deletions, and old_path", () => {
    const [out] = mapPRFilesToChangedFiles([
      diffFile({
        filename: "src/x.ts",
        status: "renamed",
        additions: 7,
        deletions: 3,
        old_path: "old/x.ts",
      }),
    ]);
    expect(out.path).toBe("src/x.ts");
    expect(out.plus).toBe(7);
    expect(out.minus).toBe(3);
    expect(out.oldPath).toBe("old/x.ts");
  });

  it("stamps repository_name on every row when supplied (multi-repo path)", () => {
    const out = mapPRFilesToChangedFiles(
      [diffFile({ filename: "a.ts" }), diffFile({ filename: "b.ts" })],
      "frontend",
    );
    expect(out.every((f) => f.repository_name === "frontend")).toBe(true);
  });

  it("defaults repository_name to '' when caller omits it (single-repo path)", () => {
    // Empty string is meaningful: PRFilesGroupedList treats one group with
    // empty name as the single-repo case and skips per-repo sub-headers.
    const out = mapPRFilesToChangedFiles([diffFile({ filename: "a.ts" })]);
    expect(out[0].repository_name).toBe("");
  });

  it("returns an empty array for empty input", () => {
    expect(mapPRFilesToChangedFiles([])).toEqual([]);
    expect(mapPRFilesToChangedFiles([], "frontend")).toEqual([]);
  });
});
