import { describe, it, expect } from "vitest";
import { buildAllFiles } from "./review-dialog";
import type { CumulativeDiff } from "@/lib/state/slices/session-runtime/types";

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
  });

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
        },
        "backend\u0000main.go": {
          status: "modified",
          staged: false,
          additions: 3,
          deletions: 0,
          diff: "@@ -1 +1,3 @@\n+a\n+b\n+c\n",
          repository_name: "backend",
          path: "main.go",
        },
      },
    } as unknown as CumulativeDiff;

    const result = buildAllFiles(null, cumulativeDiff);
    expect(result).toHaveLength(2);
    const byRepo = Object.fromEntries(result.map((f) => [f.repository_name, f]));
    expect(byRepo["frontend"]?.path).toBe("src/app.tsx");
    expect(byRepo["backend"]?.path).toBe("main.go");
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
    expect(result[0].diff).toContain("-u\n+u");
  });
});
