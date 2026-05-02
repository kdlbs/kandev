"use client";

import { hashDiff, normalizeDiffContent } from "@/components/review/types";
import type { FileInfo } from "@/lib/state/store";
import type { PRDiffFile } from "@/lib/types/github";
import type { PRChangedFile } from "./changes-panel-timeline";

export type ChangedFile = {
  path: string;
  status: FileInfo["status"];
  staged: boolean;
  plus: number | undefined;
  minus: number | undefined;
  oldPath: string | undefined;
  /** Repository this file belongs to in multi-repo workspaces; empty for single-repo. */
  repositoryName?: string;
};

export function mapToChangedFiles(files: FileInfo[]): ChangedFile[] {
  return files.map((file) => ({
    path: file.path,
    status: file.status,
    staged: file.staged,
    plus: file.additions,
    minus: file.deletions,
    oldPath: file.old_path,
    repositoryName: file.repository_name,
  }));
}

type CumulativeDiffFiles = Record<
  string,
  { diff?: string; status?: string; additions?: number; deletions?: number }
>;

function collectReviewPaths(
  uncommittedFiles: FileInfo[],
  cumulativeDiffFiles: CumulativeDiffFiles | undefined,
  prFiles?: PRDiffFile[],
): Set<string> {
  const paths = new Set<string>();
  for (const file of uncommittedFiles) {
    if (file.diff && normalizeDiffContent(file.diff)) paths.add(file.path);
  }
  if (cumulativeDiffFiles) {
    for (const [path, file] of Object.entries(cumulativeDiffFiles)) {
      if (!paths.has(path) && file.diff && normalizeDiffContent(file.diff)) paths.add(path);
    }
  }
  if (prFiles) {
    for (const file of prFiles) {
      if (!paths.has(file.filename) && file.patch) paths.add(file.filename);
    }
  }
  return paths;
}

function getDiffForPath(
  path: string,
  uncommittedFiles: FileInfo[],
  cumulativeDiffFiles: CumulativeDiffFiles | undefined,
  prFiles?: PRDiffFile[],
): string {
  const uncommitted = uncommittedFiles.find((f) => f.path === path);
  if (uncommitted?.diff) return normalizeDiffContent(uncommitted.diff);
  const cumDiff = cumulativeDiffFiles?.[path]?.diff;
  if (cumDiff) return normalizeDiffContent(cumDiff);
  if (prFiles) {
    const prFile = prFiles.find((f) => f.filename === path);
    if (prFile?.patch) return normalizeDiffContent(prFile.patch);
  }
  return "";
}

export function computeReviewProgress(
  uncommittedFiles: FileInfo[],
  cumulativeDiff: { files?: CumulativeDiffFiles } | null,
  reviews: Map<string, { reviewed: boolean; diffHash?: string }>,
  prFiles?: PRDiffFile[],
) {
  const cumulativeDiffFiles = cumulativeDiff?.files;
  const paths = collectReviewPaths(uncommittedFiles, cumulativeDiffFiles, prFiles);
  let reviewed = 0;
  for (const path of paths) {
    const state = reviews.get(path);
    if (!state?.reviewed) continue;
    const diffContent = getDiffForPath(path, uncommittedFiles, cumulativeDiffFiles, prFiles);
    if (diffContent && state.diffHash && state.diffHash !== hashDiff(diffContent)) continue;
    reviewed++;
  }
  return { reviewedCount: reviewed, totalFileCount: paths.size };
}

export function computeStagedStats(stagedFiles: FileInfo[]) {
  const adds = stagedFiles.reduce((sum, f) => sum + (f.additions || 0), 0);
  const dels = stagedFiles.reduce((sum, f) => sum + (f.deletions || 0), 0);
  return { stagedFileCount: stagedFiles.length, stagedAdditions: adds, stagedDeletions: dels };
}

export function mapPRFilesToChangedFiles(
  files: PRDiffFile[],
  repositoryName?: string,
): PRChangedFile[] {
  return files.map((file) => {
    let status: FileInfo["status"];
    switch (file.status) {
      case "added":
        status = "added";
        break;
      case "removed":
        status = "deleted";
        break;
      case "renamed":
        status = "renamed";
        break;
      default:
        status = "modified";
    }
    return {
      path: file.filename,
      status,
      plus: file.additions,
      minus: file.deletions,
      oldPath: file.old_path,
      repository_name: repositoryName ?? "",
    };
  });
}

export function filterUnpushedCommits<T extends { commit_sha: string }>(
  localCommits: T[],
  prCommits: { sha: string }[],
): T[] {
  if (prCommits.length === 0) return localCommits;
  return localCommits.filter(
    (c) =>
      !prCommits.some((pr) => pr.sha.startsWith(c.commit_sha) || c.commit_sha.startsWith(pr.sha)),
  );
}

type MergedCommit = {
  commit_sha: string;
  commit_message: string;
  insertions: number;
  deletions: number;
  pushed: boolean;
  /** Multi-repo: name of the repo this commit was made in. Empty for single-repo. */
  repository_name?: string;
};

/**
 * Merge local session commits and PR commits into a single list. Local
 * commits matched to a PR commit are marked pushed; unmatched are unpushed.
 * PR-only commits (e.g. from other contributors) are appended as pushed.
 * Order: unpushed first, then pushed.
 */
export function mergeCommits(
  localCommits: {
    commit_sha: string;
    commit_message: string;
    insertions: number;
    deletions: number;
    /** Multi-repo: name of the repo this commit was made in. */
    repository_name?: string;
  }[],
  prCommits: { sha: string; message: string; additions: number; deletions: number }[],
): MergedCommit[] {
  const matchesPR = (localSha: string) =>
    prCommits.some((pr) => pr.sha.startsWith(localSha) || localSha.startsWith(pr.sha));
  const unpushed: MergedCommit[] = [];
  const pushed: MergedCommit[] = [];
  const matchedPRShas = new Set<string>();
  for (const c of localCommits) {
    if (matchesPR(c.commit_sha)) {
      pushed.push({ ...c, pushed: true });
      for (const pr of prCommits) {
        if (pr.sha.startsWith(c.commit_sha) || c.commit_sha.startsWith(pr.sha)) {
          matchedPRShas.add(pr.sha);
        }
      }
    } else {
      unpushed.push({ ...c, pushed: false });
    }
  }
  for (const pr of prCommits) {
    if (!matchedPRShas.has(pr.sha)) {
      pushed.push({
        commit_sha: pr.sha,
        commit_message: pr.message,
        insertions: pr.additions,
        deletions: pr.deletions,
        pushed: true,
      });
    }
  }
  return [...unpushed, ...pushed];
}

export function getBaseBranchDisplay(baseBranch: string | undefined): string {
  return baseBranch ? baseBranch.replace(/^origin\//, "") : "main";
}

export function isMultiRepoCommits(commits: { repository_name?: string }[]): boolean {
  return commits.some((c) => !!c.repository_name);
}
