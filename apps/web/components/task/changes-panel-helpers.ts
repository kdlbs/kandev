"use client";

import {
  hashDiff,
  normalizeDiffContent,
  reviewFileKey,
  splitReviewFileKey,
} from "@/components/review/types";
import type { FileInfo } from "@/lib/state/store";
import type { PRDiffFile } from "@/lib/types/github";
import { normalizeFileChangeStatus, type FileChangeStatus } from "@/lib/utils/file-change-status";
import type { PRChangedFile } from "./changes-panel-timeline";
import type { ChangeLayer, CommitDetailTarget } from "./changes-diff-target";
import type { CommitPresentation } from "./commit-row";

export type ChangedFile = {
  path: string;
  status: FileChangeStatus;
  staged: boolean;
  plus: number | undefined;
  minus: number | undefined;
  oldPath: string | undefined;
  /** Repository this file belongs to in multi-repo workspaces; empty for single-repo. */
  repositoryName?: string;
  changeLayer?: ChangeLayer;
};

/**
 * Picks the group label stamped onto each PR's files so the changes panel
 * can show one section per PR for multi-branch tasks.
 *
 * Rules:
 * - Only one PR on the task -> no stamp (the section header is enough).
 * - Multi-repo, one PR per repo -> stamp the repo name (legacy behavior).
 * - Any repo has multiple PRs -> append the branch / PR number so the
 *   group label disambiguates which PR each file belongs to.
 */
export function computePRGroupStamp(args: {
  needsStamp: boolean;
  taskHasMultipleRepos: boolean;
  anyRepoMultiPR: boolean;
  repoName: string;
  branch: string;
  prNumber: number;
}): string {
  if (!args.needsStamp) return "";
  const label = args.branch || `PR #${args.prNumber}`;
  if (args.anyRepoMultiPR && args.taskHasMultipleRepos) {
    return `${args.repoName} · ${label}`;
  }
  if (args.anyRepoMultiPR) {
    return label;
  }
  return args.repoName;
}

export function mapToChangedFiles(files: FileInfo[]): ChangedFile[] {
  return files.map((file) => ({
    path: file.path,
    status: file.status,
    staged: file.staged,
    plus: file.additions,
    minus: file.deletions,
    oldPath: file.old_path,
    repositoryName: file.repository_name,
    changeLayer: file.change_layer,
  }));
}

type CumulativeDiffFiles = Record<
  string,
  {
    path?: string;
    repository_name?: string;
    diff?: string;
    status?: string;
    additions?: number;
    deletions?: number;
  }
>;

function cumulativeFileIdentity(
  mapKey: string,
  file: CumulativeDiffFiles[string],
  useRepositoryKeys: boolean,
) {
  const path = file.path ?? splitReviewFileKey(mapKey).path;
  const repositoryName = useRepositoryKeys ? file.repository_name : undefined;
  return {
    key: reviewFileKey({ path, repository_name: repositoryName }),
    path,
    repositoryName,
  };
}

export type ReviewProgressPRFile = PRDiffFile & { repository_name?: string };

export type ReviewProgressPRSource = {
  repositoryName: string;
  files: PRDiffFile[];
};

export function selectPRFilesForReviewProgress(
  sourcesByPRID: ReadonlyMap<string, ReviewProgressPRSource>,
  primaryPRID: string | undefined,
  useRepositoryKeys: boolean,
): ReviewProgressPRFile[] {
  if (!primaryPRID) return [];
  const source = sourcesByPRID.get(primaryPRID);
  if (!source) return [];
  return source.files.map((file) => ({
    ...file,
    repository_name: useRepositoryKeys ? source.repositoryName : undefined,
  }));
}

function addReviewSource(
  winningDiffs: Map<string, string | undefined>,
  paths: Set<string>,
  source: {
    key: string;
    path: string;
    repositoryName?: string;
    diff?: string;
  },
): void {
  const isScoped = source.repositoryName !== undefined;
  const collidesWithHigherPriority = isScoped
    ? source.repositoryName !== "" && winningDiffs.has(source.path)
    : paths.has(source.path);
  if (winningDiffs.has(source.key) || collidesWithHigherPriority) return;
  winningDiffs.set(source.key, source.diff);
  if (!isScoped) paths.add(source.path);
}

function buildReviewProgressIndex(
  uncommittedFiles: FileInfo[],
  cumulativeDiffFiles: CumulativeDiffFiles | undefined,
  prFiles?: ReviewProgressPRFile[],
  useRepositoryKeys = true,
): Map<string, string | undefined> {
  const winningDiffs = new Map<string, string | undefined>();
  const paths = new Set<string>();
  for (const file of uncommittedFiles) {
    const repositoryName = useRepositoryKeys ? file.repository_name : undefined;
    const key = reviewFileKey({ path: file.path, repository_name: repositoryName });
    addReviewSource(winningDiffs, paths, {
      key,
      path: file.path,
      repositoryName,
      diff: file.diff,
    });
  }
  if (cumulativeDiffFiles) {
    for (const [mapKey, file] of Object.entries(cumulativeDiffFiles)) {
      const identity = cumulativeFileIdentity(mapKey, file, useRepositoryKeys);
      addReviewSource(winningDiffs, paths, {
        ...identity,
        diff: file.diff,
      });
    }
  }
  if (prFiles) {
    for (const file of prFiles) {
      const repositoryName = useRepositoryKeys ? file.repository_name : undefined;
      const key = reviewFileKey({
        path: file.filename,
        repository_name: repositoryName,
      });
      addReviewSource(winningDiffs, paths, {
        key,
        path: file.filename,
        repositoryName,
        diff: file.patch,
      });
    }
  }
  return winningDiffs;
}

export function computeReviewProgress(
  uncommittedFiles: FileInfo[],
  cumulativeDiff: { files?: CumulativeDiffFiles } | null,
  reviews: Map<string, { reviewed: boolean; diffHash?: string }>,
  prFiles?: ReviewProgressPRFile[],
  useRepositoryKeys = true,
) {
  const cumulativeDiffFiles = cumulativeDiff?.files;
  const winningDiffs = buildReviewProgressIndex(
    uncommittedFiles,
    cumulativeDiffFiles,
    prFiles,
    useRepositoryKeys,
  );
  let reviewed = 0;
  for (const [key, diff] of winningDiffs) {
    const state = reviews.get(key);
    if (!state?.reviewed) continue;
    const diffContent = normalizeDiffContent(diff ?? "");
    if (diffContent && state.diffHash && state.diffHash !== hashDiff(diffContent)) continue;
    reviewed++;
  }
  return { reviewedCount: reviewed, totalFileCount: winningDiffs.size };
}

export function computeStagedStats(stagedFiles: FileInfo[]) {
  const adds = stagedFiles.reduce((sum, f) => sum + (f.additions || 0), 0);
  const dels = stagedFiles.reduce((sum, f) => sum + (f.deletions || 0), 0);
  return { stagedFileCount: stagedFiles.length, stagedAdditions: adds, stagedDeletions: dels };
}

export function mapPRFilesToChangedFiles(
  files: PRDiffFile[],
  repositoryName?: string,
  prKey?: string,
): PRChangedFile[] {
  return files.map((file) => ({
    path: file.filename,
    prKey,
    status: normalizeFileChangeStatus(file.status),
    plus: file.additions,
    minus: file.deletions,
    oldPath: file.old_path,
    repository_name: repositoryName ?? "",
  }));
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

/** PR Changes auto-expands only when the diff is small enough to scan inline. */
export const PR_CHANGES_AUTO_EXPAND_MAX_FILES = 5;

/**
 * Computes the first (topmost) visible section so the panel can auto-expand it,
 * mirroring the render precedence in `ChangesPanelTimeline`:
 *   PR (review mode, no local changes) → Unstaged → Staged → Commits.
 *
 * The "review mode" PR section only sits at the top when there are no local
 * working-tree changes; with local changes the PR section moves below Staged and
 * is therefore never first. Large PR diffs (>5 files) skip auto-expanding PR
 * Changes and expand Commits instead. Returns null when nothing is shown.
 */
export function firstVisibleSection(flags: {
  hasPRFiles: boolean;
  hasUnstaged: boolean;
  hasStaged: boolean;
  showCommitsList: boolean;
  prFileCount?: number;
}): FirstVisibleSection {
  const { hasPRFiles, hasUnstaged, hasStaged, showCommitsList, prFileCount = 0 } = flags;
  if (hasPRFiles && !hasUnstaged && !hasStaged) {
    if (prFileCount <= PR_CHANGES_AUTO_EXPAND_MAX_FILES) return "pr";
    if (showCommitsList) return "commits";
    return null;
  }
  if (hasUnstaged) return "unstaged";
  if (hasStaged) return "staged";
  if (showCommitsList) return "commits";
  return null;
}

export type MergedCommit = {
  commit_sha: string;
  commit_message: string;
  insertions: number;
  deletions: number;
  pushed: boolean;
  statsAvailable: boolean;
  detailTarget: CommitDetailTarget;
  /** Multi-repo: name of the repo this commit was made in. Empty for single-repo. */
  repository_name?: string;
  committed_at?: string;
  presentation?: CommitPresentation;
};

export type PRCommitForMerge = {
  sha: string;
  message: string;
  additions: number;
  deletions: number;
  author_date?: string;
  stats_available?: boolean;
  workspace_id: string;
  owner: string;
  repo: string;
  repository_name?: string;
};

function localCommitTarget(commit: {
  commit_sha: string;
  repository_name?: string;
}): CommitDetailTarget {
  return {
    source: "local",
    sha: commit.commit_sha,
    ...(commit.repository_name ? { repo: commit.repository_name } : {}),
  };
}

function githubCommitTarget(commit: PRCommitForMerge): CommitDetailTarget | null {
  if (!commit.workspace_id.trim() || !commit.owner.trim() || !commit.repo.trim()) return null;
  return {
    source: "github",
    sha: commit.sha,
    workspaceId: commit.workspace_id,
    owner: commit.owner,
    repo: commit.repo,
    ...(commit.repository_name ? { repositoryName: commit.repository_name } : {}),
  };
}

function prCommitKey(pr: PRCommitForMerge): string {
  return `${pr.repository_name ?? ""}\0${pr.sha}`;
}

function appendPROnlyCommits(
  prCommits: PRCommitForMerge[],
  matchedPRKeys: Set<string>,
  pushed: MergedCommit[],
): void {
  // The provider endpoint returns its complete history oldest-first. The
  // Changes timeline is newest-first, so provider-only rows must be reversed
  // before they are placed ahead of shared checkout history.
  for (let index = prCommits.length - 1; index >= 0; index--) {
    const pr = prCommits[index];
    if (matchedPRKeys.has(prCommitKey(pr))) continue;
    const mapped = mapCurrentPRCommit(pr);
    if (mapped) pushed.push(mapped);
  }
}

type LocalCommitForMerge = {
  commit_sha: string;
  commit_message: string;
  insertions: number;
  deletions: number;
  repository_name?: string;
  pushed?: boolean;
  committed_at?: string;
};

type LocalMergeBuckets = {
  unpushed: MergedCommit[];
  pushed: MergedCommit[];
  matchedPRKeys: Set<string>;
  localPushedByPRKey: Map<string, MergedCommit>;
};

function commitsMatch(
  local: Pick<LocalCommitForMerge, "commit_sha" | "repository_name">,
  pr: PRCommitForMerge,
): boolean {
  if (!local.commit_sha || !pr.sha) return false;
  const shaMatches = pr.sha.startsWith(local.commit_sha) || local.commit_sha.startsWith(pr.sha);
  return shaMatches && (local.repository_name ?? "") === (pr.repository_name ?? "");
}

function collectLocalCommits(
  localCommits: LocalCommitForMerge[],
  prCommits: PRCommitForMerge[],
): LocalMergeBuckets {
  const buckets: LocalMergeBuckets = {
    unpushed: [],
    pushed: [],
    matchedPRKeys: new Set(),
    localPushedByPRKey: new Map(),
  };
  for (const commit of localCommits) {
    const matchingPRs = prCommits.filter((pr) => commitsMatch(commit, pr));
    const isPushed = commit.pushed === true || matchingPRs.length > 0;
    const merged: MergedCommit = {
      ...commit,
      pushed: isPushed,
      statsAvailable: true,
      detailTarget: localCommitTarget(commit),
    };
    (isPushed ? buckets.pushed : buckets.unpushed).push(merged);
    for (const pr of matchingPRs) {
      const key = prCommitKey(pr);
      buckets.matchedPRKeys.add(key);
      if (isPushed) buckets.localPushedByPRKey.set(key, merged);
    }
  }
  return buckets;
}

function orderSingleRepoPushedCommits(
  prCommits: PRCommitForMerge[],
  providerOnly: MergedCommit[],
  pushed: MergedCommit[],
  localPushedByPRKey: Map<string, MergedCommit>,
): MergedCommit[] {
  const ordered: MergedCommit[] = [];
  const added = new Set<MergedCommit>();
  const providerOnlyBySHA = new Map(providerOnly.map((commit) => [commit.commit_sha, commit]));
  for (const pr of [...prCommits].reverse()) {
    const local = localPushedByPRKey.get(prCommitKey(pr));
    if (local && !added.has(local)) {
      ordered.push(local);
      added.add(local);
      continue;
    }
    const provider = providerOnlyBySHA.get(pr.sha);
    if (provider && !added.has(provider)) {
      ordered.push(provider);
      added.add(provider);
    }
  }
  ordered.push(...pushed.filter((commit) => !added.has(commit)));
  return ordered;
}

function repositoryKey(repositoryName: string | undefined): string {
  return repositoryName ?? "";
}

function orderPushedCommitsByRepository(
  prCommits: PRCommitForMerge[],
  providerOnly: MergedCommit[],
  pushed: MergedCommit[],
  localPushedByPRKey: Map<string, MergedCommit>,
): MergedCommit[] {
  const repositoryOrder: string[] = [];
  const seenRepositories = new Set<string>();
  const pushedByRepository = new Map<string, MergedCommit[]>();
  const providerOnlyByRepository = new Map<string, MergedCommit[]>();
  const prByRepository = new Map<string, PRCommitForMerge[]>();

  const addRepository = (key: string) => {
    if (seenRepositories.has(key)) return;
    seenRepositories.add(key);
    repositoryOrder.push(key);
  };
  const addCommit = (map: Map<string, MergedCommit[]>, commit: MergedCommit) => {
    const key = repositoryKey(commit.repository_name);
    addRepository(key);
    const commits = map.get(key) ?? [];
    commits.push(commit);
    map.set(key, commits);
  };

  for (const commit of pushed) addCommit(pushedByRepository, commit);
  for (const commit of providerOnly) addCommit(providerOnlyByRepository, commit);
  for (const pr of prCommits) {
    const key = repositoryKey(pr.repository_name);
    addRepository(key);
    const commits = prByRepository.get(key) ?? [];
    commits.push(pr);
    prByRepository.set(key, commits);
  }

  const ordered: MergedCommit[] = [];
  for (const key of repositoryOrder) {
    const repoPRCommits = prByRepository.get(key) ?? [];
    const repoProviderOnly = providerOnlyByRepository.get(key) ?? [];
    const repoPushed = pushedByRepository.get(key) ?? [];
    const hasLocalAnchor = repoPRCommits.some((pr) => localPushedByPRKey.has(prCommitKey(pr)));
    if (!hasLocalAnchor) {
      ordered.push(...repoPushed, ...repoProviderOnly);
      continue;
    }
    ordered.push(
      ...orderSingleRepoPushedCommits(
        repoPRCommits,
        repoProviderOnly,
        repoPushed,
        localPushedByPRKey,
      ),
    );
  }
  return ordered;
}

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
  localCommits: LocalCommitForMerge[],
  prCommits: PRCommitForMerge[],
): MergedCommit[] {
  const { unpushed, pushed, matchedPRKeys, localPushedByPRKey } = collectLocalCommits(
    localCommits,
    prCommits,
  );
  const providerOnly: MergedCommit[] = [];
  appendPROnlyCommits(prCommits, matchedPRKeys, providerOnly);
  return [
    ...unpushed,
    ...orderPushedCommitsByRepository(prCommits, providerOnly, pushed, localPushedByPRKey),
  ];
}

function mapLocalCheckoutCommit(commit: {
  commit_sha: string;
  commit_message: string;
  insertions: number;
  deletions: number;
  repository_name?: string;
  committed_at?: string;
}): MergedCommit {
  return {
    ...commit,
    pushed: false,
    statsAvailable: true,
    detailTarget: localCommitTarget(commit),
    presentation: "local_checkout",
  };
}

function mapCurrentPRCommit(commit: PRCommitForMerge): MergedCommit | null {
  const detailTarget = githubCommitTarget(commit);
  if (!detailTarget) return null;
  return {
    commit_sha: commit.sha,
    commit_message: commit.message,
    insertions: commit.additions,
    deletions: commit.deletions,
    pushed: true,
    statsAvailable: commit.stats_available === true,
    detailTarget,
    presentation: "current_pr",
    ...(commit.repository_name ? { repository_name: commit.repository_name } : {}),
    committed_at: commit.author_date,
  };
}

function reverseCommitsByRepository(commits: PRCommitForMerge[]): PRCommitForMerge[] {
  const repositoryOrder: string[] = [];
  const commitsByRepository = new Map<string, PRCommitForMerge[]>();
  for (const commit of commits) {
    const key = `${commit.owner}/${commit.repo}`;
    const repositoryCommits = commitsByRepository.get(key);
    if (repositoryCommits) {
      repositoryCommits.push(commit);
      continue;
    }
    commitsByRepository.set(key, [commit]);
    repositoryOrder.push(key);
  }
  return repositoryOrder.flatMap((key) => [...(commitsByRepository.get(key) ?? [])].reverse());
}

/**
 * Keeps provider and checkout rows independent after a proven history drift.
 * No SHA, message, or metadata matching occurs in this mode.
 */
export function separateCommitHistories(
  localCommits: {
    commit_sha: string;
    commit_message: string;
    insertions: number;
    deletions: number;
    repository_name?: string;
    committed_at?: string;
  }[],
  prCommits: PRCommitForMerge[],
): { providerCommits: MergedCommit[]; localCommits: MergedCommit[] } {
  return {
    providerCommits: reverseCommitsByRepository(prCommits).flatMap((commit) => {
      const mapped = mapCurrentPRCommit(commit);
      return mapped ? [mapped] : [];
    }),
    localCommits: localCommits.map(mapLocalCheckoutCommit),
  };
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
