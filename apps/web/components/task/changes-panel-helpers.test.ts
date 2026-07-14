import { describe, it, expect } from "vitest";
import {
  buildPrByRepoMap,
  computeReviewProgress,
  mapPRFilesToChangedFiles,
} from "./changes-panel-helpers";
import { reviewFileKey } from "@/components/review/types";
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
      diffFile({ filename: "e.ts", status: "copied" }),
      diffFile({ filename: "f.ts", status: "changed" }),
      diffFile({ filename: "g.ts", status: "unchanged" }),
      diffFile({ filename: "h.ts", status: "weird" as PRDiffFile["status"] }),
    ]);
    expect(out.map((f) => f.status)).toEqual([
      "added",
      "deleted",
      "renamed",
      "modified",
      "modified",
      "modified",
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

describe("computeReviewProgress", () => {
  it("keeps bare uncommitted precedence over a stamped cumulative file at the same path", () => {
    const path = "src/renamed.ts";
    const progress = computeReviewProgress(
      [
        {
          path,
          status: "renamed",
          staged: false,
          additions: 0,
          deletions: 0,
          old_path: "src/old.ts",
          diff: "",
        },
      ],
      {
        files: {
          [`frontend\u0000${path}`]: {
            path,
            repository_name: "frontend",
            diff: "@@ -1 +1 @@\n-old\n+new",
          },
        },
      },
      new Map([[path, { reviewed: true }]]),
    );

    expect(progress).toEqual({ reviewedCount: 1, totalFileCount: 1 });
  });

  it("keeps same-path composite keys from different repositories distinct", () => {
    const path = "README.md";
    const progress = computeReviewProgress(
      [
        {
          path,
          status: "modified",
          staged: false,
          diff: "",
          repository_name: "frontend",
        },
      ],
      {
        files: {
          [`backend\u0000${path}`]: {
            path,
            repository_name: "backend",
            diff: "",
          },
        },
      },
      new Map(),
    );

    expect(progress.totalFileCount).toBe(2);
  });

  it("counts a reviewed patchless file under its repository-qualified review key", () => {
    const file = {
      path: "src/renamed.ts",
      status: "renamed" as const,
      staged: false,
      additions: 0,
      deletions: 0,
      old_path: "src/old.ts",
      diff: "",
      repository_name: "frontend",
    };
    const key = reviewFileKey(file);

    expect(computeReviewProgress([file], null, new Map([[key, { reviewed: true }]]))).toEqual({
      reviewedCount: 1,
      totalFileCount: 1,
    });
  });

  it("counts patchless-only status and PR files", () => {
    const progress = computeReviewProgress(
      [
        {
          path: "src/local-renamed.ts",
          status: "renamed",
          staged: false,
          additions: 0,
          deletions: 0,
          old_path: "src/local-old.ts",
          diff: "",
        },
      ],
      null,
      new Map(),
      [
        diffFile({
          filename: "src/pr-renamed.ts",
          status: "renamed",
          additions: 0,
          deletions: 0,
          old_path: "src/pr-old.ts",
          patch: "",
        }),
      ],
    );

    expect(progress.totalFileCount).toBe(2);
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

import { firstVisibleSection, PR_CHANGES_AUTO_EXPAND_MAX_FILES } from "./changes-panel-helpers";

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

  it("review mode: PR is first when there are no local changes and few files", () => {
    expect(
      firstVisibleSection(
        flags({
          hasPRFiles: true,
          showCommitsList: true,
          prFileCount: PR_CHANGES_AUTO_EXPAND_MAX_FILES,
        }),
      ),
    ).toBe("pr");
  });

  it("review mode: commits expand instead of PR when diff exceeds file threshold", () => {
    expect(
      firstVisibleSection(
        flags({
          hasPRFiles: true,
          showCommitsList: true,
          prFileCount: PR_CHANGES_AUTO_EXPAND_MAX_FILES + 1,
        }),
      ),
    ).toBe("commits");
  });

  it("review mode: large PR with no commits list auto-expands nothing", () => {
    expect(
      firstVisibleSection(
        flags({
          hasPRFiles: true,
          prFileCount: PR_CHANGES_AUTO_EXPAND_MAX_FILES + 1,
        }),
      ),
    ).toBeNull();
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
