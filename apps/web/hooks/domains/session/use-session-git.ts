"use client";

import { useState, useCallback, useEffect, useMemo, useRef } from "react";
import { useSessionGitStatus, useSessionGitStatusByRepo } from "./use-session-git-status";
import { useSessionCommits } from "./use-session-commits";
import { useCumulativeDiff } from "./use-cumulative-diff";
import { useGitOperations } from "@/hooks/use-git-operations";
import type {
  FileInfo,
  SessionCommit,
  CumulativeDiff,
} from "@/lib/state/slices/session-runtime/types";
import type {
  GitOperationResult as RawGitOperationResult,
  PRCreateResult,
} from "@/hooks/use-git-operations";

/**
 * Per-repo result emitted by frontend-side fan-outs (commit, push, pull,
 * rebase, merge, abort, stage-all, unstage-all). Each entry mirrors the wire
 * shape returned by agentctl for one repo so the toast layer can describe
 * partial successes ("succeeded in [A], failed in [B]: …").
 */
export type PerRepoOperationResult = {
  repository_name: string;
  success: boolean;
  output: string;
  error?: string;
};

/**
 * Aggregated result of a multi-repo fan-out. The `success` / `output` /
 * `error` fields preserve the legacy `GitOperationResult` shape so existing
 * single-repo callers keep working unchanged. The optional `per_repo` field
 * is frontend-internal — it never goes back to the backend wire and only
 * appears when the op was fanned out across more than one repo.
 */
export type GitOperationResult = RawGitOperationResult & {
  per_repo?: PerRepoOperationResult[];
};

export type { PRCreateResult };

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

  // Actions. The optional `repo` param scopes the op to a single repo
  // subpath in multi-repo workspaces; omit for single-repo.
  // When `repo` is omitted in multi-repo mode, these fan out across every repo
  // (push gates on ahead > 0; pull/rebase/merge/abort hit every repo).
  pull: (rebase?: boolean, repo?: string) => Promise<GitOperationResult>;
  push: (
    options?: { force?: boolean; setUpstream?: boolean },
    repo?: string,
  ) => Promise<GitOperationResult>;
  rebase: (baseBranch: string, repo?: string) => Promise<GitOperationResult>;
  merge: (baseBranch: string, repo?: string) => Promise<GitOperationResult>;
  abort: (operation: "merge" | "rebase", repo?: string) => Promise<GitOperationResult>;
  /** Distinct repo names present in the session's files; empty for single-repo. */
  repoNames: string[];
  /** Per-repo branch / ahead / behind / hasStaged for header buttons. */
  perRepoStatus: Array<{
    repository_name: string;
    branch: string | null;
    ahead: number;
    behind: number;
    hasStaged: boolean;
    hasUnstaged: boolean;
  }>;
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
  revertCommit: (commitSHA: string, repo?: string) => Promise<GitOperationResult>;
  renameBranch: (newName: string) => Promise<GitOperationResult>;
  reset: (commitSHA: string, mode: "soft" | "hard", repo?: string) => Promise<GitOperationResult>;
  createPR: (
    title: string,
    body: string,
    baseBranch?: string,
    draft?: boolean,
    repo?: string,
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

type StageDispatchArgs = {
  gitOps: ReturnType<typeof useGitOperations>;
  repoForPath: Map<string, string>;
  reposInFiles: string[];
  stagedFiles: FileInfo[];
  setPendingStageFiles: React.Dispatch<React.SetStateAction<Set<string>>>;
};

/**
 * Encodes a (repo, path) pair as a single Set entry. We need a per-repo key
 * because two repos can have files at the same relative path (README.md,
 * .gitignore, etc.) and a flat path-only Set would conflate them. The
 * separator "::" keeps the key trivially parseable for debugging — repo
 * names don't contain "::".
 */
export function pendingKey(repo: string | undefined, path: string): string {
  return `${repo ?? ""}::${path}`;
}

/**
 * Aggregates a list of per-repo results into a single GitOperationResult.
 * - `success` is true only if every repo succeeded
 * - `output` is the joined output across repos (one line each, prefixed with the repo name)
 * - `error` is the first failure's error
 * - `per_repo` carries the full per-repo breakdown for the toast layer
 *
 * Single-repo (one entry) returns the raw result with no `per_repo` field so
 * the toast handler renders the legacy single-line message.
 */
function aggregatePerRepoResults(
  perRepo: PerRepoOperationResult[],
  operation: string,
): GitOperationResult {
  if (perRepo.length === 0) {
    return { success: true, operation, output: "" };
  }
  if (perRepo.length === 1) {
    const only = perRepo[0];
    return {
      success: only.success,
      operation,
      output: only.output,
      error: only.error,
    };
  }
  const allSucceeded = perRepo.every((r) => r.success);
  const firstFailure = perRepo.find((r) => !r.success);
  const joined = perRepo
    .map((r) => `[${r.repository_name || "default"}] ${r.output}`.trim())
    .filter(Boolean)
    .join("\n");
  return {
    success: allSucceeded,
    operation,
    output: joined,
    error: firstFailure?.error,
    per_repo: perRepo,
  };
}

/**
 * Runs `op` against each repo in `repos`, collecting per-repo results.
 * Continues past failures so partial-success state is surfaced (instead of
 * stopping at the first error and leaving the user blind to repo A having
 * already mutated).
 */
async function fanOutAcrossRepos(
  repos: string[],
  operation: string,
  op: (repo: string) => Promise<RawGitOperationResult>,
): Promise<GitOperationResult> {
  const perRepo: PerRepoOperationResult[] = [];
  for (const repo of repos) {
    try {
      const r = await op(repo);
      perRepo.push({
        repository_name: repo,
        success: r.success,
        output: r.output,
        error: r.error,
      });
    } catch (e) {
      perRepo.push({
        repository_name: repo,
        success: false,
        output: "",
        error: e instanceof Error ? e.message : String(e),
      });
    }
  }
  return aggregatePerRepoResults(perRepo, operation);
}

/**
 * Multi-repo dispatch for stage/unstage/commit/discard. The fan-out logic
 * (stage-all across every repo, commit per-repo with staged changes, etc.)
 * lives here so the parent hook stays small.
 */
// eslint-disable-next-line max-lines-per-function -- dispatch table, splitting further hurts readability
function useStageDispatch({
  gitOps,
  repoForPath,
  reposInFiles,
  stagedFiles,
  setPendingStageFiles,
}: StageDispatchArgs) {
  const groupPathsByRepo = useCallback(
    (paths: string[]): Map<string, string[]> => groupPathsByRepoName(paths, repoForPath),
    [repoForPath],
  );
  // stage-all / unstage-all fan out across every repo with files. Per-repo
  // failures are collected and surfaced via `per_repo` instead of overwriting
  // each iteration (Bug 3) — the toast layer renders partial-success cleanly.
  const stageAll = useCallback(
    async (): Promise<GitOperationResult> => {
      if (reposInFiles.length <= 1) return gitOps.stage(undefined, reposInFiles[0]);
      return fanOutAcrossRepos(reposInFiles, "stage", (r) => gitOps.stage(undefined, r));
    },
    // eslint-disable-next-line react-hooks/exhaustive-deps -- stable fn ref
    [reposInFiles, gitOps.stage],
  );
  const unstageAll = useCallback(
    async (): Promise<GitOperationResult> => {
      if (reposInFiles.length <= 1) return gitOps.unstage(undefined, reposInFiles[0]);
      return fanOutAcrossRepos(reposInFiles, "unstage", (r) => gitOps.unstage(undefined, r));
    },
    // eslint-disable-next-line react-hooks/exhaustive-deps -- stable fn ref
    [reposInFiles, gitOps.unstage],
  );
  // commit fan-out: when no `repo` is given and multiple repos have staged
  // changes, commit each one. Bug 2: previously we stopped at first failure
  // and returned only that result, so a partial success ("repo A committed,
  // repo B failed") looked like a total failure. Now we continue and
  // aggregate, preserving each per-repo result for the toast layer.
  const commit = useCallback(
    async (
      message: string,
      stageAllOpt: boolean = true,
      amend: boolean = false,
      repo?: string,
    ): Promise<GitOperationResult> => {
      if (repo !== undefined) return gitOps.commit(message, stageAllOpt, amend, repo || undefined);
      const reposWithStaged = Array.from(
        new Set(stagedFiles.map((f) => f.repository_name).filter((n): n is string => Boolean(n))),
      );
      if (reposWithStaged.length === 0) return gitOps.commit(message, stageAllOpt, amend);
      if (reposWithStaged.length === 1) {
        return gitOps.commit(message, stageAllOpt, amend, reposWithStaged[0]);
      }
      return fanOutAcrossRepos(reposWithStaged, "commit", (r) =>
        gitOps.commit(message, stageAllOpt, amend, r),
      );
    },
    // eslint-disable-next-line react-hooks/exhaustive-deps -- stable fn ref
    [gitOps.commit, stagedFiles],
  );
  const runPerRepo = useCallback(
    async (
      paths: string[],
      explicitRepo: string | undefined,
      op: (paths: string[], repo: string | undefined) => Promise<GitOperationResult>,
    ): Promise<GitOperationResult> => {
      if (explicitRepo !== undefined) return op(paths, explicitRepo || undefined);
      const buckets = groupPathsByRepo(paths);
      let last: GitOperationResult | undefined;
      for (const [repo, repoPaths] of buckets) last = await op(repoPaths, repo || undefined);
      return last as GitOperationResult;
    },
    [groupPathsByRepo],
  );
  const wrapPending = useCallback(
    async (
      paths: string[],
      repo: string | undefined,
      op: (rp: string[], r: string | undefined) => Promise<GitOperationResult>,
    ) => {
      // Bug 6: key pending entries by `repo::path` so an in-flight stage in
      // repo B isn't cleared when repo A's status update lands. The consumer
      // `FileRow` checks membership via the same encoding.
      const buckets = repo !== undefined ? new Map([[repo, paths]]) : groupPathsByRepo(paths);
      const keys: string[] = [];
      for (const [r, rp] of buckets) for (const p of rp) keys.push(pendingKey(r, p));
      setPendingStageFiles((prev) => {
        const next = new Set(prev);
        for (const k of keys) next.add(k);
        return next;
      });
      try {
        return await runPerRepo(paths, repo, op);
      } catch (err) {
        setPendingStageFiles((prev) => {
          const next = new Set(prev);
          for (const k of keys) next.delete(k);
          return next;
        });
        throw err;
      }
    },
    [runPerRepo, setPendingStageFiles, groupPathsByRepo],
  );
  const stageFile = useCallback(
    async (paths: string[], repo?: string) =>
      wrapPending(paths, repo, (rp, r) => gitOps.stage(rp, r)),
    // eslint-disable-next-line react-hooks/exhaustive-deps -- stable fn ref
    [gitOps.stage, wrapPending],
  );
  const unstageFile = useCallback(
    async (paths: string[], repo?: string) =>
      wrapPending(paths, repo, (rp, r) => gitOps.unstage(rp, r)),
    // eslint-disable-next-line react-hooks/exhaustive-deps -- stable fn ref
    [gitOps.unstage, wrapPending],
  );
  const discard = useCallback(
    async (paths?: string[], repo?: string) => {
      if (!paths || paths.length === 0) return gitOps.discard(paths, repo);
      return runPerRepo(paths, repo, (rp, r) => gitOps.discard(rp, r));
    },
    // eslint-disable-next-line react-hooks/exhaustive-deps -- stable fn ref
    [gitOps.discard, runPerRepo],
  );
  return { stageAll, unstageAll, commit, stageFile, unstageFile, discard };
}

/**
 * Multi-repo summary for the per-repo Pull/Push/Commit controls. Returns the
 * full list of repo names known to the session (even ones with no file
 * changes) plus a per-repo `branch / ahead / behind / hasStaged / hasUnstaged`
 * row for each. Empty for single-repo workspaces.
 */
function useMultiRepoSummary(
  statusByRepo: ReturnType<typeof useSessionGitStatusByRepo>,
  allFiles: FileInfo[],
  reposInFiles: string[],
) {
  // Include every repo known to the session, including the empty-name entry
  // for single-repo. Components render the same per-repo group structure in
  // both modes — the empty name routes ops to the workspace root, named
  // entries route to their respective subdirectories.
  //
  // Defensive filter: when there are named entries (multi-repo), drop the
  // empty entry. The bare task-root tracker is supposed to stay quiet for
  // multi-repo (see workspace_tracker.go's gitIndexPath guard) but a stale
  // entry can linger from older builds or a bugged code path; without this
  // filter the dropdown would show an extra "Repository" / primary-name row
  // alongside the real per-repo entries.
  const repoNamesForControls = useMemo(() => {
    const seen = new Set<string>();
    for (const { repository_name } of statusByRepo) seen.add(repository_name);
    for (const r of reposInFiles) seen.add(r);
    const all = Array.from(seen).sort((a, b) => a.localeCompare(b));
    const named = all.filter((r) => r !== "");
    return named.length > 0 ? named : all;
  }, [statusByRepo, reposInFiles]);

  const perRepoStatus = useMemo(() => {
    if (statusByRepo.length === 0) return [];
    const stagedByRepo = new Map<string, boolean>();
    const unstagedByRepo = new Map<string, boolean>();
    for (const f of allFiles) {
      const r = f.repository_name ?? "";
      if (f.staged) stagedByRepo.set(r, true);
      else unstagedByRepo.set(r, true);
    }
    const hasNamed = statusByRepo.some((s) => s.repository_name !== "");
    const filtered = hasNamed ? statusByRepo.filter((s) => s.repository_name !== "") : statusByRepo;
    return filtered.map(({ repository_name, status }) => ({
      repository_name,
      branch: status?.branch ?? null,
      ahead: status?.ahead ?? 0,
      behind: status?.behind ?? 0,
      hasStaged: stagedByRepo.get(repository_name) ?? false,
      hasUnstaged: unstagedByRepo.get(repository_name) ?? false,
    }));
  }, [statusByRepo, allFiles]);

  return { repoNamesForControls, perRepoStatus };
}

/**
 * Bug 6: clears pending stage markers per-repo, not globally. Each repo's
 * status streams in independently as agentctl finishes per-repo ops; if we
 * cleared the whole pending Set on any allFiles change, an in-flight stage
 * in repo B would lose its spinner the moment repo A finished. Detects which
 * repos' status entry changed since the last render and only drops the
 * corresponding `${repo}::${path}` keys.
 */
function usePerRepoPendingClear(
  statusByRepo: ReturnType<typeof useSessionGitStatusByRepo>,
  allFiles: FileInfo[],
  setPendingStageFiles: React.Dispatch<React.SetStateAction<Set<string>>>,
) {
  const prevStatusRef = useRef<Map<string, unknown>>(new Map());
  useEffect(() => {
    const next = new Map<string, unknown>();
    const refreshed: string[] = [];
    for (const { repository_name, status } of statusByRepo) {
      next.set(repository_name, status);
      if (prevStatusRef.current.get(repository_name) !== status) {
        refreshed.push(repository_name);
      }
    }
    // Single-repo / legacy path: gitStatus changed and there's no per-repo
    // entry to track. Clear all pending — original behavior, safe because
    // there's only one in-flight op at a time in single-repo.
    const isLegacySingleRepo = statusByRepo.length === 0;
    prevStatusRef.current = next;
    if (refreshed.length === 0 && !isLegacySingleRepo) return;
    setPendingStageFiles((prev) => {
      if (prev.size === 0) return prev;
      if (isLegacySingleRepo) return new Set();
      const out = new Set<string>();
      for (const key of prev) {
        // key shape is `${repo}::${path}`; first "::" splits repo from path.
        const sep = key.indexOf("::");
        const repo = sep === -1 ? "" : key.slice(0, sep);
        if (!refreshed.includes(repo)) out.add(key);
      }
      return out;
    });
  }, [allFiles, statusByRepo, setPendingStageFiles]);
}

type RemoteOpsArgs = {
  gitOps: ReturnType<typeof useGitOperations>;
  repoNamesForControls: string[];
  perRepoStatus: Array<{ repository_name: string; ahead: number }>;
};

/**
 * Fan-out wrappers for push/pull/rebase/merge/abort so the top-bar buttons
 * (which call `git.push()` / `git.pull()` etc. without a `repo` arg) hit
 * every repo in multi-repo workspaces instead of silently failing at the
 * workspace root (Bug 1). Single-repo workspaces and explicit-repo callers
 * fall through to the underlying `gitOps` directly.
 *
 * Push gates on `ahead > 0` per repo so we don't push repos with nothing to
 * push. Pull/Rebase/Merge/Abort fan out across every repo unconditionally.
 */
function useRemoteOpsFanOut({ gitOps, repoNamesForControls, perRepoStatus }: RemoteOpsArgs) {
  // Filter out the legacy empty-name entry; it represents the workspace root
  // which isn't a real git repo in multi-repo task workspaces. We only want
  // to iterate the named repos.
  const namedRepos = useMemo(
    () => repoNamesForControls.filter((r) => r !== ""),
    [repoNamesForControls],
  );
  const isMultiRepo = namedRepos.length > 1;
  const aheadByRepo = useMemo(() => {
    const m = new Map<string, number>();
    for (const s of perRepoStatus) m.set(s.repository_name, s.ahead);
    return m;
  }, [perRepoStatus]);

  const pull = useCallback(
    async (rebase = false, repo?: string): Promise<GitOperationResult> => {
      if (repo !== undefined || !isMultiRepo) return gitOps.pull(rebase, repo);
      return fanOutAcrossRepos(namedRepos, "pull", (r) => gitOps.pull(rebase, r));
    },
    // eslint-disable-next-line react-hooks/exhaustive-deps -- stable fn ref
    [gitOps.pull, isMultiRepo, namedRepos],
  );

  const push = useCallback(
    async (
      options?: { force?: boolean; setUpstream?: boolean },
      repo?: string,
    ): Promise<GitOperationResult> => {
      if (repo !== undefined || !isMultiRepo) return gitOps.push(options, repo);
      const reposWithAhead = namedRepos.filter((r) => (aheadByRepo.get(r) ?? 0) > 0);
      if (reposWithAhead.length === 0) {
        return { success: true, operation: "push", output: "No commits to push" };
      }
      return fanOutAcrossRepos(reposWithAhead, "push", (r) => gitOps.push(options, r));
    },
    // eslint-disable-next-line react-hooks/exhaustive-deps -- stable fn ref
    [gitOps.push, isMultiRepo, namedRepos, aheadByRepo],
  );

  const rebase = useCallback(
    async (baseBranch: string, repo?: string): Promise<GitOperationResult> => {
      if (repo !== undefined || !isMultiRepo) return gitOps.rebase(baseBranch, repo);
      return fanOutAcrossRepos(namedRepos, "rebase", (r) => gitOps.rebase(baseBranch, r));
    },
    // eslint-disable-next-line react-hooks/exhaustive-deps -- stable fn ref
    [gitOps.rebase, isMultiRepo, namedRepos],
  );

  const merge = useCallback(
    async (baseBranch: string, repo?: string): Promise<GitOperationResult> => {
      if (repo !== undefined || !isMultiRepo) return gitOps.merge(baseBranch, repo);
      return fanOutAcrossRepos(namedRepos, "merge", (r) => gitOps.merge(baseBranch, r));
    },
    // eslint-disable-next-line react-hooks/exhaustive-deps -- stable fn ref
    [gitOps.merge, isMultiRepo, namedRepos],
  );

  const abort = useCallback(
    async (operation: "merge" | "rebase", repo?: string): Promise<GitOperationResult> => {
      if (repo !== undefined || !isMultiRepo) return gitOps.abort(operation, repo);
      return fanOutAcrossRepos(namedRepos, "abort", (r) => gitOps.abort(operation, r));
    },
    // eslint-disable-next-line react-hooks/exhaustive-deps -- stable fn ref
    [gitOps.abort, isMultiRepo, namedRepos],
  );

  return { pull, push, rebase, merge, abort };
}

/**
 * Bundles the file/multirepo derivations the parent hook needs: aggregated
 * file list, staged/unstaged splits, per-path repo lookup, distinct repo
 * names, and the multi-repo summary used by per-repo header buttons.
 */
function useFileDerivations(
  statusByRepo: ReturnType<typeof useSessionGitStatusByRepo>,
  gitStatus: ReturnType<typeof useSessionGitStatus>,
) {
  const allFiles = useMemo<FileInfo[]>(
    () => aggregateFilesAcrossRepos(statusByRepo, gitStatus),
    [statusByRepo, gitStatus],
  );
  const unstagedFiles = useMemo(() => allFiles.filter((f) => !f.staged), [allFiles]);
  const stagedFiles = useMemo(() => allFiles.filter((f) => f.staged), [allFiles]);
  const repoForPath = useMemo(() => {
    const m = new Map<string, string>();
    for (const f of allFiles) {
      if (f.repository_name) m.set(f.path, f.repository_name);
    }
    return m;
  }, [allFiles]);
  const reposInFiles = useMemo(() => {
    const seen = new Set<string>();
    for (const f of allFiles) if (f.repository_name) seen.add(f.repository_name);
    return Array.from(seen);
  }, [allFiles]);
  const { repoNamesForControls, perRepoStatus } = useMultiRepoSummary(
    statusByRepo,
    allFiles,
    reposInFiles,
  );
  return {
    allFiles,
    unstagedFiles,
    stagedFiles,
    repoForPath,
    reposInFiles,
    repoNamesForControls,
    perRepoStatus,
  };
}

export function useSessionGit(sessionId: string | null | undefined): SessionGit {
  const sid = sessionId ?? null;
  const gitStatus = useSessionGitStatus(sid);
  const statusByRepo = useSessionGitStatusByRepo(sid);
  const { commits, loading: commitsLoading } = useSessionCommits(sid);
  const { diff: cumulativeDiff } = useCumulativeDiff(sid);
  const gitOps = useGitOperations(sid);
  const [pendingStageFiles, setPendingStageFiles] = useState<Set<string>>(new Set());
  const {
    allFiles,
    unstagedFiles,
    stagedFiles,
    repoForPath,
    reposInFiles,
    repoNamesForControls,
    perRepoStatus,
  } = useFileDerivations(statusByRepo, gitStatus);
  usePerRepoPendingClear(statusByRepo, allFiles, setPendingStageFiles);
  const ahead = gitStatus?.ahead ?? 0;
  const hasUnstaged = unstagedFiles.length > 0;
  const hasStaged = stagedFiles.length > 0;
  const hasCommits = commits.length > 0;

  const stageOps = useStageDispatch({
    gitOps,
    repoForPath,
    reposInFiles,
    stagedFiles,
    setPendingStageFiles,
  });
  const { stageAll, unstageAll, commit, stageFile, unstageFile, discard } = stageOps;
  const remoteOps = useRemoteOpsFanOut({
    gitOps,
    repoNamesForControls,
    perRepoStatus,
  });

  return {
    branch: gitStatus?.branch ?? null,
    remoteBranch: gitStatus?.remote_branch ?? null,
    ahead,
    behind: gitStatus?.behind ?? 0,
    repoNames: repoNamesForControls,
    perRepoStatus,

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

    pull: remoteOps.pull,
    push: remoteOps.push,
    rebase: remoteOps.rebase,
    merge: remoteOps.merge,
    abort: remoteOps.abort,
    commit,
    // stage/unstage with no paths and a `repo` arg = stage-all/unstage-all
    // for that single repo (one agentctl call). Without `repo`, we fan out
    // across every repo with files (multi-repo) or hit the workspace root
    // (single-repo). With paths, we route to the right repo per file.
    stage: (paths?: string[], repo?: string) => {
      if (paths && paths.length > 0) return stageFile(paths, repo);
      if (repo) return gitOps.stage(undefined, repo);
      return stageAll();
    },
    stageFile,
    stageAll,
    unstage: (paths?: string[], repo?: string) => {
      if (paths && paths.length > 0) return unstageFile(paths, repo);
      if (repo) return gitOps.unstage(undefined, repo);
      return unstageAll();
    },
    unstageFile,
    unstageAll,
    discard,
    revertCommit: gitOps.revertCommit,
    renameBranch: gitOps.renameBranch,
    reset: gitOps.reset,
    createPR: gitOps.createPR,
  };
}
