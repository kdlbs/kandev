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
  // Multi-repo: when `repo` is omitted, commit fans out one call per repo with
  // staged changes so each repo gets its own commit. With `repo`, only that
  // repo is committed.
  commit: (
    message: string,
    stageAll?: boolean,
    amend?: boolean,
    repo?: string,
  ) => Promise<GitOperationResult>;
  // Multi-repo: when `repo` is provided, the op runs against that repo only —
  // use this for per-file actions to avoid path-based lookup collisions
  // (same-named files like README.md exist in multiple repos). When `repo`
  // is omitted, the op falls back to a path-based lookup against allFiles
  // (works for unique paths) or fans out across every repo present.
  stage: (paths?: string[], repo?: string) => Promise<GitOperationResult>;
  stageFile: (paths: string[], repo?: string) => Promise<GitOperationResult>;
  stageAll: () => Promise<GitOperationResult>;
  unstage: (paths?: string[], repo?: string) => Promise<GitOperationResult>;
  unstageFile: (paths: string[], repo?: string) => Promise<GitOperationResult>;
  unstageAll: () => Promise<GitOperationResult>;
  discard: (paths?: string[], repo?: string) => Promise<GitOperationResult>;
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
 * Groups paths into per-repo buckets using a path → repository_name lookup.
 * Paths missing a known repo land under "" (the single-repo bucket) so legacy
 * single-repo workspaces and stray entries stay correct. Insertion order in
 * `paths` is preserved within each bucket.
 *
 * Exported for testing — also used internally by useSessionGit's stage/unstage
 * fan-out, where every per-repo bucket becomes one agentctl call.
 */
export function groupPathsByRepoName(
  paths: string[],
  repoForPath: Map<string, string>,
): Map<string, string[]> {
  const buckets = new Map<string, string[]>();
  for (const p of paths) {
    const repo = repoForPath.get(p) ?? "";
    const list = buckets.get(repo);
    if (list) list.push(p);
    else buckets.set(repo, [p]);
  }
  return buckets;
}

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

  // Multi-repo path-to-repo lookup: agentctl's stage/unstage/discard endpoints
  // need an explicit repo subpath (the repo dir basename). Without it the op
  // runs at the workspace root which, for multi-repo task workspaces, isn't
  // a git repo and silently no-ops.
  const repoForPath = useMemo(() => {
    const m = new Map<string, string>();
    for (const f of allFiles) {
      if (f.repository_name) m.set(f.path, f.repository_name);
    }
    return m;
  }, [allFiles]);
  // List of distinct repo names present in the current files; empty when single-repo.
  const reposInFiles = useMemo(() => {
    const seen = new Set<string>();
    for (const f of allFiles) if (f.repository_name) seen.add(f.repository_name);
    return Array.from(seen);
  }, [allFiles]);

  const groupPathsByRepo = useCallback(
    (paths: string[]): Map<string, string[]> => groupPathsByRepoName(paths, repoForPath),
    [repoForPath],
  );

  // Fan out a stage-all / unstage-all (no paths) across every repo when multi-repo.
  // Returns the last op's result (UI only checks success/error of the most recent).
  const runOnAllRepos = useCallback(
    async (op: (paths: string[] | undefined, repo?: string) => Promise<GitOperationResult>) => {
      if (reposInFiles.length <= 1) {
        return op(undefined, reposInFiles[0]);
      }
      let last: GitOperationResult | undefined;
      for (const repo of reposInFiles) {
        last = await op(undefined, repo);
      }
      return last as GitOperationResult;
    },
    [reposInFiles],
  );

  const stageAll = useCallback(
    async () => runOnAllRepos((paths, repo) => gitOps.stage(paths, repo)),
    // eslint-disable-next-line react-hooks/exhaustive-deps -- depend on stable fn ref, not the whole gitOps object
    [runOnAllRepos, gitOps.stage],
  );
  const unstageAll = useCallback(
    async () => runOnAllRepos((paths, repo) => gitOps.unstage(paths, repo)),
    // eslint-disable-next-line react-hooks/exhaustive-deps -- depend on stable fn ref, not the whole gitOps object
    [runOnAllRepos, gitOps.unstage],
  );

  // Multi-repo commit: when the caller doesn't pin a repo, commit each repo
  // that has staged files separately (with the same message). Committing at
  // the workspace root for a multi-repo task fails because the task root
  // isn't itself a git repo — that's the "exit 1" the user saw.
  const commit = useCallback(
    async (
      message: string,
      stageAll: boolean = true,
      amend: boolean = false,
      repo?: string,
    ): Promise<GitOperationResult> => {
      if (repo !== undefined) {
        return gitOps.commit(message, stageAll, amend, repo || undefined);
      }
      const reposWithStaged = Array.from(
        new Set(
          stagedFiles
            .map((f) => f.repository_name)
            .filter((n): n is string => Boolean(n)),
        ),
      );
      // Single-repo (or repo-less): one call, no repo arg.
      if (reposWithStaged.length === 0) {
        return gitOps.commit(message, stageAll, amend);
      }
      // Multi-repo: one commit per repo with staged changes. Stop on first
      // failure so the user sees the actual error instead of cascading ones.
      let last: GitOperationResult | undefined;
      for (const r of reposWithStaged) {
        last = await gitOps.commit(message, stageAll, amend, r);
        if (!last.success) return last;
      }
      return last as GitOperationResult;
    },
    // eslint-disable-next-line react-hooks/exhaustive-deps -- depend on stable fn ref, not the whole gitOps object
    [gitOps.commit, stagedFiles],
  );

  // runPerRepo invokes `op` once per repo bucket. When the caller supplies an
  // explicit `repo`, all paths route there as a single call (skipping the
  // path → repo lookup that can collide on same-named files). Otherwise paths
  // are split by repository_name from allFiles.
  const runPerRepo = useCallback(
    async (
      paths: string[],
      explicitRepo: string | undefined,
      op: (paths: string[], repo: string | undefined) => Promise<GitOperationResult>,
    ): Promise<GitOperationResult> => {
      if (explicitRepo !== undefined) {
        return op(paths, explicitRepo || undefined);
      }
      const buckets = groupPathsByRepo(paths);
      let last: GitOperationResult | undefined;
      for (const [repo, repoPaths] of buckets) {
        last = await op(repoPaths, repo || undefined);
      }
      return last as GitOperationResult;
    },
    [groupPathsByRepo],
  );

  const stageFile = useCallback(
    async (paths: string[], repo?: string) => {
      for (const p of paths) setPendingStageFiles((prev) => new Set(prev).add(p));
      try {
        return await runPerRepo(paths, repo, (rp, r) => gitOps.stage(rp, r));
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
    [gitOps.stage, runPerRepo],
  );

  const unstageFile = useCallback(
    async (paths: string[], repo?: string) => {
      for (const p of paths) setPendingStageFiles((prev) => new Set(prev).add(p));
      try {
        return await runPerRepo(paths, repo, (rp, r) => gitOps.unstage(rp, r));
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
    [gitOps.unstage, runPerRepo],
  );

  const discard = useCallback(
    async (paths?: string[], repo?: string) => {
      if (!paths || paths.length === 0) {
        // Discard with no paths is unsupported by the API; passthrough to surface the error.
        return gitOps.discard(paths, repo);
      }
      return runPerRepo(paths, repo, (rp, r) => gitOps.discard(rp, r));
    },
    // eslint-disable-next-line react-hooks/exhaustive-deps -- depend on stable fn ref, not the whole gitOps object
    [gitOps.discard, runPerRepo],
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
    commit,
    // stage/unstage route through the fan-out wrappers so multi-repo workspaces
    // hit the right repo subpath in agentctl. Empty/undefined paths = stage all.
    stage: (paths?: string[], repo?: string) =>
      paths && paths.length > 0 ? stageFile(paths, repo) : stageAll(),
    stageFile,
    stageAll,
    unstage: (paths?: string[], repo?: string) =>
      paths && paths.length > 0 ? unstageFile(paths, repo) : unstageAll(),
    unstageFile,
    unstageAll,
    discard,
    revertCommit: gitOps.revertCommit,
    renameBranch: gitOps.renameBranch,
    reset: gitOps.reset,
    createPR: gitOps.createPR,
  };
}
