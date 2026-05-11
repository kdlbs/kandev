import { describe, it, expect } from "vitest";
import { buildReviewSources } from "./use-review-sources";
import type { PRDiffFile } from "@/lib/types/github";

/* eslint-disable max-lines-per-function -- multi-case describe block */
describe("buildReviewSources", () => {
  it("returns empty result when no inputs", () => {
    const result = buildReviewSources({
      gitStatus: undefined,
      statusByRepo: undefined,
      cumulativeDiff: null,
      prDiffFiles: undefined,
    });
    expect(result.allFiles).toEqual([]);
    expect(result.sourceCounts).toEqual({ uncommitted: 0, committed: 0, pr: 0 });
  });

  it("tags uncommitted files from gitStatus", () => {
    const result = buildReviewSources({
      gitStatus: {
        files: {
          "src/a.ts": {
            diff: "@@ -1 +1 @@\n-a\n+b\n",
            status: "modified",
            additions: 1,
            deletions: 1,
          },
        },
      },
      statusByRepo: undefined,
      cumulativeDiff: null,
      prDiffFiles: undefined,
    });
    expect(result.allFiles).toHaveLength(1);
    expect(result.allFiles[0].source).toBe("uncommitted");
    expect(result.allFiles[0].path).toBe("src/a.ts");
    expect(result.sourceCounts).toEqual({ uncommitted: 1, committed: 0, pr: 0 });
  });

  it("tags committed files from cumulativeDiff", () => {
    const result = buildReviewSources({
      gitStatus: undefined,
      statusByRepo: undefined,
      cumulativeDiff: {
        files: {
          "src/b.ts": {
            diff: "@@ -1 +1 @@\n-x\n+y\n",
            status: "modified",
            additions: 1,
            deletions: 1,
          },
        },
      },
      prDiffFiles: undefined,
    });
    expect(result.allFiles).toHaveLength(1);
    expect(result.allFiles[0].source).toBe("committed");
    expect(result.sourceCounts).toEqual({ uncommitted: 0, committed: 1, pr: 0 });
  });

  it("tags pr files from prDiffFiles", () => {
    const prFiles: PRDiffFile[] = [
      {
        filename: "src/c.ts",
        status: "modified",
        patch: "@@ -1 +1 @@\n-p\n+q\n",
        additions: 1,
        deletions: 1,
      },
    ];
    const result = buildReviewSources({
      gitStatus: undefined,
      statusByRepo: undefined,
      cumulativeDiff: null,
      prDiffFiles: prFiles,
    });
    expect(result.allFiles).toHaveLength(1);
    expect(result.allFiles[0].source).toBe("pr");
    expect(result.sourceCounts).toEqual({ uncommitted: 0, committed: 0, pr: 1 });
  });

  it("dedupes by path: uncommitted wins over committed wins over PR", () => {
    const prFiles: PRDiffFile[] = [
      {
        filename: "src/shared.ts",
        status: "modified",
        patch: "@@ -1 +1 @@\n-pr\n+pr\n",
        additions: 1,
        deletions: 1,
      },
    ];
    const result = buildReviewSources({
      gitStatus: {
        files: {
          "src/shared.ts": {
            diff: "@@ -1 +1 @@\n-u\n+u\n",
            status: "modified",
            additions: 1,
            deletions: 1,
          },
        },
      },
      statusByRepo: undefined,
      cumulativeDiff: {
        files: {
          "src/shared.ts": {
            diff: "@@ -1 +1 @@\n-c\n+c\n",
            status: "modified",
            additions: 1,
            deletions: 1,
          },
        },
      },
      prDiffFiles: prFiles,
    });
    expect(result.allFiles).toHaveLength(1);
    expect(result.allFiles[0].source).toBe("uncommitted");
    expect(result.sourceCounts).toEqual({ uncommitted: 1, committed: 0, pr: 0 });
  });

  it("counts files across three distinct sources", () => {
    const prFiles: PRDiffFile[] = [
      {
        filename: "src/c.ts",
        status: "modified",
        patch: "@@ -1 +1 @@\n-p\n+q\n",
        additions: 1,
        deletions: 1,
      },
    ];
    const result = buildReviewSources({
      gitStatus: {
        files: { "src/a.ts": { diff: "@@a@@", status: "modified", additions: 1, deletions: 0 } },
      },
      statusByRepo: undefined,
      cumulativeDiff: {
        files: { "src/b.ts": { diff: "@@b@@", status: "modified", additions: 1, deletions: 0 } },
      },
      prDiffFiles: prFiles,
    });
    expect(result.sourceCounts).toEqual({ uncommitted: 1, committed: 1, pr: 1 });
    expect(result.allFiles).toHaveLength(3);
  });

  it("multi-repo: tags uncommitted files with repository_name from statusByRepo", () => {
    const result = buildReviewSources({
      gitStatus: undefined,
      statusByRepo: [
        {
          repository_name: "frontend",
          status: {
            files: {
              "src/x.ts": {
                diff: "@@x@@",
                status: "modified",
                additions: 1,
                deletions: 0,
              },
            },
          },
        },
        {
          repository_name: "backend",
          status: {
            files: {
              "src/y.go": {
                diff: "@@y@@",
                status: "modified",
                additions: 1,
                deletions: 0,
              },
            },
          },
        },
      ],
      cumulativeDiff: null,
      prDiffFiles: undefined,
    });
    expect(result.allFiles).toHaveLength(2);
    expect(result.sourceCounts).toEqual({ uncommitted: 2, committed: 0, pr: 0 });
    const repoNames = result.allFiles.map((f) => f.repository_name).sort();
    expect(repoNames).toEqual(["backend", "frontend"]);
  });

  it("multi-repo uncommitted + cumulativeDiff overlap: file appears once as uncommitted", () => {
    const result = buildReviewSources({
      gitStatus: undefined,
      statusByRepo: [
        {
          repository_name: "frontend",
          status: {
            files: {
              "src/shared.ts": {
                diff: "@@u@@",
                status: "modified",
                additions: 1,
                deletions: 0,
              },
            },
          },
        },
      ],
      cumulativeDiff: {
        files: {
          "src/shared.ts": {
            diff: "@@c@@",
            status: "modified",
            additions: 1,
            deletions: 0,
          },
        },
      },
      prDiffFiles: undefined,
    });
    expect(result.allFiles).toHaveLength(1);
    expect(result.allFiles[0].source).toBe("uncommitted");
    expect(result.sourceCounts).toEqual({ uncommitted: 1, committed: 0, pr: 0 });
  });

  it("multi-repo uncommitted + PR overlap: file appears once as uncommitted", () => {
    const result = buildReviewSources({
      gitStatus: undefined,
      statusByRepo: [
        {
          repository_name: "frontend",
          status: {
            files: {
              "src/shared.ts": {
                diff: "@@u@@",
                status: "modified",
                additions: 1,
                deletions: 0,
              },
            },
          },
        },
      ],
      cumulativeDiff: null,
      prDiffFiles: [
        {
          filename: "src/shared.ts",
          status: "modified",
          patch: "@@p@@",
          additions: 1,
          deletions: 0,
        },
      ],
    });
    expect(result.allFiles).toHaveLength(1);
    expect(result.allFiles[0].source).toBe("uncommitted");
    expect(result.sourceCounts).toEqual({ uncommitted: 1, committed: 0, pr: 0 });
  });

  it("sorts files by repository_name then path", () => {
    const result = buildReviewSources({
      gitStatus: undefined,
      statusByRepo: [
        {
          repository_name: "zeta",
          status: {
            files: {
              "z.ts": { diff: "@@@@", status: "modified", additions: 0, deletions: 0 },
            },
          },
        },
        {
          repository_name: "alpha",
          status: {
            files: {
              "b.ts": { diff: "@@@@", status: "modified", additions: 0, deletions: 0 },
              "a.ts": { diff: "@@@@", status: "modified", additions: 0, deletions: 0 },
            },
          },
        },
      ],
      cumulativeDiff: null,
      prDiffFiles: undefined,
    });
    const paths = result.allFiles.map((f) => `${f.repository_name}:${f.path}`);
    expect(paths).toEqual(["alpha:a.ts", "alpha:b.ts", "zeta:z.ts"]);
  });
});
