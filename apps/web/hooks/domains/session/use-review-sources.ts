import { useEffect, useMemo } from "react";
import { useAppStore } from "@/components/state-provider";
import { useSessionGitStatus, useSessionGitStatusByRepo } from "./use-session-git-status";
import { useCumulativeDiff } from "./use-cumulative-diff";
import {
  findReviewPRByKey,
  useReviewPRSelection,
} from "@/hooks/domains/github/use-review-pr-selection";
import { usePRDiff } from "@/hooks/domains/github/use-pr-diff";
import { usePRReviewRepositoryIdentity } from "@/hooks/domains/github/use-pr-review-repository-identity";
import { useTaskRepositories } from "@/hooks/domains/kanban/use-task-repositories";
import {
  getCumulativeReviewRepositoryNames,
  isReviewMultiRepo,
  normalizeDiffContent,
  reviewFileKey,
  splitReviewFileKey,
  suppressAvailableGitlinkFiles,
} from "@/components/review/types";
import { createDebugLogger } from "@/lib/debug/log";
import type { ReviewChangeFacet, ReviewFile } from "@/components/review/types";
import type { PRDiffFile, TaskPR } from "@/lib/types/github";
import { normalizeFileChangeStatus } from "@/lib/utils/file-change-status";
import { prTaskKey } from "@/components/github/pr-utils";

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
  is_submodule?: boolean;
  staged_change?: UncommittedChangeFacet;
  unstaged_change?: UncommittedChangeFacet;
};

type UncommittedChangeFacet = {
  diff?: string;
  diff_skip_reason?: ReviewFile["diff_skip_reason"];
  status?: string;
  old_path?: string;
  additions?: number;
  deletions?: number;
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
  is_submodule?: boolean;
};

function addUncommittedFiles(
  fileMap: Map<string, ReviewFile>,
  files: Record<string, UncommittedFile>,
  repositoryName?: string,
  isSubmodule?: boolean,
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
      staged_change: normalizeUncommittedFacet(file.staged_change),
      unstaged_change: normalizeUncommittedFacet(file.unstaged_change),
      repository_name: repositoryName,
      is_submodule: file.is_submodule ?? isSubmodule,
    });
  }
}

function normalizeUncommittedFacet(
  facet: UncommittedChangeFacet | undefined,
): ReviewChangeFacet | undefined {
  if (!facet) return undefined;
  return {
    diff: facet.diff ? normalizeDiffContent(facet.diff) : "",
    status: normalizeFileChangeStatus(facet.status),
    additions: facet.additions ?? 0,
    deletions: facet.deletions ?? 0,
    old_path: facet.old_path,
    diff_skip_reason: facet.diff_skip_reason,
  };
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
    // A bare path is the workspace-root scope in a multi-repository review;
    // it must not hide a same-named file from a child repository. The exact
    // key still preserves uncommitted precedence for the owning scope, while
    // useRepositoryKeys=false keeps legacy single-repository deduplication.
    if (fileMap.has(key) || uncommittedPaths.has(key)) continue;
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
      is_submodule: file.is_submodule,
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
    if (fileMap.has(key) || uncommittedPaths.has(key)) continue;
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
      is_submodule: file.is_submodule,
    });
  }
}

function collectPathsFromFiles(
  paths: Set<string>,
  files: Record<string, UncommittedFile>,
  repositoryName?: string,
  useRepositoryKeys: boolean = true,
): void {
  for (const path of Object.keys(files)) {
    const scope = useRepositoryKeys ? repositoryName : undefined;
    paths.add(reviewFileKey({ path, repository_name: scope }));
  }
}

function collectUncommittedPaths(
  statusByRepo: BuildReviewSourcesInput["statusByRepo"],
  gitStatus: BuildReviewSourcesInput["gitStatus"],
  useRepositoryKeys: boolean,
): Set<string> {
  const paths = new Set<string>();
  if (statusByRepo && statusByRepo.length > 0) {
    for (const { repository_name, status } of statusByRepo) {
      if (status?.files)
        collectPathsFromFiles(
          paths,
          status.files as Record<string, UncommittedFile>,
          repository_name,
          useRepositoryKeys,
        );
    }
  } else if (gitStatus?.files) {
    collectPathsFromFiles(
      paths,
      gitStatus.files as Record<string, UncommittedFile>,
      useRepositoryKeys ? "" : undefined,
      useRepositoryKeys,
    );
  }
  return paths;
}

export type BuildReviewSourcesInput = {
  gitStatus: { files?: Record<string, UncommittedFile>; is_submodule?: boolean } | undefined;
  statusByRepo:
    | Array<{
        repository_name: string;
        status: { files?: Record<string, UncommittedFile>; is_submodule?: boolean };
      }>
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

function normalizeMultiRepoStatusByRepo(
  input: NormalizeReviewStatusSourcesInput,
  hasRootStatus: boolean,
) {
  if (input.statusByRepo.length === 0) return undefined;
  if (hasRootStatus || input.taskRepositoryCount > 1) return input.statusByRepo;
  if (!input.gitStatus?.files) return input.statusByRepo;
  return [{ repository_name: "", status: input.gitStatus }, ...input.statusByRepo];
}

export function normalizeReviewStatusSources(input: NormalizeReviewStatusSourcesInput) {
  const namedStatuses = input.statusByRepo.filter((entry) => entry.repository_name !== "");
  const useRepositoryKeys = isReviewMultiRepo(
    input.taskRepositoryCount,
    input.statusByRepo
      .map((entry) => entry.repository_name)
      .concat(Array.from(input.cumulativeRepositoryNames ?? [])),
  );
  if (useRepositoryKeys) {
    const hasRootStatus = input.statusByRepo.some((entry) => entry.repository_name === "");
    return {
      useRepositoryKeys: true,
      prRepoName: input.resolvedPRRepoName,
      normalizedGitStatus: input.gitStatus,
      normalizedStatusByRepo: normalizeMultiRepoStatusByRepo(input, hasRootStatus),
    };
  }
  return {
    useRepositoryKeys: false,
    prRepoName: undefined,
    normalizedGitStatus: input.gitStatus?.files ? input.gitStatus : namedStatuses[0]?.status,
    normalizedStatusByRepo: undefined,
  };
}

function addUncommittedSources(
  fileMap: Map<string, ReviewFile>,
  statusByRepo: BuildReviewSourcesInput["statusByRepo"],
  gitStatus: BuildReviewSourcesInput["gitStatus"],
  useRepositoryKeys: boolean,
) {
  if (statusByRepo && statusByRepo.length > 0) {
    for (const { repository_name, status } of statusByRepo) {
      if (status?.files) {
        addUncommittedFiles(
          fileMap,
          status.files as Record<string, UncommittedFile>,
          useRepositoryKeys ? repository_name : undefined,
          status.is_submodule,
        );
      }
    }
    return;
  }
  if (gitStatus?.files) {
    addUncommittedFiles(
      fileMap,
      gitStatus.files as Record<string, UncommittedFile>,
      useRepositoryKeys ? "" : undefined,
      gitStatus.is_submodule,
    );
  }
}

function addCommittedAndPRSources(
  fileMap: Map<string, ReviewFile>,
  input: BuildReviewSourcesInput,
  uncommittedPaths: Set<string>,
) {
  const { cumulativeDiff, prDiffFiles, prRepoName, useRepositoryKeys = true } = input;
  if (cumulativeDiff?.files) {
    addCumulativeFiles(
      fileMap,
      cumulativeDiff.files,
      uncommittedPaths,
      useRepositoryKeys,
      cumulativeDiff.base_commit,
    );
  }
  if (prDiffFiles && prDiffFiles.length > 0) {
    addPRFiles(fileMap, prDiffFiles, uncommittedPaths, useRepositoryKeys ? prRepoName : undefined);
  }
}

function sortReviewFiles(fileMap: Map<string, ReviewFile>): ReviewFile[] {
  return Array.from(fileMap.values()).sort((a, b) => {
    const repoCmp = (a.repository_name ?? "").localeCompare(b.repository_name ?? "");
    if (repoCmp !== 0) return repoCmp;
    return a.path.localeCompare(b.path);
  });
}

function countReviewSources(files: ReviewFile[]): SourceCounts {
  const sourceCounts: SourceCounts = { uncommitted: 0, committed: 0, pr: 0 };
  for (const file of files) sourceCounts[file.source]++;
  return sourceCounts;
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
  const { gitStatus, statusByRepo } = input;
  const fileMap = new Map<string, ReviewFile>();
  const useRepositoryKeys = input.useRepositoryKeys ?? true;

  const uncommittedPaths = collectUncommittedPaths(statusByRepo, gitStatus, useRepositoryKeys);
  addUncommittedSources(fileMap, statusByRepo, gitStatus, useRepositoryKeys);
  addCommittedAndPRSources(fileMap, input, uncommittedPaths);
  const allFiles = suppressAvailableGitlinkFiles(sortReviewFiles(fileMap));
  return { allFiles, sourceCounts: countReviewSources(allFiles) };
}

export type UseReviewSourcesResult = {
  allFiles: ReviewFile[];
  sourceCounts: SourceCounts;
  hasPR: boolean;
  cumulativeLoading: boolean;
  prDiffLoading: boolean;
  prDiffError: string | null;
  refreshPRDiff: () => void;
  prs: TaskPR[];
  selectedPR: TaskPR | null;
  selectedPRKey: string | null;
  selectPR: (pr: TaskPR) => void;
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
   * The selected PR is used unless file mode supplies an explicit PR key.
   */
  rawPRFiles: ReviewFile[];
};

function useReviewRepositoryContext(
  sessionId: string | null | undefined,
  pr: TaskPR | null,
  gitStatus: ReturnType<typeof useSessionGitStatus>,
  statusByRepo: ReturnType<typeof useSessionGitStatusByRepo>,
  cumulativeDiff: ReturnType<typeof useCumulativeDiff>["diff"],
) {
  const activeTaskId = useAppStore((state) => state.tasks.activeTaskId);
  const taskRepositories = useTaskRepositories(activeTaskId);
  const resolvedPRRepoName = usePRReviewRepositoryIdentity(activeTaskId, sessionId, pr);
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
export function useReviewSources(
  sessionId: string | null | undefined,
  explicitPRKey?: string,
): UseReviewSourcesResult {
  const gitStatus = useSessionGitStatus(sessionId ?? null);
  const statusByRepo = useSessionGitStatusByRepo(sessionId ?? null);
  const { diff: cumulativeDiff, loading: cumulativeLoading } = useCumulativeDiff(sessionId ?? null);
  const activeTaskId = useAppStore((state) => state.tasks.activeTaskId);
  const { prs, selectedPR, selectedKey, selectPR } = useReviewPRSelection(activeTaskId);
  const pr = explicitPRKey ? findReviewPRByKey(prs, explicitPRKey) : selectedPR;
  const { normalizedGitStatus, normalizedStatusByRepo, prRepoName, useRepositoryKeys } =
    useReviewRepositoryContext(sessionId, pr, gitStatus, statusByRepo, cumulativeDiff);
  const {
    files: prDiffFiles,
    loading: prDiffLoading,
    error: prDiffError,
    refresh: refreshPRDiff,
  } = usePRDiff(pr?.owner ?? null, pr?.repo ?? null, pr?.pr_number ?? null, pr?.last_synced_at);

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
    prDiffError,
    refreshPRDiff,
    prs,
    selectedPR: pr,
    selectedPRKey: pr ? prTaskKey(pr) : selectedKey,
    selectPR,
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
