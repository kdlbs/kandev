import { describe, it, expect } from "vitest";
import { buildPrByRepoMap, mapPRFilesToChangedFiles } from "./changes-panel-helpers";
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

describe("buildPrByRepoMap", () => {
  it("does not copy a multi-repo TaskPR URL into the empty-key fallback", () => {
    const map = buildPrByRepoMap(
      [
        { pr_url: "https://github.com/o/r/pull/1", repository_id: "id-b" },
        { pr_url: "https://github.com/o/r/pull/2", repository_id: "id-a" },
      ],
      { "id-a": "repo-a", "id-b": "repo-b" },
      undefined,
    );
    expect(map[""]).toBeUndefined();
    expect(map["repo-a"]).toBe("https://github.com/o/r/pull/2");
    expect(map["repo-b"]).toBe("https://github.com/o/r/pull/1");
  });

  it("uses empty-key fallback only for legacy TaskPR rows without repository_id", () => {
    const map = buildPrByRepoMap(
      [{ pr_url: "https://dev.azure.com/o/p/_git/r/pullrequest/9" }],
      {},
      undefined,
    );
    expect(map[""]).toBe("https://dev.azure.com/o/p/_git/r/pullrequest/9");
  });
});

import { firstVisibleSection } from "./changes-panel-helpers";

describe("firstVisibleSection", () => {
  const flags = (over: Partial<Parameters<typeof firstVisibleSection>[0]>) => ({
    hasPRFiles: false,
    hasUnstaged: false,
    hasStaged: false,
    showCommitsList: false,
    ...over,
  });

  it("returns null when nothing is shown", () => {
    expect(firstVisibleSection(flags({}))).toBeNull();
  });

  it("review mode: PR is first when there are no local changes", () => {
    expect(firstVisibleSection(flags({ hasPRFiles: true, showCommitsList: true }))).toBe("pr");
  });

  it("commits is first when it is the only section", () => {
    expect(firstVisibleSection(flags({ showCommitsList: true }))).toBe("commits");
  });

  it("unstaged wins over staged and commits", () => {
    expect(
      firstVisibleSection(flags({ hasUnstaged: true, hasStaged: true, showCommitsList: true })),
    ).toBe("unstaged");
  });

  it("staged is first when there is no unstaged", () => {
    expect(firstVisibleSection(flags({ hasStaged: true, showCommitsList: true }))).toBe("staged");
  });

  it("hybrid: local changes win over a PR (PR is not first)", () => {
    expect(
      firstVisibleSection(flags({ hasPRFiles: true, hasUnstaged: true, showCommitsList: true })),
    ).toBe("unstaged");
    expect(
      firstVisibleSection(flags({ hasPRFiles: true, hasStaged: true, showCommitsList: true })),
    ).toBe("staged");
  });
});
