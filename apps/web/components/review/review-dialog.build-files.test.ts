import { describe, it, expect } from "vitest";
import { buildAllFiles, filterPendingDiffCommentsForSession } from "./review-dialog";
import { reviewFileKey } from "./types";
import type { CumulativeDiff } from "@/lib/state/slices/session-runtime/types";
import type { Comment } from "@/lib/state/slices/comments";

const RENAMED_PATH = "src/new-name.ts";
const PREVIOUS_PATH = "src/old-name.ts";
const README_PATH = "README.md";
const FRONTEND_REPO = "frontend";
const BACKEND_REPO = "backend";

function pendingDiffComment(overrides: Partial<Comment>): Comment {
  return {
    id: "c1",
    sessionId: "s1",
    source: "diff",
    filePath: "src/app.tsx",
    startLine: 1,
    endLine: 1,
    side: "additions",
    codeContent: "const value = 1;",
    text: "fix this",
    createdAt: "2026-01-01T00:00:00Z",
    status: "pending",
    ...overrides,
  } as Comment;
}

describe("buildAllFiles (review dialog)", () => {
  // Regression for the "No changes to review" bug introduced by multi-repo
  // support (PR #767). Single-repo cumulative diffs from `parseCommitDiff`
  // carry the path only on the map key — `file.path` is undefined. The dialog
  // used to skip every such entry with a console.warn, so a 14-file commit
  // rendered as an empty review. Verify we fall back to the map key.
  it("single-repo cumulative diff: uses map key when file.path is missing", () => {
    const cumulativeDiff = {
      session_id: "s1",
      base_commit: "abc",
      head_commit: "def",
      total_commits: 1,
      files: {
        "src/a.ts": {
          status: "modified",
          staged: false,
          additions: 1,
          deletions: 1,
          diff: "@@ -1 +1 @@\n-a\n+b\n",
        },
        "src/b.ts": {
          status: "added",
          staged: false,
          additions: 5,
          deletions: 0,
          diff: "@@ -0,0 +1,5 @@\n+new\n",
        },
      },
    } as unknown as CumulativeDiff;

    const result = buildAllFiles(null, cumulativeDiff);
    expect(result).toHaveLength(2);
    const paths = result.map((f) => f.path).sort();
    expect(paths).toEqual(["src/a.ts", "src/b.ts"]);
    expect(result.every((f) => f.source === "committed")).toBe(true);
    expect(result.every((f) => !f.repository_name)).toBe(true);
    expect(result).toEqual(
      expect.arrayContaining([
        expect.objectContaining({ path: "src/a.ts", base_ref: "abc" }),
        expect.objectContaining({ path: "src/b.ts", base_ref: "abc" }),
      ]),
    );
  });
});

describe("buildAllFiles cumulative diff composition", () => {
  // Multi-repo: `mergeCumulativeFiles` uses a `<repo>\x00<path>` composite map
  // key and stamps the clean repo-relative path on `file.path`. The displayed
  // path must come from the stamped value, never from the composite key.
  it("multi-repo cumulative diff: prefers stamped path over composite map key", () => {
    const cumulativeDiff = {
      session_id: "s1",
      base_commit: "abc",
      head_commit: "def",
      total_commits: 2,
      files: {
        "frontend\u0000src/app.tsx": {
          status: "modified",
          staged: false,
          additions: 2,
          deletions: 1,
          diff: "@@ -1 +1 @@\n-x\n+y\n",
          repository_name: "frontend",
          path: "src/app.tsx",
          base_ref: "frontend-base-sha",
        },
        "backend\u0000main.go": {
          status: "modified",
          staged: false,
          additions: 3,
          deletions: 0,
          diff: "@@ -1 +1,3 @@\n+a\n+b\n+c\n",
          repository_name: "backend",
          path: "main.go",
          base_ref: "backend-base-sha",
        },
      },
    } as unknown as CumulativeDiff;

    const result = buildAllFiles(null, cumulativeDiff);
    expect(result).toHaveLength(2);
    const byRepo = Object.fromEntries(result.map((f) => [f.repository_name, f]));
    expect(byRepo["frontend"]?.path).toBe("src/app.tsx");
    expect(byRepo["backend"]?.path).toBe("main.go");
    expect(byRepo["frontend"]).toMatchObject({ base_ref: "frontend-base-sha" });
    expect(byRepo["backend"]).toMatchObject({ base_ref: "backend-base-sha" });
    // No NUL or composite-key leakage into displayed paths.
    expect(result.every((f) => !f.path.includes("\u0000"))).toBe(true);
  });

  it("returns empty array when all sources are empty/null", () => {
    expect(buildAllFiles(null, null)).toEqual([]);
  });

  it("uncommitted gitStatus wins over cumulative for the same path", () => {
    const cumulativeDiff = {
      session_id: "s1",
      base_commit: "abc",
      head_commit: "def",
      total_commits: 1,
      files: {
        "src/shared.ts": {
          status: "modified",
          staged: false,
          additions: 1,
          deletions: 1,
          diff: "@@ -1 +1 @@\n-c\n+c\n",
        },
      },
    } as unknown as CumulativeDiff;

    const gitStatusFiles = {
      "src/shared.ts": {
        path: "src/shared.ts",
        status: "modified" as const,
        staged: false,
        additions: 1,
        deletions: 1,
        diff: "@@ -1 +1 @@\n-u\n+u\n",
      },
    };

    const result = buildAllFiles(gitStatusFiles, cumulativeDiff);
    expect(result).toHaveLength(1);
    expect(result[0].source).toBe("uncommitted");
    // Exact equality (not toContain) so a future normalization change that
    // appends or rewrites diff content can't silently pass this guard.
    expect(result[0].diff).toBe("@@ -1 +1 @@\n-u\n+u");
  });
});

describe("buildAllFiles source collisions", () => {
  it("deduplicates a repo-stamped cumulative file against repo-unaware git status", () => {
    const path = "src/shared.ts";
    const gitStatusFiles = {
      [path]: {
        path,
        status: "modified" as const,
        staged: false,
        additions: 1,
        deletions: 1,
        diff: "@@ -1 +1 @@\n-local\n+fresh\n",
      },
    };
    const cumulativeDiff = {
      session_id: "s1",
      base_commit: "abc",
      head_commit: "def",
      total_commits: 1,
      files: {
        [`${FRONTEND_REPO}\u0000${path}`]: {
          path,
          repository_name: FRONTEND_REPO,
          status: "modified",
          staged: false,
          additions: 1,
          deletions: 1,
          diff: "@@ -1 +1 @@\n-stale\n+snapshot\n",
        },
      },
    } as unknown as CumulativeDiff;

    const result = buildAllFiles(gitStatusFiles, cumulativeDiff);

    expect(result).toHaveLength(1);
    expect(result[0]).toMatchObject({ path, source: "uncommitted" });
  });

  it("uses a bare cumulative key in single-repository review mode", () => {
    const path = "src/committed.ts";
    const cumulativeDiff = {
      session_id: "s1",
      base_commit: "abc",
      head_commit: "def",
      total_commits: 1,
      files: {
        [`${FRONTEND_REPO}\u0000${path}`]: {
          path,
          repository_name: FRONTEND_REPO,
          status: "modified",
          staged: false,
          diff: "@@committed@@",
        },
      },
    } as CumulativeDiff;

    const result = buildAllFiles(null, cumulativeDiff, undefined, undefined, false);

    expect(result).toHaveLength(1);
    expect(reviewFileKey(result[0])).toBe(path);
    expect(result[0].repository_name).toBeUndefined();
  });
});

describe("buildAllFiles sort order", () => {
  it("sorts multi-repo files by repository name and then path", () => {
    const gitStatusFiles = {
      "frontend\u0000src/a.ts": {
        path: "src/a.ts",
        status: "modified" as const,
        staged: false,
        repository_name: FRONTEND_REPO,
      },
      "backend\u0000src/b.ts": {
        path: "src/b.ts",
        status: "modified" as const,
        staged: false,
        repository_name: BACKEND_REPO,
      },
    };

    const result = buildAllFiles(gitStatusFiles, null);

    expect(result.map((file) => `${file.repository_name}:${file.path}`)).toEqual([
      "backend:src/b.ts",
      "frontend:src/a.ts",
    ]);
  });
});

describe("buildAllFiles patchless status files", () => {
  it("keeps a patchless PR rename with its previous path", () => {
    const result = buildAllFiles(null, null, [
      {
        filename: RENAMED_PATH,
        old_path: PREVIOUS_PATH,
        status: "renamed",
        patch: "",
        additions: 0,
        deletions: 0,
      },
    ]);

    expect(result).toEqual([
      expect.objectContaining({
        path: RENAMED_PATH,
        old_path: PREVIOUS_PATH,
        status: "renamed",
        diff: "",
        source: "pr",
      }),
    ]);
    expect(reviewFileKey(result[0])).toBe(RENAMED_PATH);
    expect(result[0].repository_name).toBeUndefined();
  });

  it("keeps patchless uncommitted and cumulative files with metadata", () => {
    const gitStatusFiles = {
      "assets/logo.png": {
        path: "assets/logo.png",
        status: "modified" as const,
        staged: false,
        diff_skip_reason: "binary" as const,
      },
    };
    const cumulativeDiff = {
      session_id: "s1",
      base_commit: "abc",
      head_commit: "def",
      total_commits: 1,
      files: {
        [RENAMED_PATH]: {
          path: RENAMED_PATH,
          old_path: PREVIOUS_PATH,
          status: "renamed",
          staged: false,
          additions: 0,
          deletions: 0,
          diff: "",
        },
      },
    } as CumulativeDiff;

    const result = buildAllFiles(gitStatusFiles, cumulativeDiff);

    expect(result).toEqual(
      expect.arrayContaining([
        expect.objectContaining({
          path: "assets/logo.png",
          diff_skip_reason: "binary",
          diff: "",
          source: "uncommitted",
        }),
        expect.objectContaining({
          path: RENAMED_PATH,
          old_path: PREVIOUS_PATH,
          status: "renamed",
          diff: "",
          source: "committed",
        }),
      ]),
    );
    expect(result).toHaveLength(2);
  });
});

describe("buildAllFiles patchless source identity", () => {
  it("keeps patchless uncommitted precedence over a same-repo PR file", () => {
    const gitStatusFiles = {
      [`${FRONTEND_REPO}\u0000${README_PATH}`]: {
        path: README_PATH,
        status: "modified" as const,
        staged: false,
        repository_name: FRONTEND_REPO,
      },
    };

    const result = buildAllFiles(
      gitStatusFiles,
      null,
      [
        {
          filename: README_PATH,
          status: "modified",
          patch: "",
          additions: 0,
          deletions: 0,
        },
      ],
      FRONTEND_REPO,
    );

    expect(result).toEqual([
      expect.objectContaining({
        path: README_PATH,
        source: "uncommitted",
        repository_name: FRONTEND_REPO,
      }),
    ]);
  });

  it("keeps patchless cumulative precedence over a same-repo PR file", () => {
    const cumulativeDiff = {
      session_id: "s1",
      base_commit: "abc",
      head_commit: "def",
      total_commits: 1,
      files: {
        [`${FRONTEND_REPO}\u0000${README_PATH}`]: {
          path: README_PATH,
          status: "modified",
          staged: false,
          repository_name: FRONTEND_REPO,
        },
      },
    } as CumulativeDiff;

    const result = buildAllFiles(
      null,
      cumulativeDiff,
      [
        {
          filename: README_PATH,
          status: "modified",
          patch: "",
          additions: 0,
          deletions: 0,
        },
      ],
      FRONTEND_REPO,
    );

    expect(result).toEqual([
      expect.objectContaining({
        path: README_PATH,
        source: "committed",
        repository_name: FRONTEND_REPO,
      }),
    ]);
  });

  it("keeps same-path files from different repositories distinct", () => {
    const gitStatusFiles = {
      [`${BACKEND_REPO}\u0000${README_PATH}`]: {
        path: README_PATH,
        status: "modified" as const,
        staged: false,
        repository_name: BACKEND_REPO,
      },
    };

    const result = buildAllFiles(
      gitStatusFiles,
      null,
      [
        {
          filename: README_PATH,
          status: "modified",
          patch: "",
          additions: 0,
          deletions: 0,
        },
      ],
      FRONTEND_REPO,
    );

    expect(result.map(reviewFileKey).sort()).toEqual([
      "backend\u0000README.md",
      "frontend\u0000README.md",
    ]);
  });
});

describe("filterPendingDiffCommentsForSession", () => {
  it("keeps only diff comments from the active session", () => {
    const comments = [
      pendingDiffComment({ id: "current", sessionId: "current" }),
      pendingDiffComment({ id: "other", sessionId: "other" }),
      {
        id: "plan",
        sessionId: "current",
        source: "plan",
        selectedText: "task text",
        text: "plan note",
        createdAt: "2026-01-01T00:00:00Z",
        status: "pending",
      },
    ] satisfies Comment[];

    expect(filterPendingDiffCommentsForSession(comments, "current").map((c) => c.id)).toEqual([
      "current",
    ]);
  });
});
