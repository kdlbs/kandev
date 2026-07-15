import { describe, expect, it } from "vitest";
import { reviewFileKey } from "@/components/review/types";
import type { PRDiffFile } from "@/lib/types/github";
import { buildReviewSources, normalizeReviewStatusSources } from "./use-review-sources";

const namedStatus = {
  repository_name: "frontend",
  status: {
    files: {
      "src/local.ts": { diff: "@@local@@", status: "modified", additions: 1, deletions: 0 },
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
