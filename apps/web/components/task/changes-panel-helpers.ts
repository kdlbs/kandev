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

/** Which timeline section, if any, renders first (topmost) in the Changes panel. */
type FirstVisibleSection = "pr" | "unstaged" | "staged" | "commits" | null;

/**
 * Computes the first (topmost) visible section so the panel can auto-expand it,
 * mirroring the render precedence in `ChangesPanelTimeline`:
 *   PR (review mode, no local changes) → Unstaged → Staged → Commits.
 *
 * The "review mode" PR section only sits at the top when there are no local
 * working-tree changes; with local changes the PR section moves below Staged and
 * is therefore never first. Returns null when nothing is shown.
 */
export function firstVisibleSection(flags: {
  hasPRFiles: boolean;
  hasUnstaged: boolean;
  hasStaged: boolean;
  showCommitsList: boolean;
}): FirstVisibleSection {
  const { hasPRFiles, hasUnstaged, hasStaged, showCommitsList } = flags;
  if (hasPRFiles && !hasUnstaged && !hasStaged) return "pr";
  if (hasUnstaged) return "unstaged";
  if (hasStaged) return "staged";
  if (showCommitsList) return "commits";
  return null;
}

type MergedCommit = {
  commit_sha: string;
  commit_message: string;
  insertions: number;
  deletions: number;
  pushed: boolean;
  /** Multi-repo: name of the repo this commit was made in. Empty for single-repo. */
  repository_name?: string;
  committed_at?: string;
};

/**
 * Merge local session commits and PR commits into a single list. A commit is
 * pushed when EITHER source confirms it: the backend's `pushed` field
 * (computed from the upstream tracking ref) or a SHA match in `prCommits` (PR
 * existence implies the commit is on the remote). The git signal fixes the
 * original bug — pushed commits showing as unpushed when no PR exists — while
 * keeping the PR signal so a commit visible only via a PR (e.g. authored by
 * another contributor, or a rebased SHA the local repo doesn't track yet)
 * still renders as pushed. PR commits not represented locally are appended
 * as pushed. Order: unpushed first, then pushed.
 */
export function mergeCommits(
  localCommits: {
    commit_sha: string;
    commit_message: string;
    insertions: number;
    deletions: number;
    /** Multi-repo: name of the repo this commit was made in. */
    repository_name?: string;
    pushed?: boolean;
    committed_at?: string;
  }[],
  prCommits: {
    sha: string;
    message: string;
    additions: number;
    deletions: number;
    author_date?: string;
  }[],
): MergedCommit[] {
  const shaMatches = (a: string, b: string) => a.startsWith(b) || b.startsWith(a);
  const unpushed: MergedCommit[] = [];
  const pushed: MergedCommit[] = [];
  const matchedPRShas = new Set<string>();
  for (const c of localCommits) {
    const matchesPR = prCommits.some((pr) => shaMatches(pr.sha, c.commit_sha));
    const isPushed = c.pushed === true || matchesPR;
    if (isPushed) {
      pushed.push({ ...c, pushed: true });
    } else {
      unpushed.push({ ...c, pushed: false });
    }
    if (matchesPR) {
      for (const pr of prCommits) {
        if (shaMatches(pr.sha, c.commit_sha)) {
          matchedPRShas.add(pr.sha);
        }
      }
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
        committed_at: pr.author_date,
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

type TaskPRUrlInput = {
  pr_url?: string;
  repository_id?: string;
};

/** Workspace repository id → display name for multi-repo PR/commit grouping. */
export function buildRepoNameById(
  reposByWorkspace: Record<string, Array<{ id: string; name: string }>>,
): Record<string, string> {
  const out: Record<string, string> = {};
  for (const list of Object.values(reposByWorkspace)) {
    for (const r of list) {
      out[r.id] = r.name;
    }
  }
  return out;
}

/** Merge synced TaskPR URLs and client-only pending PR URLs by repo display name. */
export function buildPrByRepoMap(
  taskPRs: TaskPRUrlInput[] | undefined,
  repoNameById: Record<string, string>,
  pendingByRepo: Record<string, string> | undefined,
): Record<string, string | undefined> {
  const map: Record<string, string | undefined> = {};
  if (taskPRs) {
    for (const pr of taskPRs) {
      if (!pr.pr_url) continue;
      const repoKey = pr.repository_id ? (repoNameById[pr.repository_id] ?? "") : "";
      map[repoKey] = map[repoKey] ?? pr.pr_url;
    }
  }
  if (pendingByRepo) {
    for (const [repoKey, url] of Object.entries(pendingByRepo)) {
      if (url) map[repoKey] = map[repoKey] ?? url;
    }
  }
  const legacySingleRepo = taskPRs?.find((p) => p.pr_url && !p.repository_id);
  if (!map[""] && legacySingleRepo?.pr_url) {
    map[""] = legacySingleRepo.pr_url;
  }
  return map;
}
