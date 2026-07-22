import { useEffect, useMemo } from "react";
import { useAppStore } from "@/components/state-provider";
import { useSessionGitStatus, useSessionGitStatusByRepo } from "./use-session-git-status";
import { useCumulativeDiff } from "./use-cumulative-diff";
import { useActiveTaskPR } from "@/hooks/domains/github/use-task-pr";
import { usePRDiff } from "@/hooks/domains/github/use-pr-diff";
import { useTaskRepositories } from "@/hooks/domains/kanban/use-task-repositories";
import { useRepository } from "@/hooks/domains/workspace/use-repository";
import {
  getCumulativeReviewRepositoryNames,
  isReviewMultiRepo,
  normalizeDiffContent,
  resolvePRReviewRepositoryName,
  reviewFileKey,
  splitReviewFileKey,
} from "@/components/review/types";
import { createDebugLogger } from "@/lib/debug/log";
import type { ReviewFile } from "@/components/review/types";
import type { PRDiffFile } from "@/lib/types/github";
import { normalizeFileChangeStatus } from "@/lib/utils/file-change-status";

const debug = createDebugLogger("review:sources");

export type ReviewSource = "uncommitted" | "committed" | "pr";

export type SourceCounts = Record<ReviewSource, number>;

type UncommittedFile = {
  diff?: string;
  diff_skip_reason?: ReviewFile["diff_skip_reason"];
  status?: string;
  old_path?: string;
  additions?: number;
  deletions?: number;
  staged?: boolean;
};

type CumulativeFile = {
  diff?: string;
  diff_skip_reason?: ReviewFile["diff_skip_reason"];
  status?: string;
  old_path?: string;
  additions?: number;
  deletions?: number;
  repository_name?: string;
  base_ref?: string;
  /**
   * Repo-relative path. Stamped by the backend's multi-repo cumulative-diff
   * merge (`mergeCumulativeFiles` in agentctl); single-repo payloads omit
   * this and carry the path only on the map key.
   */
  path?: string;
};

function addUncommittedFiles(
  fileMap: Map<string, ReviewFile>,
  files: Record<string, UncommittedFile>,
  repositoryName?: string,
) {
  for (const [path, file] of Object.entries(files)) {
    const diff = file.diff ? normalizeDiffContent(file.diff) : "";
    const skipReason = file.diff_skip_reason;
    const key = reviewFileKey({ path, repository_name: repositoryName });
    fileMap.set(key, {
      path,
      diff,
      status: normalizeFileChangeStatus(file.status),
      additions: file.additions ?? 0,
      deletions: file.deletions ?? 0,
      staged: file.staged ?? false,
      source: "uncommitted",
      old_path: file.old_path,
      diff_skip_reason: skipReason,
      repository_name: repositoryName,
    });
  }
}

function addCumulativeFiles(
  fileMap: Map<string, ReviewFile>,
  files: Record<string, CumulativeFile>,
  uncommittedPaths: Set<string>,
  useRepositoryKeys: boolean,
  defaultBaseRef?: string,
) {
  for (const [mapKey, file] of Object.entries(files)) {
    // Multi-repo cumulative payloads use a NUL-composite `<repo>\x00<path>`
    // map key and stamp the clean repo-relative path on `file.path`.
    // Single-repo keeps the bare path on the map key with no `file.path`.
    // Prefer the stamped value so the composite key doesn't bleed into the
    // displayed path; fall back to the map key for single-repo.
    const path = file.path ?? splitReviewFileKey(mapKey).path;
    const repositoryName = useRepositoryKeys ? file.repository_name : undefined;
    const key = reviewFileKey({ path, repository_name: repositoryName });
    const hasRepoUnawareCollision = key !== path && fileMap.has(path);
    if (fileMap.has(key) || uncommittedPaths.has(key) || hasRepoUnawareCollision) continue;
    const diff = file.diff ? normalizeDiffContent(file.diff) : "";
    fileMap.set(key, {
      path,
      diff,
      status: normalizeFileChangeStatus(file.status),
      additions: file.additions ?? 0,
      deletions: file.deletions ?? 0,
      staged: false,
      source: "committed",
      old_path: file.old_path,
      diff_skip_reason: file.diff_skip_reason,
      repository_name: repositoryName,
      base_ref: file.base_ref ?? defaultBaseRef,
    });
  }
}

function addPRFiles(
  fileMap: Map<string, ReviewFile>,
  files: PRDiffFile[],
  uncommittedPaths: Set<string>,
  repoName?: string,
) {
  for (const file of files) {
    const key = reviewFileKey({ path: file.filename, repository_name: repoName });
    const hasRepoUnawareCollision = key !== file.filename && fileMap.has(file.filename);
    if (fileMap.has(key) || uncommittedPaths.has(key) || hasRepoUnawareCollision) continue;
    const diff = file.patch ? normalizeDiffContent(file.patch) : "";
    fileMap.set(key, {
      path: file.filename,
      diff,
      status: normalizeFileChangeStatus(file.status),
      additions: file.additions ?? 0,
      deletions: file.deletions ?? 0,
      staged: false,
      source: "pr",
      old_path: file.old_path,
      repository_name: repoName,
    });
  }
}

function collectPathsFromFiles(
  paths: Set<string>,
  files: Record<string, UncommittedFile>,
  repositoryName?: string,
): void {
  for (const path of Object.keys(files)) {
    // Always add bare path (for deduping repo-unaware sources like cumulative
    // diffs that may not carry repository_name).
    paths.add(path);
    // Also add composite key (for repo-aware dedup when sources carry repo info).
    if (repositoryName) paths.add(reviewFileKey({ path, repository_name: repositoryName }));
  }
}

function collectUncommittedPaths(
  statusByRepo: BuildReviewSourcesInput["statusByRepo"],
  gitStatus: BuildReviewSourcesInput["gitStatus"],
): Set<string> {
  const paths = new Set<string>();
  if (statusByRepo && statusByRepo.length > 0) {
    for (const { repository_name, status } of statusByRepo) {
      if (status?.files)
        collectPathsFromFiles(
          paths,
          status.files as Record<string, UncommittedFile>,
          repository_name || undefined,
        );
    }
  } else if (gitStatus?.files) {
    collectPathsFromFiles(paths, gitStatus.files as Record<string, UncommittedFile>);
  }
  return paths;
}

export type BuildReviewSourcesInput = {
  gitStatus: { files?: Record<string, UncommittedFile> } | undefined;
  statusByRepo:
    | Array<{ repository_name: string; status: { files?: Record<string, UncommittedFile> } }>
    | undefined;
  cumulativeDiff: {
    base_commit?: string;
    files?: Record<string, CumulativeFile>;
  } | null;
  prDiffFiles: PRDiffFile[] | undefined;
  /** Repository name for the primary PR — used to composite-key PR files so
   *  same-named files in different repos are not incorrectly deduped. */
  prRepoName?: string;
  /** Whether review identities include repository_name. False keeps every
   * source bare during legacy/single-repository status hydration. */
  useRepositoryKeys?: boolean;
};

export type BuildReviewSourcesResult = {
  allFiles: ReviewFile[];
  sourceCounts: SourceCounts;
};

type NormalizeReviewStatusSourcesInput = {
  gitStatus: BuildReviewSourcesInput["gitStatus"];
  statusByRepo: NonNullable<BuildReviewSourcesInput["statusByRepo"]>;
  taskRepositoryCount: number;
  resolvedPRRepoName?: string;
  cumulativeRepositoryNames?: Iterable<string>;
};

export function normalizeReviewStatusSources(input: NormalizeReviewStatusSourcesInput) {
  const namedStatuses = input.statusByRepo.filter((entry) => entry.repository_name !== "");
  const useRepositoryKeys = isReviewMultiRepo(
    input.taskRepositoryCount,
    namedStatuses
      .map((entry) => entry.repository_name)
      .concat(Array.from(input.cumulativeRepositoryNames ?? [])),
  );
  if (useRepositoryKeys) {
    return {
      useRepositoryKeys: true,
      prRepoName: input.resolvedPRRepoName,
      normalizedGitStatus: input.gitStatus,
      normalizedStatusByRepo: namedStatuses.length > 0 ? namedStatuses : undefined,
    };
  }
  return {
    useRepositoryKeys: false,
    prRepoName: undefined,
    normalizedGitStatus: input.gitStatus?.files ? input.gitStatus : namedStatuses[0]?.status,
    normalizedStatusByRepo: undefined,
  };
}

/**
 * Pure helper that merges the three diff sources into one sorted, deduped
 * list and computes per-source counts. Uncommitted files write first under
 * shared `reviewFileKey` composite keys (multi-repo) or bare paths
 * (single-repo). Committed and PR files write next under the same key shape
 * but skip any path already present in the uncommitted set — preserving
 * dedup priority (uncommitted > committed > PR).
 * Keep priority, dedup keys, and sorting aligned with `buildAllFiles` in
 * `components/review/review-dialog.tsx`.
 */
export function buildReviewSources(input: BuildReviewSourcesInput): BuildReviewSourcesResult {
  const {
    gitStatus,
    statusByRepo,
    cumulativeDiff,
    prDiffFiles,
    prRepoName,
    useRepositoryKeys = true,
  } = input;
  const fileMap = new Map<string, ReviewFile>();

  const uncommittedPaths = collectUncommittedPaths(statusByRepo, gitStatus);

  if (statusByRepo && statusByRepo.length > 0) {
    for (const { repository_name, status } of statusByRepo) {
      if (status?.files) {
        addUncommittedFiles(
          fileMap,
          status.files as Record<string, UncommittedFile>,
          repository_name || undefined,
        );
      }
    }
  } else if (gitStatus?.files) {
    addUncommittedFiles(fileMap, gitStatus.files as Record<string, UncommittedFile>);
  }

  if (cumulativeDiff?.files) {
    addCumulativeFiles(
      fileMap,
      cumulativeDiff.files,
      uncommittedPaths,
      useRepositoryKeys,
      cumulativeDiff.base_commit,
    );
  }

  if (prDiffFiles && prDiffFiles.length > 0)
    addPRFiles(fileMap, prDiffFiles, uncommittedPaths, prRepoName);

  const allFiles = Array.from(fileMap.values()).sort((a, b) => {
    const repoCmp = (a.repository_name ?? "").localeCompare(b.repository_name ?? "");
    if (repoCmp !== 0) return repoCmp;
    return a.path.localeCompare(b.path);
  });

  const sourceCounts: SourceCounts = { uncommitted: 0, committed: 0, pr: 0 };
  for (const f of allFiles) sourceCounts[f.source]++;

  return { allFiles, sourceCounts };
}

export type UseReviewSourcesResult = {
  allFiles: ReviewFile[];
  sourceCounts: SourceCounts;
  hasPR: boolean;
  cumulativeLoading: boolean;
  prDiffLoading: boolean;
  /**
   * Files the backend dropped from the cumulative diff because the range
   * exceeded its file cap (large rebase). Zero when nothing was hidden;
   * surfaced by TaskChangesPanel as a banner.
   */
  truncatedFilesCount: number;
  /** Raw single-repo gitStatus (kept for `useAutoCloseWhenEmpty` consumers). */
  gitStatus: ReturnType<typeof useSessionGitStatus>;
  /**
   * Raw PR diff files as ReviewFile[], NOT deduplicated with uncommitted/committed.
   * Use when displaying the PR-specific diff for a file (e.g. clicking a PR file row),
   * where the same file may also have local changes that would win deduplication in allFiles.
   * NOTE: Only covers the primary PR (single-repo tasks). Multi-PR support is Bug 2.
   */
  rawPRFiles: ReviewFile[];
};

function useReviewRepositoryContext(
  pr: ReturnType<typeof useActiveTaskPR>,
  gitStatus: ReturnType<typeof useSessionGitStatus>,
  statusByRepo: ReturnType<typeof useSessionGitStatusByRepo>,
  cumulativeDiff: ReturnType<typeof useCumulativeDiff>["diff"],
) {
  const activeTaskId = useAppStore((state) => state.tasks.activeTaskId);
  const taskRepositories = useTaskRepositories(activeTaskId);
  const prRepository = useRepository(pr?.repository_id ?? null);
  const resolvedPRRepoName = resolvePRReviewRepositoryName(pr, prRepository?.name);
  return useMemo(
    () =>
      normalizeReviewStatusSources({
        gitStatus,
        statusByRepo,
        taskRepositoryCount: taskRepositories.length,
        resolvedPRRepoName,
        cumulativeRepositoryNames: getCumulativeReviewRepositoryNames(cumulativeDiff?.files),
      }),
    [gitStatus, statusByRepo, taskRepositories.length, resolvedPRRepoName, cumulativeDiff],
  );
}

/**
 * Multi-source merge hook. Aggregates uncommitted / committed / PR diffs
 * into one sorted ReviewFile[] tagged with `.source`. Shared by
 * `TaskChangesPanel` (diff viewer) and `MobileDiffSheet` (source tab bar).
 *
 * Returns counts per source so consumers can render tab badges without
 * re-running the merge.
 */
export function useReviewSources(sessionId: string | null | undefined): UseReviewSourcesResult {
  const gitStatus = useSessionGitStatus(sessionId ?? null);
  const statusByRepo = useSessionGitStatusByRepo(sessionId ?? null);
  const { diff: cumulativeDiff, loading: cumulativeLoading } = useCumulativeDiff(sessionId ?? null);
  const pr = useActiveTaskPR();
  const { normalizedGitStatus, normalizedStatusByRepo, prRepoName, useRepositoryKeys } =
    useReviewRepositoryContext(pr, gitStatus, statusByRepo, cumulativeDiff);
  const { files: prDiffFiles, loading: prDiffLoading } = usePRDiff(
    pr?.owner ?? null,
    pr?.repo ?? null,
    pr?.pr_number ?? null,
  );

  const { allFiles, sourceCounts } = useMemo(
    () =>
      buildReviewSources({
        gitStatus: normalizedGitStatus,
        statusByRepo: normalizedStatusByRepo,
        cumulativeDiff,
        prDiffFiles: prDiffFiles.length > 0 ? prDiffFiles : undefined,
        prRepoName,
        useRepositoryKeys,
      }),
    [
      normalizedGitStatus,
      normalizedStatusByRepo,
      cumulativeDiff,
      prDiffFiles,
      prRepoName,
      useRepositoryKeys,
    ],
  );

  const rawPRFiles = useMemo(() => {
    if (!prDiffFiles || prDiffFiles.length === 0) return [] as ReviewFile[];
    const fileMap = new Map<string, ReviewFile>();
    addPRFiles(fileMap, prDiffFiles, new Set(), prRepoName);
    return Array.from(fileMap.values());
  }, [prDiffFiles, prRepoName]);

  // Keep `hasPR` keyed on the existence of a TaskPR row, not on whether the
  // PR diff files have loaded yet — avoids the tab bar reflowing the moment
  // the GitHub PR diff hydrates.
  const hasPR = !!pr;

  useEffect(() => {
    if (!sessionId) return;
    debug("merged", {
      sessionId,
      total: allFiles.length,
      uncommitted: sourceCounts.uncommitted,
      committed: sourceCounts.committed,
      pr: sourceCounts.pr,
      hasPR,
      hasCumulativeDiff: !!cumulativeDiff,
      cumulativeLoading,
      prDiffLoading,
      prRepo: pr?.repo ?? null,
      prNumber: pr?.pr_number ?? null,
    });
  }, [
    sessionId,
    allFiles.length,
    sourceCounts.uncommitted,
    sourceCounts.committed,
    sourceCounts.pr,
    hasPR,
    cumulativeDiff,
    cumulativeLoading,
    prDiffLoading,
    pr?.repo,
    pr?.pr_number,
  ]);

  return {
    allFiles,
    sourceCounts,
    hasPR,
    cumulativeLoading,
    prDiffLoading,
    gitStatus,
    rawPRFiles,
    truncatedFilesCount: readTruncatedFilesCount(cumulativeDiff),
  };
}

/** Cumulative-diff file-truncation count, defaulting to 0 when absent. Kept as
 *  a module helper so the branches don't add to useReviewSources' complexity. */
function readTruncatedFilesCount(
  cumulativeDiff: { truncated_files_count?: number } | null | undefined,
): number {
  return cumulativeDiff?.truncated_files_count ?? 0;
}

export type { CumulativeFile, UncommittedFile };
