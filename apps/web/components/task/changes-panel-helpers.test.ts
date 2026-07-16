import { describe, it, expect } from "vitest";
import {
  buildPrByRepoMap,
  computeReviewProgress,
  mapPRFilesToChangedFiles,
  selectPRFilesForReviewProgress,
  type ReviewProgressPRFile,
} from "./changes-panel-helpers";
import { hashDiff, resolvePRReviewRepositoryName, reviewFileKey } from "@/components/review/types";
import type { PRDiffFile } from "@/lib/types/github";

const PRIMARY_PR_ID = "primary-pr";
const RENAMED_PATH = "src/renamed.ts";

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

function progressPRFile(
  overrides: Partial<PRDiffFile> & { repository_name?: string },
): ReviewProgressPRFile {
  return {
    ...diffFile(overrides),
    repository_name: overrides.repository_name,
  };
}

function progressPRSource(repositoryName: string, filename: string) {
  return { repositoryName, files: [diffFile({ filename })] };
}

type ProgressLocalFile = Parameters<typeof computeReviewProgress>[0][number];

function progressLocalFile(
  path: string,
  overrides: Partial<ProgressLocalFile> = {},
): ProgressLocalFile {
  return { path, status: "modified", staged: false, diff: "", ...overrides };
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

describe("selectPRFilesForReviewProgress", () => {
  it("selects only the primary PR and qualifies it in a multi-repo task", () => {
    const repositoryName = resolvePRReviewRepositoryName(
      { repository_id: "repo-frontend", repo: "widgets" },
      "acme/widgets",
    );
    const files = selectPRFilesForReviewProgress(
      new Map([
        [PRIMARY_PR_ID, progressPRSource(repositoryName ?? "widgets", "README.md")],
        ["secondary-pr", progressPRSource("backend", "README.md")],
      ]),
      PRIMARY_PR_ID,
      true,
    );

    expect(files.map((file) => reviewFileKey({ path: file.filename, ...file }))).toEqual([
      "acme-widgets\u0000README.md",
    ]);
  });

  it("keeps the selected primary PR bare in a legacy single-repo task", () => {
    const files = selectPRFilesForReviewProgress(
      new Map([[PRIMARY_PR_ID, progressPRSource("kandev", "README.md")]]),
      PRIMARY_PR_ID,
      false,
    );

    expect(files.map((file) => reviewFileKey({ path: file.filename, ...file }))).toEqual([
      "README.md",
    ]);
  });
});

describe("computeReviewProgress PR files", () => {
  it("counts same-path PR reviews per repository using each patch hash", () => {
    const path = "README.md";
    const frontendPatch = "@@ -1 +1 @@\n-frontend old\n+frontend new";
    const backendPatch = "@@ -1 +1 @@\n-backend old\n+backend new";
    const frontendKey = reviewFileKey({ path, repository_name: "frontend" });
    const backendKey = reviewFileKey({ path, repository_name: "backend" });

    const progress = computeReviewProgress(
      [],
      null,
      new Map([
        [frontendKey, { reviewed: true, diffHash: hashDiff(frontendPatch) }],
        [backendKey, { reviewed: true, diffHash: hashDiff(backendPatch) }],
      ]),
      [
        progressPRFile({ filename: path, patch: frontendPatch, repository_name: "frontend" }),
        progressPRFile({ filename: path, patch: backendPatch, repository_name: "backend" }),
      ],
    );

    expect(progress).toEqual({ reviewedCount: 2, totalFileCount: 2 });
  });

  it("keeps a same-repo uncommitted file over PR data without dropping another repo", () => {
    const path = "README.md";
    const frontendDiff = "@@ -1 +1 @@\n-local old\n+local new";
    const backendPatch = "@@ -1 +1 @@\n-backend old\n+backend new";
    const frontendKey = reviewFileKey({ path, repository_name: "frontend" });
    const backendKey = reviewFileKey({ path, repository_name: "backend" });

    const progress = computeReviewProgress(
      [progressLocalFile(path, { diff: frontendDiff, repository_name: "frontend" })],
      null,
      new Map([
        [frontendKey, { reviewed: true, diffHash: hashDiff(frontendDiff) }],
        [backendKey, { reviewed: true, diffHash: hashDiff(backendPatch) }],
      ]),
      [
        progressPRFile({
          filename: path,
          patch: "@@ -1 +1 @@\n-pr old\n+pr new",
          repository_name: "frontend",
        }),
        progressPRFile({ filename: path, patch: backendPatch, repository_name: "backend" }),
      ],
    );

    expect(progress).toEqual({ reviewedCount: 2, totalFileCount: 2 });
  });
});

describe("computeReviewProgress rejected source precedence", () => {
  const uncommittedWinner: Parameters<typeof computeReviewProgress>[0] = [
    progressLocalFile(RENAMED_PATH, { status: "renamed", old_path: "src/old.ts" }),
  ];
  const cumulativeWinner: Parameters<typeof computeReviewProgress>[1] = {
    files: { [RENAMED_PATH]: { path: RENAMED_PATH, status: "renamed", diff: "" } },
  };

  it.each([
    { name: "uncommitted", uncommittedFiles: uncommittedWinner, cumulativeDiff: null },
    { name: "cumulative", uncommittedFiles: [], cumulativeDiff: cumulativeWinner },
  ])("does not hash a rejected PR patch past a patchless $name winner", (winner) => {
    const progress = computeReviewProgress(
      winner.uncommittedFiles,
      winner.cumulativeDiff,
      new Map([[RENAMED_PATH, { reviewed: true, diffHash: hashDiff("") }]]),
      [diffFile({ filename: RENAMED_PATH, patch: "@@rejected@@" })],
    );

    expect(progress).toEqual({ reviewedCount: 1, totalFileCount: 1 });
  });
});

describe("computeReviewProgress source precedence", () => {
  it("keeps bare uncommitted precedence over a stamped cumulative file at the same path", () => {
    const path = RENAMED_PATH;
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
      path: RENAMED_PATH,
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

describe("computeReviewProgress repository key mode", () => {
  it("uses bare local, cumulative, and PR keys for a single named repository", () => {
    const localPath = "src/local.ts";
    const cumulativePath = "src/cumulative.ts";
    const prPath = "src/pr.ts";
    const progress = computeReviewProgress(
      [progressLocalFile(localPath, { repository_name: "frontend", diff: "@@local@@" })],
      {
        files: {
          [`frontend\u0000${cumulativePath}`]: {
            path: cumulativePath,
            repository_name: "frontend",
            diff: "@@cumulative@@",
          },
        },
      },
      new Map([
        [localPath, { reviewed: true }],
        [cumulativePath, { reviewed: true }],
        [prPath, { reviewed: true }],
      ]),
      [progressPRFile({ filename: prPath, patch: "@@pr@@" })],
      false,
    );

    expect(progress).toEqual({ reviewedCount: 3, totalFileCount: 3 });
  });
});

describe("computeReviewProgress scalability", () => {
  it("indexes cumulative file identities once for large review sets", () => {
    const fileCount = 80;
    let pathReads = 0;
    const files = Object.fromEntries(
      Array.from({ length: fileCount }, (_, index) => {
        const path = `src/file-${index}.ts`;
        return [
          `frontend\u0000${path}`,
          {
            get path() {
              pathReads++;
              return path;
            },
            repository_name: "frontend",
            diff: `@@ -1 +1 @@\n-old-${index}\n+new-${index}`,
          },
        ];
      }),
    );
    const reviews = new Map(
      Array.from({ length: fileCount }, (_, index) => [
        `frontend\u0000src/file-${index}.ts`,
        { reviewed: true },
      ]),
    );

    expect(computeReviewProgress([], { files }, reviews)).toEqual({
      reviewedCount: fileCount,
      totalFileCount: fileCount,
    });
    expect(pathReads).toBeLessThanOrEqual(fileCount * 4);
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
