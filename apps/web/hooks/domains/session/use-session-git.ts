"use client";

import { useState, useCallback, useEffect, useMemo } from "react";
import { useSessionGitStatus } from "./use-session-git-status";
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
  pendingStageFiles: Set<string>;

  // Actions
  pull: (rebase?: boolean) => Promise<GitOperationResult>;
  push: (options?: { force?: boolean; setUpstream?: boolean }) => Promise<GitOperationResult>;
  rebase: (baseBranch: string) => Promise<GitOperationResult>;
  merge: (baseBranch: string) => Promise<GitOperationResult>;
  abort: (operation: "merge" | "rebase") => Promise<GitOperationResult>;
  commit: (message: string, stageAll?: boolean) => Promise<GitOperationResult>;
  stage: (paths?: string[]) => Promise<GitOperationResult>;
  stageAll: () => Promise<GitOperationResult>;
  unstage: (paths?: string[]) => Promise<GitOperationResult>;
  discard: (paths?: string[]) => Promise<GitOperationResult>;
  createPR: (
    title: string,
    body: string,
    baseBranch?: string,
    draft?: boolean,
  ) => Promise<PRCreateResult>;
};

export function useSessionGit(sessionId: string | null | undefined): SessionGit {
  const sid = sessionId ?? null;
  const gitStatus = useSessionGitStatus(sid);
  const { commits, loading: commitsLoading } = useSessionCommits(sid);
  const { diff: cumulativeDiff } = useCumulativeDiff(sid);
  const gitOps = useGitOperations(sid);

  const [pendingStageFiles, setPendingStageFiles] = useState<Set<string>>(new Set());

  const allFiles = useMemo<FileInfo[]>(
    () => (gitStatus?.files ? Object.values(gitStatus.files) : []),
    [gitStatus],
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

  const stageAll = useCallback(async () => gitOps.stage(), [gitOps]);

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
    pendingStageFiles,

    pull: gitOps.pull,
    push: gitOps.push,
    rebase: gitOps.rebase,
    merge: gitOps.merge,
    abort: gitOps.abort,
    commit: gitOps.commit,
    stage: gitOps.stage,
    stageAll,
    unstage: gitOps.unstage,
    discard: gitOps.discard,
    createPR: gitOps.createPR,
  };
}
