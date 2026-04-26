"use client";

import { useState, useCallback, useEffect, useMemo } from "react";
import { useSessionGitStatus, useSessionGitStatusByRepo } from "./use-session-git-status";
import { useSessionCommits } from "./use-session-commits";
import { useCumulativeDiff } from "./use-cumulative-diff";
import { useGitOperations } from "@/hooks/use-git-operations";
import type {
  FileInfo,
  SessionCommit,
  CumulativeDiff,
} from "@/lib/state/slices/session-runtime/types";
import type { GitOperationResult, PRCreateResult } from "@/hooks/use-git-operations";

export type { GitOperationResult, PRCreateResult };

export type SessionGit = {
  // Branch info
  branch: string | null;
  remoteBranch: string | null;
  ahead: number;
  behind: number;

  // Files (raw FileInfo from store)
  allFiles: FileInfo[];
  unstagedFiles: FileInfo[];
  stagedFiles: FileInfo[];

  // Commits
  commits: SessionCommit[];
  cumulativeDiff: CumulativeDiff | null;
  commitsLoading: boolean;

  // Derived state — single source of truth for all git-dependent UI
  hasUnstaged: boolean;
  hasStaged: boolean;
  hasCommits: boolean;
  hasChanges: boolean; // hasUnstaged || hasStaged
  hasAnything: boolean; // hasChanges || hasCommits
  canStageAll: boolean; // hasUnstaged
  canCommit: boolean; // hasStaged
  canPush: boolean; // ahead > 0
  canCreatePR: boolean; // hasCommits

  // Operation state
  isLoading: boolean;
  loadingOperation: string | null;
  pendingStageFiles: Set<string>;

  // Actions
  pull: (rebase?: boolean) => Promise<GitOperationResult>;
  push: (options?: { force?: boolean; setUpstream?: boolean }) => Promise<GitOperationResult>;
  rebase: (baseBranch: string) => Promise<GitOperationResult>;
  merge: (baseBranch: string) => Promise<GitOperationResult>;
  abort: (operation: "merge" | "rebase") => Promise<GitOperationResult>;
  commit: (message: string, stageAll?: boolean, amend?: boolean) => Promise<GitOperationResult>;
  stage: (paths?: string[]) => Promise<GitOperationResult>;
  stageFile: (paths: string[]) => Promise<GitOperationResult>;
  stageAll: () => Promise<GitOperationResult>;
  unstage: (paths?: string[]) => Promise<GitOperationResult>;
  unstageFile: (paths: string[]) => Promise<GitOperationResult>;
  unstageAll: () => Promise<GitOperationResult>;
  discard: (paths?: string[]) => Promise<GitOperationResult>;
  revertCommit: (commitSHA: string) => Promise<GitOperationResult>;
  renameBranch: (newName: string) => Promise<GitOperationResult>;
  reset: (commitSHA: string, mode: "soft" | "hard") => Promise<GitOperationResult>;
  createPR: (
    title: string,
    body: string,
    baseBranch?: string,
    draft?: boolean,
  ) => Promise<PRCreateResult>;
};

/**
 * Builds the SessionGit's flat file list. For multi-repo workspaces it
 * stamps each FileInfo with its repository_name so consumers can group;
 * for single-repo it returns the legacy single-status files unchanged.
 */
function aggregateFilesAcrossRepos(
  statusByRepo: ReturnType<typeof useSessionGitStatusByRepo>,
  gitStatus: ReturnType<typeof useSessionGitStatus>,
): FileInfo[] {
  if (statusByRepo.length > 0) {
    const out: FileInfo[] = [];
    for (const { repository_name, status } of statusByRepo) {
      if (!status?.files) continue;
      for (const f of Object.values(status.files)) {
        out.push(repository_name ? { ...f, repository_name } : f);
      }
    }
    return out;
  }
  return gitStatus?.files ? Object.values(gitStatus.files) : [];
}

export function useSessionGit(sessionId: string | null | undefined): SessionGit {
  const sid = sessionId ?? null;
  const gitStatus = useSessionGitStatus(sid);
  const statusByRepo = useSessionGitStatusByRepo(sid);
  const { commits, loading: commitsLoading } = useSessionCommits(sid);
  const { diff: cumulativeDiff } = useCumulativeDiff(sid);
  const gitOps = useGitOperations(sid);

  const [pendingStageFiles, setPendingStageFiles] = useState<Set<string>>(new Set());

  // Multi-repo: aggregate files from every repo's per-repo status, stamping
  // each FileInfo with its repository_name so downstream UI can group them.
  // Single-repo (or repo-less) sessions fall back to the legacy single status.
  const allFiles = useMemo<FileInfo[]>(
    () => aggregateFilesAcrossRepos(statusByRepo, gitStatus),
    [statusByRepo, gitStatus],
  );
  const unstagedFiles = useMemo(() => allFiles.filter((f) => !f.staged), [allFiles]);
  const stagedFiles = useMemo(() => allFiles.filter((f) => f.staged), [allFiles]);

  // Clear pending indicators when git status updates (files changed)
  useEffect(() => {
    if (pendingStageFiles.size > 0) setPendingStageFiles(new Set());
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [allFiles]);

  const ahead = gitStatus?.ahead ?? 0;
  const hasUnstaged = unstagedFiles.length > 0;
  const hasStaged = stagedFiles.length > 0;
  const hasCommits = commits.length > 0;

  // eslint-disable-next-line react-hooks/exhaustive-deps -- depend on stable fn ref, not the whole gitOps object
  const stageAll = useCallback(async () => gitOps.stage(), [gitOps.stage]);
  // eslint-disable-next-line react-hooks/exhaustive-deps -- depend on stable fn ref, not the whole gitOps object
  const unstageAll = useCallback(async () => gitOps.unstage(), [gitOps.unstage]);

  const stageFile = useCallback(
    async (paths: string[]) => {
      for (const p of paths) setPendingStageFiles((prev) => new Set(prev).add(p));
      try {
        return await gitOps.stage(paths);
      } catch (err) {
        setPendingStageFiles((prev) => {
          const next = new Set(prev);
          for (const p of paths) next.delete(p);
          return next;
        });
        throw err;
      }
    },
    // eslint-disable-next-line react-hooks/exhaustive-deps -- depend on stable fn ref, not the whole gitOps object
    [gitOps.stage],
  );

  const unstageFile = useCallback(
    async (paths: string[]) => {
      for (const p of paths) setPendingStageFiles((prev) => new Set(prev).add(p));
      try {
        return await gitOps.unstage(paths);
      } catch (err) {
        setPendingStageFiles((prev) => {
          const next = new Set(prev);
          for (const p of paths) next.delete(p);
          return next;
        });
        throw err;
      }
    },
    // eslint-disable-next-line react-hooks/exhaustive-deps -- depend on stable fn ref, not the whole gitOps object
    [gitOps.unstage],
  );

  return {
    branch: gitStatus?.branch ?? null,
    remoteBranch: gitStatus?.remote_branch ?? null,
    ahead,
    behind: gitStatus?.behind ?? 0,

    allFiles,
    unstagedFiles,
    stagedFiles,

    commits,
    cumulativeDiff,
    commitsLoading: commitsLoading ?? false,

    hasUnstaged,
    hasStaged,
    hasCommits,
    hasChanges: hasUnstaged || hasStaged,
    hasAnything: hasUnstaged || hasStaged || hasCommits,
    canStageAll: hasUnstaged,
    canCommit: hasStaged,
    canPush: ahead > 0,
    canCreatePR: hasCommits,

    isLoading: gitOps.isLoading,
    loadingOperation: gitOps.loadingOperation,
    pendingStageFiles,

    pull: gitOps.pull,
    push: gitOps.push,
    rebase: gitOps.rebase,
    merge: gitOps.merge,
    abort: gitOps.abort,
    commit: gitOps.commit,
    stage: gitOps.stage,
    stageFile,
    stageAll,
    unstage: gitOps.unstage,
    unstageFile,
    unstageAll,
    discard: gitOps.discard,
    revertCommit: gitOps.revertCommit,
    renameBranch: gitOps.renameBranch,
    reset: gitOps.reset,
    createPR: gitOps.createPR,
  };
}
