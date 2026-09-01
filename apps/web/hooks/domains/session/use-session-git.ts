"use client";

import { useState, useCallback, useEffect, useMemo, useRef } from "react";
import { useSessionGitStatus, useSessionGitStatusByRepo } from "./use-session-git-status";
import { useSessionCommits } from "./use-session-commits";
import { useCumulativeDiff } from "./use-cumulative-diff";
import { useGitOperations } from "@/hooks/use-git-operations";
import { createDebugLogger, isDebug } from "@/lib/debug/log";
import type {
  FileInfo,
  SessionCommit,
  CumulativeDiff,
  GitStatusEntry,
} from "@/lib/state/slices/session-runtime/types";
import type {
  GitOperationResult as RawGitOperationResult,
  PRCreateResult,
} from "@/hooks/use-git-operations";
import { t } from "@/lib/i18n";
import {
  repositoryScopesWithAvailableAncestors,
  runRepositoryScopeWaves,
} from "./use-session-git-repository-order";
import { useMultiRepoSummary } from "./use-session-git-summary";
import { deriveComparisonValues, deriveSessionGitValues } from "./use-session-git-derived";
import { useScopedStageOperations } from "./use-scoped-stage-operations";
import { normalizeGitStatusFiles } from "@/lib/state/slices/session-runtime/git-status-normalizer";
import { splitFilesByChangeLayer } from "./git-change-facets";

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
  error_code?: string;
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

const debugDeriv = createDebugLogger("git-status:derive");

export type SessionGit = {
  // Branch info
  branch: string | null;
  remoteBranch: string | null;
  headCommit: string | null;
  remoteHeadCommit: string | null;
  ahead: number;
  behind: number;
  remoteAhead: number;
  remoteBehind: number;
  pushAhead: number;
  pullBehind: number;

  // Files (raw FileInfo from store)
  allFiles: FileInfo[];
  unstagedFiles: FileInfo[];
  stagedFiles: FileInfo[];

  // Commits
  commits: SessionCommit[];
  cumulativeDiff: CumulativeDiff | null;
  commitsLoading: boolean;

  // Derived state — single source of truth for all git-dependent UI
  statusLoaded: boolean;
  hasUnstaged: boolean;
  hasStaged: boolean;
  hasCommits: boolean;
  hasChanges: boolean; // hasUnstaged || hasStaged
  hasAnything: boolean; // hasChanges || hasCommits
  canStageAll: boolean; // hasUnstaged
  canCommit: boolean; // hasStaged
  canPush: boolean; // pushAhead > 0
  canPull: boolean; // pullBehind > 0
  canCreatePR: boolean; // hasCommits
  comparisonTargets: string[];
  comparisonUnavailable: boolean;
  comparisonErrorCode: string | null;

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
    remoteAhead: number;
    remoteBehind: number;
    pushAhead: number;
    pullBehind: number;
    hasUpstream: boolean;
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
  renameBranch: (newName: string, repo?: string) => Promise<GitOperationResult>;
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
 * Returns mutation scopes that have a live status tracker. A file can still
 * appear under the empty scope when the parent repository only reports a
 * changed gitlink; that scope is not a runnable repository unless it is also
 * present in statusByRepo. Legacy single-repository hydration has no per-repo
 * list, so it keeps the historical empty-scope fallback.
 */
export function repositoryScopesForMutation(
  allFiles: Pick<FileInfo, "repository_name">[],
  availableScopes: Iterable<string>,
): string[] {
  const available = Array.from(new Set(availableScopes));
  if (available.length === 0) {
    return repositoryScopesWithAvailableAncestors(
      new Set(allFiles.map((file) => file.repository_name ?? "")),
      available,
    );
  }

  const availableSet = new Set(available);
  const requested = new Set<string>();
  for (const file of allFiles) {
    const scope = file.repository_name ?? "";
    if (availableSet.has(scope)) requested.add(scope);
  }
  return repositoryScopesWithAvailableAncestors(requested, available);
}

/**
 * Builds the SessionGit's flat file list. For multi-repo workspaces it
 * stamps each FileInfo with its repository_name so consumers can group;
 * for single-repo it returns the legacy single-status files unchanged.
 *
 * Each entry is stamped with `path` from the map key when the payload entry
 * omits it. Git-status payloads always carry `path`, but the DB-snapshot
 * fallback can replay archived cumulative-diff entries whose older shape only
 * sat under the key (no `path` field) — the changes tree splits on
 * `file.path` and crashes on `undefined`, so a missing path must never reach
 * consumers. Multi-repo cumulative keys are `<repo>\x00<path>` composites;
 * the normalizer restores both the path and repository scope from that key.
 */
export function aggregateFilesAcrossRepos(
  statusByRepo: ReturnType<typeof useSessionGitStatusByRepo>,
  gitStatus: ReturnType<typeof useSessionGitStatus>,
): FileInfo[] {
  if (statusByRepo.length > 0) {
    const out: FileInfo[] = [];
    for (const { repository_name, status } of statusByRepo) {
      if (!status?.files) continue;
      for (const file of Object.values(normalizeGitStatusFiles(status.files) ?? {})) {
        out.push(repository_name ? { ...file, repository_name } : file);
      }
    }
    return out;
  }
  return Object.values(normalizeGitStatusFiles(gitStatus?.files) ?? {});
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
      error_code: only.error_code,
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
    error_code: firstFailure?.error_code,
    per_repo: perRepo,
  };
}

async function fanOutAcrossRepositoryWaves(
  repos: string[],
  operation: string,
  op: (repo: string) => Promise<RawGitOperationResult>,
): Promise<GitOperationResult> {
  const results = await runRepositoryScopeWaves(
    repos,
    async (repo) => {
      try {
        return await op(repo);
      } catch (e) {
        return {
          success: false,
          operation,
          output: "",
          error: e instanceof Error ? e.message : String(e),
        };
      }
    },
    (repo, failedScopes) => ({
      success: false,
      operation,
      output: "",
      error: t("common:gitScopeSkipped", {
        scope: repo || t("common:workspace"),
        failedScopes: failedScopes.join(", "),
      }),
    }),
  );
  return aggregatePerRepoResults(
    results.map(({ repository_name, result }) => ({
      repository_name,
      success: result.success,
      output: result.output,
      error: result.error,
      error_code: result.error_code,
    })),
    operation,
  );
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
      return fanOutAcrossRepositoryWaves(reposInFiles, "stage", (r) => gitOps.stage(undefined, r));
    },
    // eslint-disable-next-line react-hooks/exhaustive-deps -- stable fn ref
    [reposInFiles, gitOps.stage],
  );
  const unstageAll = useCallback(
    async (): Promise<GitOperationResult> => {
      if (reposInFiles.length <= 1) return gitOps.unstage(undefined, reposInFiles[0]);
      return fanOutAcrossRepositoryWaves(reposInFiles, "unstage", (r) =>
        gitOps.unstage(undefined, r),
      );
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
      if (repo !== undefined) return gitOps.commit(message, stageAllOpt, amend, repo);
      const reposWithStaged = Array.from(new Set(stagedFiles.map((f) => f.repository_name ?? "")));
      const reposToCommit = stageAllOpt ? reposInFiles : reposWithStaged;
      if (reposToCommit.length === 0) return gitOps.commit(message, stageAllOpt, amend);
      if (reposToCommit.length === 1) {
        return gitOps.commit(message, stageAllOpt, amend, reposToCommit[0]);
      }
      return fanOutAcrossRepositoryWaves(reposToCommit, "commit", (r) =>
        gitOps.commit(message, stageAllOpt, amend, r),
      );
    },
    // eslint-disable-next-line react-hooks/exhaustive-deps -- stable fn ref
    [gitOps.commit, reposInFiles, stagedFiles],
  );
  const runPerRepo = useCallback(
    async (
      paths: string[],
      explicitRepo: string | undefined,
      operation: string,
      op: (paths: string[], repo: string | undefined) => Promise<GitOperationResult>,
    ): Promise<GitOperationResult> => {
      if (explicitRepo !== undefined) return op(paths, explicitRepo);
      const buckets = groupPathsByRepo(paths);
      if (buckets.size <= 1) {
        const [repo, repoPaths] = buckets.entries().next().value as [string, string[]];
        return op(repoPaths, repo);
      }
      return fanOutAcrossRepositoryWaves(Array.from(buckets.keys()), operation, (repo) =>
        op(buckets.get(repo) ?? [], repo),
      );
    },
    [groupPathsByRepo],
  );
  const wrapPending = useCallback(
    async (
      paths: string[],
      repo: string | undefined,
      op: (rp: string[], r: string | undefined) => Promise<GitOperationResult>,
      operation: string,
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
        return await runPerRepo(paths, repo, operation, op);
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
      wrapPending(paths, repo, (rp, r) => gitOps.stage(rp, r), "stage"),
    // eslint-disable-next-line react-hooks/exhaustive-deps -- stable fn ref
    [gitOps.stage, wrapPending],
  );
  const unstageFile = useCallback(
    async (paths: string[], repo?: string) =>
      wrapPending(paths, repo, (rp, r) => gitOps.unstage(rp, r), "unstage"),
    // eslint-disable-next-line react-hooks/exhaustive-deps -- stable fn ref
    [gitOps.unstage, wrapPending],
  );
  const discard = useCallback(
    async (paths?: string[], repo?: string) => {
      if (!paths || paths.length === 0) return gitOps.discard(paths, repo);
      return runPerRepo(paths, repo, "discard", (rp, r) => gitOps.discard(rp, r));
    },
    // eslint-disable-next-line react-hooks/exhaustive-deps -- stable fn ref
    [gitOps.discard, runPerRepo],
  );
  return { stageAll, unstageAll, commit, stageFile, unstageFile, discard };
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
  perRepoStatus: Array<{ repository_name: string; pushAhead: number }>;
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
  const namedRepos = useMemo(() => repoNamesForControls, [repoNamesForControls]);
  const isMultiRepo = repoNamesForControls.length > 1;
  const aheadByRepo = useMemo(() => {
    const m = new Map<string, number>();
    for (const s of perRepoStatus) m.set(s.repository_name, s.pushAhead);
    return m;
  }, [perRepoStatus]);

  const pull = useCallback(
    async (rebase = false, repo?: string): Promise<GitOperationResult> => {
      if (repo !== undefined || !isMultiRepo) return gitOps.pull(rebase, repo);
      return fanOutAcrossRepositoryWaves(namedRepos, "pull", (r) => gitOps.pull(rebase, r));
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
      return fanOutAcrossRepositoryWaves(reposWithAhead, "push", (r) => gitOps.push(options, r));
    },
    // eslint-disable-next-line react-hooks/exhaustive-deps -- stable fn ref
    [gitOps.push, isMultiRepo, namedRepos, aheadByRepo],
  );

  const rebase = useCallback(
    async (baseBranch: string, repo?: string): Promise<GitOperationResult> => {
      if (repo !== undefined || !isMultiRepo) return gitOps.rebase(baseBranch, repo);
      return fanOutAcrossRepositoryWaves(namedRepos, "rebase", (r) => gitOps.rebase(baseBranch, r));
    },
    // eslint-disable-next-line react-hooks/exhaustive-deps -- stable fn ref
    [gitOps.rebase, isMultiRepo, namedRepos],
  );

  const merge = useCallback(
    async (baseBranch: string, repo?: string): Promise<GitOperationResult> => {
      if (repo !== undefined || !isMultiRepo) return gitOps.merge(baseBranch, repo);
      return fanOutAcrossRepositoryWaves(namedRepos, "merge", (r) => gitOps.merge(baseBranch, r));
    },
    // eslint-disable-next-line react-hooks/exhaustive-deps -- stable fn ref
    [gitOps.merge, isMultiRepo, namedRepos],
  );

  const abort = useCallback(
    async (operation: "merge" | "rebase", repo?: string): Promise<GitOperationResult> => {
      if (repo !== undefined || !isMultiRepo) return gitOps.abort(operation, repo);
      return fanOutAcrossRepositoryWaves(namedRepos, "abort", (r) => gitOps.abort(operation, r));
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
  useEffect(() => {
    if (!isDebug()) return;
    debugDeriv("aggregate", {
      path: statusByRepo.length > 0 ? "multi-repo" : "single-repo-fallback",
      statusByRepoEntries: statusByRepo.map((s) => ({
        repo: s.repository_name,
        files: Object.keys(s.status?.files ?? {}).length,
      })),
      legacyGitStatusFiles: Object.keys(gitStatus?.files ?? {}).length,
      allFilesCount: allFiles.length,
    });
  }, [statusByRepo, gitStatus, allFiles]);
  const { stagedFiles, unstagedFiles } = useMemo(
    () => splitFilesByChangeLayer(allFiles),
    [allFiles],
  );
  const repoForPath = useMemo(() => {
    const m = new Map<string, string>();
    for (const f of allFiles) {
      if (f.repository_name) m.set(f.path, f.repository_name);
    }
    return m;
  }, [allFiles]);
  const reposInFiles = useMemo(() => {
    return repositoryScopesForMutation(
      allFiles,
      statusByRepo.map(({ repository_name }) => repository_name),
    );
  }, [allFiles, statusByRepo]);
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

  const stageOps = useStageDispatch({
    gitOps,
    repoForPath,
    reposInFiles,
    stagedFiles,
    setPendingStageFiles,
  });
  const { stageAll, unstageAll, commit, stageFile, unstageFile, discard } = stageOps;
  const derived = deriveSessionGitValues(
    gitStatus,
    statusByRepo.length > 0,
    unstagedFiles,
    stagedFiles,
    commits,
  );
  const comparison = deriveComparisonValues(comparisonStatuses(statusByRepo, gitStatus));
  const remoteOps = useRemoteOpsFanOut({
    gitOps,
    repoNamesForControls,
    perRepoStatus,
  });
  const scopedStageOperations = useScopedStageOperations(
    gitOps,
    stageAll,
    stageFile,
    unstageAll,
    unstageFile,
  );

  return {
    ...derived,
    ...comparison,
    repoNames: repoNamesForControls,
    perRepoStatus,

    allFiles,
    unstagedFiles,
    stagedFiles,

    commits,
    cumulativeDiff,
    commitsLoading: commitsLoading ?? false,

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
    stage: scopedStageOperations.stage,
    stageFile,
    stageAll,
    unstage: scopedStageOperations.unstage,
    unstageFile,
    unstageAll,
    discard,
    revertCommit: gitOps.revertCommit,
    renameBranch: gitOps.renameBranch,
    reset: gitOps.reset,
    createPR: gitOps.createPR,
  };
}

function comparisonStatuses(
  statusByRepo: Array<{ status: GitStatusEntry }>,
  gitStatus: GitStatusEntry | null | undefined,
): GitStatusEntry[] {
  if (statusByRepo.length > 0) return statusByRepo.map(({ status }) => status);
  return gitStatus ? [gitStatus] : [];
}
