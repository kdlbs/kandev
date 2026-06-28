"use client";

import { useEffect, useMemo, useRef } from "react";
import type { Repository, Executor, ExecutorProfile } from "@/lib/types/http";
import { DEFAULT_LOCAL_EXECUTOR_TYPE } from "@/lib/utils";
import { useToast } from "@/components/toast-provider";
import {
  discoverRepositoriesAction,
  getLocalRepositoryStatusAction,
} from "@/app/actions/workspaces";
import { listWorkflowSteps } from "@/lib/api/domains/workflow-api";
import { getLocalStorage } from "@/lib/local-storage";
import { STORAGE_KEYS } from "@/lib/settings/constants";
import { parseGitHubAnyUrl } from "@/hooks/domains/github/use-pr-info-by-url";
import type {
  DialogFormState,
  StoreSelections,
  TaskCreateEffectsArgs,
} from "@/components/task-create-dialog-types";
import {
  useAgentProfileAutopickEffect,
  useWorkflowAgentProfileEffect,
} from "@/components/task-create-dialog-autopick";
import { useMultiRepoGuardEffect } from "@/components/task-create-dialog-multi-repo-guard";
import { useRepositoryAutoSelectEffect } from "@/components/task-create-dialog-repository-autopick";
import { computeSelectedRepoCount } from "@/components/task-create-dialog-computed";
import { createDebugLogger, isDebug } from "@/lib/debug/log";

// Re-export autopick hooks for callers that imported them from this module.
export { useWorkflowAgentProfileEffect };
export { useRepositoryAutoSelectEffect } from "@/components/task-create-dialog-repository-autopick";
// Also re-exported for the test file, which expects the symbol to live here.
export { decideAgentProfileAutopick } from "@/components/task-create-dialog-autopick";

const selectionDebug = createDebugLogger("task-create:selection");

export function useWorkflowStepsEffect(fs: DialogFormState, workflowId: string | null) {
  const { selectedWorkflowId, setFetchedSteps } = fs;
  useEffect(() => {
    if (!selectedWorkflowId || selectedWorkflowId === workflowId) {
      void Promise.resolve().then(() => setFetchedSteps(null));
      return;
    }
    let cancelled = false;
    listWorkflowSteps(selectedWorkflowId)
      .then((response) => {
        if (cancelled) return;
        const sorted = [...response.steps].sort((a, b) => a.position - b.position);
        setFetchedSteps(sorted.map((s) => ({ id: s.id, title: s.name, events: s.events })));
      })
      .catch(() => {
        if (!cancelled) setFetchedSteps(null);
      });
    return () => {
      cancelled = true;
    };
  }, [selectedWorkflowId, workflowId, setFetchedSteps]);
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
  lastUsedExecutorProfileId?: string | null,
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
  if (
    lastUsedExecutorProfileId &&
    eligibleProfiles.some((p) => p.id === lastUsedExecutorProfileId)
  ) {
    return lastUsedExecutorProfileId;
  }

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
  userSettingsLoaded?: boolean;
  lastUsedExecutorProfileId?: string | null;
  noRepository: boolean;
  preferLocalExecutor: boolean;
};

type ExecutorProfileLastUsedState = {
  allProfiles: ExecutorProfileCandidate[];
  eligibleProfiles: ExecutorProfileCandidate[];
  localStorageId: string | null;
  localStorageValid: boolean;
  settingsId: string | null;
  settingsValid: boolean;
};

function getExecutorProfileLastUsedState(
  context: ExecutorAutopickContext,
  settingsId: string | null,
): ExecutorProfileLastUsedState {
  const { executors, noRepository, preferLocalExecutor } = context;
  const allProfiles = flattenExecutorProfiles(executors);
  const eligibleProfiles =
    noRepository || preferLocalExecutor
      ? allProfiles.filter((p) => !isWorktreeExecutorType(p._executorType))
      : allProfiles;
  const localStorageId = getLocalStorage<string | null>(
    STORAGE_KEYS.LAST_EXECUTOR_PROFILE_ID,
    null,
  );
  return {
    allProfiles,
    eligibleProfiles,
    localStorageId,
    localStorageValid: Boolean(
      localStorageId && eligibleProfiles.some((p) => p.id === localStorageId),
    ),
    settingsId,
    settingsValid: Boolean(settingsId && eligibleProfiles.some((p) => p.id === settingsId)),
  };
}

function shouldDeferExecutorProfileAutopick(
  context: ExecutorAutopickContext,
  lastUsed: ExecutorProfileLastUsedState,
) {
  return (
    context.userSettingsLoaded === false && !lastUsed.localStorageValid && !lastUsed.settingsValid
  );
}

export function shouldWaitForLastUsedExecutorProfile(context: ExecutorAutopickContext) {
  const lastUsedProfile = getExecutorProfileLastUsedState(
    context,
    context.lastUsedExecutorProfileId ?? null,
  );
  if (!lastUsedProfile.localStorageValid && !lastUsedProfile.settingsValid) return false;
  if (isDebug()) {
    selectionDebug("executor-autopick", {
      current: "-",
      pick: "-",
      executor_count: context.executors.length,
      workspace_default: context.workspaceDefaults?.default_executor_id ?? "-",
      no_repository: context.noRepository,
      prefer_local_executor: context.preferLocalExecutor,
      source: "executor-profile-last-used",
    });
  }
  return true;
}

function logExecutorProfileAutopick(
  pick: string | null,
  context: ExecutorAutopickContext,
  lastUsed: ExecutorProfileLastUsedState,
  source?: string,
) {
  if (!isDebug()) return;
  selectionDebug("executor-profile-autopick", {
    current: "-",
    pick: pick ?? "-",
    local_storage_id: lastUsed.localStorageId ?? "-",
    local_storage_valid: lastUsed.localStorageValid,
    settings_id: lastUsed.settingsId ?? "-",
    settings_valid: lastUsed.settingsValid,
    profile_count: lastUsed.allProfiles.length,
    workspace_default_executor: context.workspaceDefaults?.default_executor_id ?? "-",
    no_repository: context.noRepository,
    prefer_local_executor: context.preferLocalExecutor,
    ...(source ? { source } : {}),
  });
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
    if (context.userSettingsLoaded === false) {
      if (isDebug()) {
        selectionDebug("executor-autopick", {
          current: "-",
          pick: "-",
          executor_count: executors.length,
          workspace_default: workspaceDefaults?.default_executor_id ?? "-",
          no_repository: noRepository,
          prefer_local_executor: preferLocalExecutor,
          source: "user-settings-loading",
        });
      }
      return;
    }
    if (shouldWaitForLastUsedExecutorProfile(context)) {
      return;
    }
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
    context.userSettingsLoaded,
    context.lastUsedExecutorProfileId,
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
    const lastUsed = getExecutorProfileLastUsedState(
      context,
      context.lastUsedExecutorProfileId ?? null,
    );
    if (shouldDeferExecutorProfileAutopick(context, lastUsed)) {
      logExecutorProfileAutopick(null, context, lastUsed, "user-settings-loading");
      return;
    }
    const pick = pickDefaultExecutorProfileId(
      executors,
      workspaceDefaults,
      noRepository,
      preferLocalExecutor,
      context.lastUsedExecutorProfileId,
    );
    logExecutorProfileAutopick(pick, context, lastUsed);
    if (pick) void Promise.resolve().then(() => setExecutorProfileId(pick));
  }, [
    open,
    executorProfileId,
    executors,
    workspaceDefaults,
    setExecutorProfileId,
    noRepository,
    preferLocalExecutor,
    context.lastUsedExecutorProfileId,
    context.userSettingsLoaded,
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
    () => ({
      executors,
      workspaceDefaults,
      userSettingsLoaded: sel.userSettingsLoaded,
      lastUsedExecutorProfileId: sel.lastUsedExecutorProfileId,
      noRepository,
      preferLocalExecutor,
    }),
    [
      executors,
      workspaceDefaults,
      sel.userSettingsLoaded,
      sel.lastUsedExecutorProfileId,
      noRepository,
      preferLocalExecutor,
    ],
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
  useWorkflowAgentProfileEffect(fs, workflows, agentProfiles, compatibleAgentProfiles, {
    lastUsedAgentProfileId: args.lastUsedAgentProfileId,
    authLoaded,
  });
  useRepositoryAutoSelectEffect(fs, open, workspaceId, repositories, {
    lastUsedRepositoryId: args.lastUsedRepositoryId,
    userSettingsLoaded: args.userSettingsLoaded,
  });
  useDiscoverReposEffect(fs, open, workspaceId, repositoriesLoading, toast);
  useCurrentLocalBranchEffect(fs, open, workspaceId, repositories);
  useResetBranchOnLocalSwitchEffect(fs, isLocalExecutor, args.preserveBranch);
  useDefaultSelectionsEffect(
    fs,
    open,
    {
      agentProfiles,
      compatibleAgentProfiles,
      authLoaded,
      executors,
      workspaceDefaults,
      userSettingsLoaded: args.userSettingsLoaded,
      lastUsedAgentProfileId: args.lastUsedAgentProfileId,
      lastUsedExecutorProfileId: args.lastUsedExecutorProfileId,
    },
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
