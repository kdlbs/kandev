import { describe, expect, it } from "vitest";
import { reviewFileKey } from "@/components/review/types";
import type { PRDiffFile } from "@/lib/types/github";
import { buildReviewSources, normalizeReviewStatusSources } from "./use-review-sources";

const NESTED_SCOPE = "vendor/lib";

const namedStatus = {
  repository_name: "frontend",
  status: {
    files: {
      "src/local.ts": { diff: "@@local@@", status: "modified", additions: 1, deletions: 0 },
    },
  },
};

const nestedStatus = {
  repository_name: NESTED_SCOPE,
  status: {
    files: {
      "src/local.ts": { diff: "@@nested@@", status: "modified", additions: 1, deletions: 0 },
    },
  },
};

const prDiffFiles = [
  { filename: "src/pr.ts", status: "modified", patch: "@@pr@@" },
] as PRDiffFile[];

const cumulativeDiff = {
  files: {
    ["frontend\u0000src/committed.ts"]: {
      path: "src/committed.ts",
      repository_name: "frontend",
      diff: "@@committed@@",
    },
  },
};

function reviewKeysFor(taskRepositoryCount: number) {
  const normalized = normalizeReviewStatusSources({
    gitStatus: undefined,
    statusByRepo: [namedStatus],
    taskRepositoryCount,
    resolvedPRRepoName: "frontend",
  });
  const result = buildReviewSources({
    gitStatus: normalized.normalizedGitStatus,
    statusByRepo: normalized.normalizedStatusByRepo,
    cumulativeDiff,
    prDiffFiles,
    prRepoName: normalized.prRepoName,
    useRepositoryKeys: normalized.useRepositoryKeys,
  });
  return result.allFiles.map(reviewFileKey).sort();
}

describe("review source key mode", () => {
  it.each([
    {
      name: "bare for one named status in a single-repo task",
      taskRepositoryCount: 1,
      expected: ["src/committed.ts", "src/local.ts", "src/pr.ts"],
    },
    {
      name: "composite for a true multi-repo task",
      taskRepositoryCount: 2,
      expected: [
        "frontend\u0000src/committed.ts",
        "frontend\u0000src/local.ts",
        "frontend\u0000src/pr.ts",
      ],
    },
  ])("keeps local, cumulative, and PR keys $name", ({ taskRepositoryCount, expected }) => {
    expect(reviewKeysFor(taskRepositoryCount)).toEqual(expected);
  });

  it("preserves same-path cumulative files before task and status metadata hydrate", () => {
    const normalized = normalizeReviewStatusSources({
      gitStatus: undefined,
      statusByRepo: [namedStatus],
      taskRepositoryCount: 0,
      resolvedPRRepoName: "frontend",
      cumulativeRepositoryNames: ["frontend", "backend"],
    });
    const samePathCumulativeDiff = {
      files: Object.fromEntries(
        ["frontend", "backend"].map((repositoryName) => [
          `${repositoryName}\u0000README.md`,
          { path: "README.md", repository_name: repositoryName, diff: `@@${repositoryName}@@` },
        ]),
      ),
    };

    const result = buildReviewSources({
      gitStatus: normalized.normalizedGitStatus,
      statusByRepo: normalized.normalizedStatusByRepo,
      cumulativeDiff: samePathCumulativeDiff,
      prDiffFiles: undefined,
      prRepoName: normalized.prRepoName,
      useRepositoryKeys: normalized.useRepositoryKeys,
    });

    expect(result.allFiles.map(reviewFileKey).sort()).toEqual([
      "backend\u0000README.md",
      "frontend\u0000README.md",
      "frontend\u0000src/local.ts",
    ]);
  });
});

describe("nested scope review sources", () => {
  it("retains a real workspace root alongside a named submodule scope", () => {
    const rootStatus = {
      repository_name: "",
      status: {
        files: {
          "README.md": { diff: "@@root@@", status: "modified", additions: 1, deletions: 0 },
        },
      },
    };
    const normalized = normalizeReviewStatusSources({
      gitStatus: undefined,
      statusByRepo: [rootStatus, nestedStatus],
      taskRepositoryCount: 1,
    });

    expect(normalized.useRepositoryKeys).toBe(true);
    expect(normalized.normalizedStatusByRepo?.map((entry) => entry.repository_name)).toEqual([
      "",
      NESTED_SCOPE,
    ]);

    const result = buildReviewSources({
      gitStatus: normalized.normalizedGitStatus,
      statusByRepo: normalized.normalizedStatusByRepo,
      cumulativeDiff: null,
      prDiffFiles: undefined,
      useRepositoryKeys: normalized.useRepositoryKeys,
    });

    expect(result.allFiles.map(reviewFileKey)).toEqual([
      "\u0000README.md",
      `${NESTED_SCOPE}\u0000src/local.ts`,
    ]);
  });

  it("keeps same-named root and nested-scope cumulative files distinct", () => {
    const result = buildReviewSources({
      gitStatus: undefined,
      statusByRepo: [
        {
          repository_name: "",
          status: {
            files: {
              "README.md": {
                diff: "@@root@@",
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
          [`${NESTED_SCOPE}\u0000README.md`]: {
            path: "README.md",
            repository_name: NESTED_SCOPE,
            diff: "@@child@@",
            status: "modified",
            additions: 1,
            deletions: 0,
          },
        },
      },
      prDiffFiles: undefined,
      useRepositoryKeys: true,
    });

    expect(result.allFiles.map(reviewFileKey).sort()).toEqual([
      "\u0000README.md",
      `${NESTED_SCOPE}\u0000README.md`,
    ]);
  });

  it("keeps a root cumulative file beside a same-named child uncommitted file", () => {
    const result = buildReviewSources({
      gitStatus: undefined,
      statusByRepo: [nestedStatus],
      cumulativeDiff: {
        files: {
          "README.md": {
            path: "README.md",
            diff: "@@root-committed@@",
            status: "modified",
            additions: 1,
            deletions: 0,
          },
        },
      },
      prDiffFiles: undefined,
      useRepositoryKeys: true,
    });

    expect(result.allFiles.map(reviewFileKey).sort()).toEqual([
      "README.md",
      `${NESTED_SCOPE}\u0000src/local.ts`,
    ]);
    expect(result.allFiles.find((file) => file.repository_name === undefined)?.source).toBe(
      "committed",
    );
    expect(result.allFiles.find((file) => file.repository_name === NESTED_SCOPE)?.source).toBe(
      "uncommitted",
    );
  });
});

describe("nested scope gitlink sources", () => {
  it("suppresses an available parent gitlink but keeps an unavailable one", () => {
    const rootStatus = {
      repository_name: "",
      status: {
        files: {
          [NESTED_SCOPE]: { diff: "@@gitlink@@", status: "modified", additions: 1, deletions: 1 },
        },
      },
    };
    const childStatus = {
      repository_name: NESTED_SCOPE,
      status: {
        files: {
          "README.md": { diff: "@@child@@", status: "modified", additions: 1, deletions: 0 },
        },
      },
    };
    const available = buildReviewSources({
      gitStatus: undefined,
      statusByRepo: [rootStatus, childStatus],
      cumulativeDiff: null,
      prDiffFiles: undefined,
      useRepositoryKeys: true,
    });
    expect(available.allFiles.map(reviewFileKey)).toEqual([`${NESTED_SCOPE}\u0000README.md`]);

    const unavailable = buildReviewSources({
      gitStatus: undefined,
      statusByRepo: [rootStatus],
      cumulativeDiff: null,
      prDiffFiles: undefined,
      useRepositoryKeys: true,
    });
    expect(unavailable.allFiles.map(reviewFileKey)).toEqual([`\u0000${NESTED_SCOPE}`]);
  });
});
