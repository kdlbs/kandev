import { useMemo } from "react";
import { useSessionGitStatus, useSessionGitStatusByRepo } from "./use-session-git-status";
import { useCumulativeDiff } from "./use-cumulative-diff";
import { useActiveTaskPR } from "@/hooks/domains/github/use-task-pr";
import { usePRDiff } from "@/hooks/domains/github/use-pr-diff";
import { normalizeDiffContent } from "@/components/review/types";
import type { ReviewFile } from "@/components/review/types";
import type { PRDiffFile } from "@/lib/types/github";

export type ReviewSource = "uncommitted" | "committed" | "pr";

export type SourceCounts = Record<ReviewSource, number>;

type UncommittedFile = {
  diff?: string;
  diff_skip_reason?: ReviewFile["diff_skip_reason"];
  status?: string;
  additions?: number;
  deletions?: number;
  staged?: boolean;
};

type CumulativeFile = { diff?: string; status?: string; additions?: number; deletions?: number };

function addUncommittedFiles(
  fileMap: Map<string, ReviewFile>,
  files: Record<string, UncommittedFile>,
  repositoryName?: string,
) {
  for (const [path, file] of Object.entries(files)) {
    const diff = file.diff ? normalizeDiffContent(file.diff) : "";
    const skipReason = file.diff_skip_reason;
    if (!diff && !skipReason) continue;
    fileMap.set(path, {
      path,
      diff,
      status: file.status ?? "modified",
      additions: file.additions ?? 0,
      deletions: file.deletions ?? 0,
      staged: file.staged ?? false,
      source: "uncommitted",
      diff_skip_reason: skipReason,
      repository_name: repositoryName,
    });
  }
}

function addCumulativeFiles(
  fileMap: Map<string, ReviewFile>,
  files: Record<string, CumulativeFile>,
) {
  for (const [path, file] of Object.entries(files)) {
    if (fileMap.has(path)) continue;
    const diff = file.diff ? normalizeDiffContent(file.diff) : "";
    if (!diff) continue;
    fileMap.set(path, {
      path,
      diff,
      status: file.status || "modified",
      additions: file.additions ?? 0,
      deletions: file.deletions ?? 0,
      staged: false,
      source: "committed",
    });
  }
}

function prFileStatus(status: string): "added" | "deleted" | "modified" {
  if (status === "added") return "added";
  if (status === "removed") return "deleted";
  return "modified";
}

function addPRFiles(fileMap: Map<string, ReviewFile>, files: PRDiffFile[]) {
  for (const file of files) {
    if (fileMap.has(file.filename)) continue;
    const diff = file.patch ? normalizeDiffContent(file.patch) : "";
    if (!diff) continue;
    fileMap.set(file.filename, {
      path: file.filename,
      diff,
      status: prFileStatus(file.status),
      additions: file.additions ?? 0,
      deletions: file.deletions ?? 0,
      staged: false,
      source: "pr",
    });
  }
}

export type BuildReviewSourcesInput = {
  gitStatus: { files?: Record<string, UncommittedFile> } | undefined;
  statusByRepo:
    | Array<{ repository_name: string; status: { files?: Record<string, UncommittedFile> } }>
    | undefined;
  cumulativeDiff: { files?: Record<string, CumulativeFile> } | null;
  prDiffFiles: PRDiffFile[] | undefined;
};

export type BuildReviewSourcesResult = {
  allFiles: ReviewFile[];
  sourceCounts: SourceCounts;
};

/**
 * Pure helper that merges the three diff sources into one sorted, deduped
 * list and computes per-source counts. PR files come first into the map so
 * that uncommitted and committed entries overwrite same-path PR rows
 * (dedup priority: uncommitted > committed > PR).
 */
export function buildReviewSources(input: BuildReviewSourcesInput): BuildReviewSourcesResult {
  const { gitStatus, statusByRepo, cumulativeDiff, prDiffFiles } = input;
  const fileMap = new Map<string, ReviewFile>();

  if (prDiffFiles && prDiffFiles.length > 0) addPRFiles(fileMap, prDiffFiles);

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

  if (cumulativeDiff?.files) addCumulativeFiles(fileMap, cumulativeDiff.files);

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
  /** Raw single-repo gitStatus (kept for `useAutoCloseWhenEmpty` consumers). */
  gitStatus: ReturnType<typeof useSessionGitStatus>;
};

/**
 * Multi-source merge hook. Aggregates uncommitted / committed / PR diffs
 * into one sorted ReviewFile[] tagged with `.source`. Shared by
 * `TaskChangesPanel` (diff viewer) and `MobileChangesTabs` (source filter).
 *
 * Returns counts per source so consumers can render tab badges without
 * re-running the merge.
 */
export function useReviewSources(sessionId: string | null | undefined): UseReviewSourcesResult {
  const gitStatus = useSessionGitStatus(sessionId ?? null);
  const statusByRepo = useSessionGitStatusByRepo(sessionId ?? null);
  const { diff: cumulativeDiff, loading: cumulativeLoading } = useCumulativeDiff(sessionId ?? null);
  const pr = useActiveTaskPR();
  const { files: prDiffFiles, loading: prDiffLoading } = usePRDiff(
    pr?.owner ?? null,
    pr?.repo ?? null,
    pr?.pr_number ?? null,
  );

  const { allFiles, sourceCounts } = useMemo(
    () =>
      buildReviewSources({
        gitStatus,
        statusByRepo,
        cumulativeDiff: cumulativeDiff as { files?: Record<string, CumulativeFile> } | null,
        prDiffFiles: prDiffFiles.length > 0 ? prDiffFiles : undefined,
      }),
    [gitStatus, statusByRepo, cumulativeDiff, prDiffFiles],
  );

  // Keep `hasPR` keyed on the existence of a TaskPR row, not on whether the
  // PR diff files have loaded yet — avoids the tab bar reflowing the moment
  // the GitHub PR diff hydrates.
  const hasPR = !!pr;

  return {
    allFiles,
    sourceCounts,
    hasPR,
    cumulativeLoading,
    prDiffLoading,
    gitStatus,
  };
}

export type { CumulativeFile, UncommittedFile };
