"use client";

import { useEffect, useMemo, useRef } from "react";
import { useQuery } from "@tanstack/react-query";
import type { Repository, Executor, ExecutorProfile } from "@/lib/types/http";
import { DEFAULT_LOCAL_EXECUTOR_TYPE } from "@/lib/utils";
import { useToast } from "@/components/toast-provider";
import {
  discoverRepositoriesAction,
  getLocalRepositoryStatusAction,
} from "@/app/actions/workspaces";
import { getLocalStorage } from "@/lib/local-storage";
import { workflowStepsQueryOptions } from "@/lib/query/query-options";
import { STORAGE_KEYS } from "@/lib/settings/constants";
import { parseGitHubAnyUrl } from "@/hooks/domains/github/use-pr-info-by-url";
import type {
  DialogFormState,
  StoreSelections,
  TaskCreateEffectsArgs,
  TaskRepoRow,
} from "@/components/task-create-dialog-types";
import {
  useAgentProfileAutopickEffect,
  useWorkflowAgentProfileEffect,
} from "@/components/task-create-dialog-autopick";
import { computeSelectedRepoCount } from "@/components/task-create-dialog-computed";
import { createDebugLogger, isDebug } from "@/lib/debug/log";

// Re-export autopick hooks for callers that imported them from this module.
export { useWorkflowAgentProfileEffect };
// Also re-exported for the test file, which expects the symbol to live here.
export { decideAgentProfileAutopick } from "@/components/task-create-dialog-autopick";

const selectionDebug = createDebugLogger("task-create:selection");

type RepositoryAutoPickDecision = {
  pickId: string | null;
  source: string;
  localStorageRepoId: string | null;
  localStorageValid: boolean;
  settingsRepoId: string | null;
  settingsValid: boolean;
};

export function useWorkflowStepsEffect(fs: DialogFormState, workflowId: string | null) {
  const { selectedWorkflowId, setFetchedSteps } = fs;
  const shouldFetch = Boolean(selectedWorkflowId && selectedWorkflowId !== workflowId);
  const query = useQuery({
    ...workflowStepsQueryOptions(selectedWorkflowId ?? ""),
    enabled: shouldFetch,
  });

  useEffect(() => {
    if (!shouldFetch || !query.data) {
      setFetchedSteps(null);
      return;
    }
    setFetchedSteps(
      query.data.map((step) => ({ id: step.id, title: step.name, events: step.events })),
    );
  }, [query.data, setFetchedSteps, shouldFetch]);
}

export function useRepositoryAutoSelectEffect(
  fs: DialogFormState,
  open: boolean,
  workspaceId: string | null,
  repositories: Repository[],
  lastUsedRepositoryId?: string | null,
) {
  // On open, ensure there's always at least one chip rendered: prefer the
  // user's last-used repo (or the workspace's only repo) so the chip lands
  // pre-filled, but fall back to an empty row so the picker is visible
  // instead of just the "+" button. URL mode is excluded — that flow swaps
  // the chip row for a URL input.
  const { repositories: rows, useRemote, setRepositories } = fs;
  useEffect(() => {
    if (!open || !workspaceId || useRemote) return;
    const decision = decideRepositoryAutoPick(repositories, lastUsedRepositoryId);
    const { pickId } = decision;
    if (rows.length > 0 && !canReplaceEmptyRepositoryPlaceholder(rows, pickId)) return;
    logRepositoryAutoPick(workspaceId, repositories.length, decision);
    // Use the functional setter so the deferred microtask sees fresh state.
    // Without this, a sibling effect (resetTaskForm / useLockedFieldSync) that
    // seeds rows from `initialValues.repositoryId` synchronously races with
    // this microtask — the microtask captured rows.length === 0 at queue time
    // and would blindly clobber the initialValues-seeded row.
    void Promise.resolve().then(() => {
      setRepositories((prev) => {
        if (prev.length > 0) {
          if (canReplaceEmptyRepositoryPlaceholder(prev, pickId)) {
            return [buildRepositoryAutoPickRow(prev[0]?.key ?? "row-0", pickId!)];
          }
          if (isDebug()) {
            selectionDebug("repository-autopick-skip", {
              reason: "rows-seeded-before-microtask",
              row_count: prev.length,
            });
          }
          return prev;
        }
        return [
          pickId ? buildRepositoryAutoPickRow("row-0", pickId) : { key: "row-0", branch: "" },
        ];
      });
    });
  }, [open, repositories, rows, useRemote, workspaceId, setRepositories, lastUsedRepositoryId]);
}

function decideRepositoryAutoPick(
  repositories: Repository[],
  lastUsedRepositoryId?: string | null,
): RepositoryAutoPickDecision {
  const localStorageRepoId = getLocalStorage<string | null>(STORAGE_KEYS.LAST_REPOSITORY_ID, null);
  const settingsRepoId = lastUsedRepositoryId ?? null;
  const localStorageValid = isRepositoryIdValid(localStorageRepoId, repositories);
  const settingsValid = isRepositoryIdValid(settingsRepoId, repositories);
  if (localStorageRepoId && localStorageValid) {
    return {
      pickId: localStorageRepoId,
      source: "localStorage:lastRepositoryId",
      localStorageRepoId,
      localStorageValid,
      settingsRepoId,
      settingsValid,
    };
  }
  if (settingsRepoId && settingsValid) {
    return {
      pickId: settingsRepoId,
      source: "settings:taskCreateLastUsed",
      localStorageRepoId,
      localStorageValid,
      settingsRepoId,
      settingsValid,
    };
  }
  return {
    pickId: repositories.length === 1 ? repositories[0].id : null,
    source: repositories.length === 1 ? "single-workspace-repo" : "empty-row",
    localStorageRepoId,
    localStorageValid,
    settingsRepoId,
    settingsValid,
  };
}

function isRepositoryIdValid(repositoryId: string | null, repositories: Repository[]): boolean {
  return Boolean(repositoryId && repositories.some((r: Repository) => r.id === repositoryId));
}

function logRepositoryAutoPick(
  workspaceId: string,
  repoCount: number,
  decision: RepositoryAutoPickDecision,
) {
  if (!isDebug()) return;
  selectionDebug("repository-autopick", {
    workspace_id: workspaceId,
    local_storage_id: decision.localStorageRepoId ?? "-",
    local_storage_valid: decision.localStorageValid,
    settings_id: decision.settingsRepoId ?? "-",
    settings_valid: decision.settingsValid,
    repo_count: repoCount,
    source: decision.source,
    pick: decision.pickId ?? "-",
  });
}

function buildRepositoryAutoPickRow(key: string, repositoryId: string): TaskRepoRow {
  return { key, repositoryId, branch: "" };
}

function canReplaceEmptyRepositoryPlaceholder(rows: TaskRepoRow[], pickId: string | null): boolean {
  if (!pickId || rows.length !== 1) return false;
  const row = rows[0];
  return Boolean(row && !row.repositoryId && !row.localPath && !row.branch);
}

export function useDiscoverReposEffect(
  fs: DialogFormState,
  open: boolean,
  workspaceId: string | null,
  repositoriesLoading: boolean,
  toast: ReturnType<typeof useToast>["toast"],
) {
  const {
    discoverReposLoaded,
    discoverReposLoading,
    setDiscoveredRepositories,
    setDiscoverReposLoading,
    setDiscoverReposLoaded,
  } = fs;
  useEffect(() => {
    if (!open || !workspaceId || repositoriesLoading || discoverReposLoaded || discoverReposLoading)
      return;
    void Promise.resolve()
      .then(() => setDiscoverReposLoading(true))
      .then(() => discoverRepositoriesAction(workspaceId))
      .then((r) => {
        setDiscoveredRepositories(r.repositories);
      })
      .catch((e) => {
        toast({
          title: "Failed to discover repositories",
          description: e instanceof Error ? e.message : "Request failed",
          variant: "error",
        });
        setDiscoveredRepositories([]);
      })
      .finally(() => {
        setDiscoverReposLoading(false);
        setDiscoverReposLoaded(true);
      });
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [
    discoverReposLoaded,
    discoverReposLoading,
    open,
    fs.discoveredRepositories.length,
    repositoriesLoading,
    toast,
    workspaceId,
  ]);
}

// Per-row branch listing now lives in the chip itself via useBranches, so the
// old useLocalBranchesEffect is gone.
//
// useCurrentLocalBranchEffect still earns its keep — the fresh-branch
// consent flow needs to know which branch the on-disk clone is currently on,
// and that's only meaningful for a single-row local-executor task. For multi-
// repo tasks fresh-branch is hidden in the UI, so we only resolve a path
// when there's exactly one row.
export function useCurrentLocalBranchEffect(
  fs: DialogFormState,
  open: boolean,
  workspaceId: string | null,
  repositories: Repository[],
) {
  const { repositories: rows, useRemote, setCurrentLocalBranch, setCurrentLocalBranchLoading } = fs;
  useEffect(() => {
    if (!open || !workspaceId || useRemote || rows.length !== 1) {
      setCurrentLocalBranch("");
      setCurrentLocalBranchLoading(false);
      return;
    }
    const row = rows[0];
    let path = row.localPath ?? "";
    if (!path && row.repositoryId) {
      const repo = repositories.find((r: Repository) => r.id === row.repositoryId);
      path = repo?.local_path ?? "";
    }
    if (!path) {
      setCurrentLocalBranch("");
      setCurrentLocalBranchLoading(false);
      return;
    }
    let cancelled = false;
    setCurrentLocalBranchLoading(true);
    getLocalRepositoryStatusAction(workspaceId, path)
      .then((r) => {
        if (cancelled) return;
        setCurrentLocalBranch(r.current_branch ?? "");
        setCurrentLocalBranchLoading(false);
      })
      .catch(() => {
        if (cancelled) return;
        setCurrentLocalBranch("");
        setCurrentLocalBranchLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, [
    open,
    workspaceId,
    useRemote,
    rows,
    repositories,
    setCurrentLocalBranch,
    setCurrentLocalBranchLoading,
  ]);
}

/**
 * Picks the default executor ID to auto-fill on dialog open. Repo-less tasks
 * skip the worktree executor (it needs a repo). Explicit local paths prefer
 * the local executor because the user chose an on-machine working tree.
 * Otherwise repo-backed tasks use the workspace default →
 * DEFAULT_LOCAL_EXECUTOR_TYPE → first available, in priority order.
 */
function pickDefaultExecutorId(
  executors: Executor[],
  workspaceDefaults: { default_executor_id?: string | null } | null | undefined,
  noRepository: boolean,
  preferLocalExecutor: boolean,
): string | null {
  const eligible =
    noRepository || preferLocalExecutor
      ? executors.filter((e: Executor) => e.type !== "worktree")
      : executors;
  if (eligible.length === 0) return null;
  const defId = workspaceDefaults?.default_executor_id ?? null;
  if (defId && eligible.some((e: Executor) => e.id === defId)) return defId;
  if (noRepository || preferLocalExecutor) {
    const directLocal = eligible.find((e: Executor) => isDirectLocalExecutorType(e.type));
    if (directLocal) return directLocal.id;
  }
  const local = eligible.find((e: Executor) => e.type === DEFAULT_LOCAL_EXECUTOR_TYPE);
  return local?.id ?? eligible[0].id;
}

type ExecutorProfileCandidate = ExecutorProfile & {
  _executorId: string;
  _executorType: string;
};

function isDirectLocalExecutorType(executorType: string | undefined): boolean {
  return executorType === "local" || executorType === "local_pc";
}

function isWorktreeExecutorType(executorType: string | undefined): boolean {
  return executorType === "worktree";
}

function flattenExecutorProfiles(executors: Executor[]): ExecutorProfileCandidate[] {
  return executors.flatMap((e) =>
    (e.profiles ?? []).map((p) => ({
      ...p,
      _executorId: e.id,
      _executorType: p.executor_type ?? e.type,
    })),
  );
}

function pickDefaultExecutorProfileId(
  executors: Executor[],
  workspaceDefaults: { default_executor_id?: string | null } | null | undefined,
  noRepository: boolean,
  preferLocalExecutor: boolean,
): string | null {
  const allProfiles = flattenExecutorProfiles(executors);
  if (allProfiles.length === 0) return null;
  const eligibleProfiles =
    noRepository || preferLocalExecutor
      ? allProfiles.filter((p) => !isWorktreeExecutorType(p._executorType))
      : allProfiles;
  if (eligibleProfiles.length === 0) return null;

  const lastId = getLocalStorage<string | null>(STORAGE_KEYS.LAST_EXECUTOR_PROFILE_ID, null);
  if (lastId && eligibleProfiles.some((p) => p.id === lastId)) return lastId;

  const executorId = pickDefaultExecutorId(
    executors,
    workspaceDefaults,
    noRepository,
    preferLocalExecutor,
  );
  const executorProfile = eligibleProfiles.find((p) => p._executorId === executorId);
  return executorProfile?.id ?? eligibleProfiles[0].id;
}

type ExecutorAutopickContext = {
  executors: Executor[];
  workspaceDefaults: StoreSelections["workspaceDefaults"];
  noRepository: boolean;
  preferLocalExecutor: boolean;
};

function useMultiRepoGuardEffect(
  open: boolean,
  executorProfileId: string,
  setExecutorProfileId: (id: string) => void,
  executors: Executor[],
  selectedRepoCount: number,
) {
  // Multi-repo guard: when 2+ repos are selected, only worktree profiles can
  // run the task (Docker / Sprites / standalone don't yet provision sibling
  // repos under one task root). If the current profile is non-worktree, swap
  // to a worktree profile — preferring the last-used worktree, otherwise the
  // first one available. Single-repo selections leave the profile alone.
  useEffect(() => {
    if (!open || !executorProfileId || executors.length === 0) return;
    if (selectedRepoCount <= 1) return;
    const profileToType = new Map<string, string | undefined>();
    const worktreeProfileIds: string[] = [];
    for (const e of executors) {
      for (const p of e.profiles ?? []) {
        const type = p.executor_type ?? e.type;
        profileToType.set(p.id, type);
        if (type === "worktree") worktreeProfileIds.push(p.id);
      }
    }
    if (worktreeProfileIds.length === 0) return;
    if (profileToType.get(executorProfileId) === "worktree") return;
    const lastId = getLocalStorage<string | null>(STORAGE_KEYS.LAST_EXECUTOR_PROFILE_ID, null);
    const pick = lastId && worktreeProfileIds.includes(lastId) ? lastId : worktreeProfileIds[0];
    void Promise.resolve().then(() => setExecutorProfileId(pick));
  }, [open, executorProfileId, executors, selectedRepoCount, setExecutorProfileId]);
}

function useExecutorIdAutopickEffect({
  open,
  executorId,
  context,
  setExecutorId,
}: {
  open: boolean;
  executorId: string;
  context: ExecutorAutopickContext;
  setExecutorId: (id: string) => void;
}) {
  const { executors, workspaceDefaults, noRepository, preferLocalExecutor } = context;
  useEffect(() => {
    if (!open || executorId || executors.length === 0) return;
    const pick = pickDefaultExecutorId(
      executors,
      workspaceDefaults,
      noRepository,
      preferLocalExecutor,
    );
    if (isDebug()) {
      selectionDebug("executor-autopick", {
        current: "-",
        pick: pick ?? "-",
        executor_count: executors.length,
        workspace_default: workspaceDefaults?.default_executor_id ?? "-",
        no_repository: noRepository,
        prefer_local_executor: preferLocalExecutor,
      });
    }
    if (pick) void Promise.resolve().then(() => setExecutorId(pick));
  }, [
    open,
    executorId,
    executors,
    workspaceDefaults,
    setExecutorId,
    noRepository,
    preferLocalExecutor,
  ]);
}

function useExecutorProfileAutopickEffect({
  open,
  executorProfileId,
  context,
  setExecutorProfileId,
}: {
  open: boolean;
  executorProfileId: string;
  context: ExecutorAutopickContext;
  setExecutorProfileId: (id: string) => void;
}) {
  const { executors, workspaceDefaults, noRepository, preferLocalExecutor } = context;
  useEffect(() => {
    // Auto-select executor profile: last used (localStorage) → source-aware
    // executor default → first eligible profile.
    if (!open || executorProfileId || executors.length === 0) return;
    const pick = pickDefaultExecutorProfileId(
      executors,
      workspaceDefaults,
      noRepository,
      preferLocalExecutor,
    );
    if (isDebug()) {
      const lastId = getLocalStorage<string | null>(STORAGE_KEYS.LAST_EXECUTOR_PROFILE_ID, null);
      const allProfiles = flattenExecutorProfiles(executors);
      selectionDebug("executor-profile-autopick", {
        current: "-",
        pick: pick ?? "-",
        local_storage_id: lastId ?? "-",
        local_storage_valid: Boolean(lastId && allProfiles.some((p) => p.id === lastId)),
        profile_count: allProfiles.length,
        workspace_default_executor: workspaceDefaults?.default_executor_id ?? "-",
        no_repository: noRepository,
        prefer_local_executor: preferLocalExecutor,
      });
    }
    if (pick) void Promise.resolve().then(() => setExecutorProfileId(pick));
  }, [
    open,
    executorProfileId,
    executors,
    workspaceDefaults,
    setExecutorProfileId,
    noRepository,
    preferLocalExecutor,
  ]);
}

export function useDefaultSelectionsEffect(
  fs: DialogFormState,
  open: boolean,
  sel: StoreSelections,
  workflows: Array<{ id: string; agent_profile_id?: string }>,
) {
  const { executors, workspaceDefaults } = sel;
  const {
    executorId,
    executorProfileId,
    setExecutorId,
    setExecutorProfileId,
    noRepository,
    useRemote,
    repositories,
    remoteRepos,
  } = fs;
  const preferLocalExecutor =
    !noRepository && !useRemote && repositories.some((row) => Boolean(row.localPath));
  const executorAutopickContext = useMemo(
    () => ({ executors, workspaceDefaults, noRepository, preferLocalExecutor }),
    [executors, workspaceDefaults, noRepository, preferLocalExecutor],
  );
  useAgentProfileAutopickEffect(fs, open, sel, workflows);
  useExecutorIdAutopickEffect({
    open,
    executorId,
    context: executorAutopickContext,
    setExecutorId,
  });
  useExecutorProfileAutopickEffect({
    open,
    executorProfileId,
    context: executorAutopickContext,
    setExecutorProfileId,
  });

  // Derive executorId from the selected executor profile
  useEffect(() => {
    if (!executorProfileId) return;
    for (const executor of executors) {
      const match = (executor.profiles ?? []).find((p) => p.id === executorProfileId);
      if (match) {
        if (isDebug()) {
          selectionDebug("executor-derived-from-profile", {
            executor_profile_id: executorProfileId,
            executor_id: executor.id,
          });
        }
        void Promise.resolve().then(() => setExecutorId(executor.id));
        return;
      }
    }
  }, [executorProfileId, executors, setExecutorId]);

  // Count is mode-aware: Remote mode counts non-empty URL rows, workspace
  // mode counts rows with a repo/path. Without this, 2 Remote rows + 0
  // workspace rows would slip past the guard because the legacy check only
  // inspected `fs.repositories` — `computeSelectedRepoCount` handles both.
  // Depend on the count primitive, not the whole `fs` object. `fs` is a fresh
  // literal every render, so listing it in the dep array would re-run this
  // effect on every render. computeSelectedRepoCount only reads noRepository /
  // useRemote / remoteRepos / repositories, so memoize over exactly those.
  const selectedRepoCount = useMemo(
    () =>
      computeSelectedRepoCount({
        noRepository,
        useRemote,
        remoteRepos,
        repositories,
      } as DialogFormState),
    [noRepository, useRemote, remoteRepos, repositories],
  );
  useMultiRepoGuardEffect(
    open,
    executorProfileId,
    setExecutorProfileId,
    executors,
    selectedRepoCount,
  );
}

/**
 * Surfaces a "Invalid GitHub URL" error for the first remote row when its URL
 * doesn't parse as a repo or a PR URL. Per-row PR-info fetching + branch
 * auto-select live inside `RemoteRepoChip` via `usePRInfoByURL` and
 * `useRowBranchAutoSelect`; this effect just keeps the surfaced error banner
 * in sync with the first row's URL.
 */
export function useGitHubUrlErrorEffect(fs: DialogFormState, open: boolean) {
  const { useRemote, setGitHubUrlError } = fs;
  const firstUrl = fs.remoteRepos[0]?.url ?? "";
  useEffect(() => {
    if (!open) return;
    // When the user leaves Remote mode (toggle off / switch to workspace
    // mode / dialog reopens in non-Remote mode) we must clear any stale
    // error left over from a previous Remote-mode pass. The early return
    // used to skip this — the banner stuck around after the field that
    // produced it had been hidden, which surfaced confusing "Invalid
    // GitHub URL" text alongside a repo picker.
    if (!useRemote) {
      setGitHubUrlError(null);
      return;
    }
    const trimmed = firstUrl.trim();
    if (!trimmed) {
      setGitHubUrlError(null);
      return;
    }
    const parsed = parseGitHubAnyUrl(trimmed);
    if (!parsed) {
      setGitHubUrlError("Invalid GitHub URL — expected github.com/owner/repo or .../pull/123");
      return;
    }
    setGitHubUrlError(null);
  }, [open, useRemote, firstUrl, setGitHubUrlError]);
}

export function useTaskCreateDialogEffects(fs: DialogFormState, args: TaskCreateEffectsArgs) {
  const { open, workspaceId, workflowId, repositories, repositoriesLoading } = args;
  const {
    agentProfiles,
    compatibleAgentProfiles,
    authLoaded,
    executors,
    workspaceDefaults,
    toast,
    workflows,
    isLocalExecutor,
  } = args;
  useWorkflowStepsEffect(fs, workflowId);
  useWorkflowAgentProfileEffect(fs, workflows, agentProfiles, compatibleAgentProfiles);
  useRepositoryAutoSelectEffect(fs, open, workspaceId, repositories, args.lastUsedRepositoryId);
  useDiscoverReposEffect(fs, open, workspaceId, repositoriesLoading, toast);
  useCurrentLocalBranchEffect(fs, open, workspaceId, repositories);
  useResetBranchOnLocalSwitchEffect(fs, isLocalExecutor, args.preserveBranch);
  useDefaultSelectionsEffect(
    fs,
    open,
    { agentProfiles, compatibleAgentProfiles, authLoaded, executors, workspaceDefaults },
    workflows,
  );
  useGitHubUrlErrorEffect(fs, open);
}

// Reset row.branch on every "switch to local executor" transition so the
// chip's autoselect effect can re-fire and prefer the workspace's current
// branch (preferredDefaultBranch). Without this, a branch the user picked
// under worktree mode (say "develop") would persist on the row, the chip
// would show "develop" after switching to local, and submit would carry
// "develop" → backend `git checkout develop` against the user's working
// tree. With the reset, switching to local always defaults to "current
// branch on disk" and the user has to opt back into a different branch
// explicitly.
function useResetBranchOnLocalSwitchEffect(
  fs: DialogFormState,
  isLocalExecutor: boolean,
  preserveBranch: string | undefined,
) {
  const { repositories: rows, updateRepository } = fs;
  const wasLocalRef = useRef(isLocalExecutor);
  useEffect(() => {
    const prev = wasLocalRef.current;
    wasLocalRef.current = isLocalExecutor;
    if (!isLocalExecutor || prev) return; // only fire on false → true transition
    for (const row of rows) {
      // Preserve a branch the caller asked us to keep (e.g. the PR head branch
      // when launching from a GitHub PR). Without this, the executor's async
      // settle on dialog open looks like a worktree→local switch and clobbers
      // the explicit branch choice, leaving the chip showing "current: main".
      // Both `row.branch` and `preserveBranch` are bare branch names with no
      // remote prefix — current callers (`initialValues.checkoutBranch` /
      // `initialValues.branch`) never pass `origin/...` here.
      if (row.branch && row.branch !== preserveBranch) {
        updateRepository(row.key, { branch: "" });
      }
    }
  }, [isLocalExecutor, rows, updateRepository, preserveBranch]);
}
