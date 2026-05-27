"use client";

import { useEffect, useRef } from "react";
import type { Repository, Executor } from "@/lib/types/http";
import { DEFAULT_LOCAL_EXECUTOR_TYPE } from "@/lib/utils";
import { useToast } from "@/components/toast-provider";
import {
  discoverRepositoriesAction,
  getLocalRepositoryStatusAction,
} from "@/app/actions/workspaces";
import { listWorkflowSteps } from "@/lib/api/domains/workflow-api";
import { fetchPRInfo } from "@/lib/api/domains/github-api";
import { getLocalStorage } from "@/lib/local-storage";
import { STORAGE_KEYS } from "@/lib/settings/constants";
import { parseGitHubRepoUrl } from "@/lib/github/parse-url";
import type {
  DialogFormState,
  StoreSelections,
  TaskCreateEffectsArgs,
} from "@/components/task-create-dialog-types";
import {
  useAgentProfileAutopickEffect,
  useWorkflowAgentProfileEffect,
} from "@/components/task-create-dialog-autopick";

// Re-export autopick hooks for callers that imported them from this module.
export { useWorkflowAgentProfileEffect };
// Also re-exported for the test file, which expects the symbol to live here.
export { decideAgentProfileAutopick } from "@/components/task-create-dialog-autopick";

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

export function useRepositoryAutoSelectEffect(
  fs: DialogFormState,
  open: boolean,
  workspaceId: string | null,
  repositories: Repository[],
) {
  // On open, ensure there's always at least one chip rendered: prefer the
  // user's last-used repo (or the workspace's only repo) so the chip lands
  // pre-filled, but fall back to an empty row so the picker is visible
  // instead of just the "+" button. URL mode is excluded — that flow swaps
  // the chip row for a URL input.
  const { repositories: rows, useRemote, setRepositories } = fs;
  useEffect(() => {
    if (!open || !workspaceId || useRemote) return;
    if (rows.length > 0) return;
    const lastUsedRepoId = getLocalStorage<string | null>(STORAGE_KEYS.LAST_REPOSITORY_ID, null);
    let pickId: string | null = null;
    if (lastUsedRepoId && repositories.some((r: Repository) => r.id === lastUsedRepoId)) {
      pickId = lastUsedRepoId;
    } else if (repositories.length === 1) {
      pickId = repositories[0].id;
    }
    // Use the functional setter so the deferred microtask sees fresh state.
    // Without this, a sibling effect (resetTaskForm / useLockedFieldSync) that
    // seeds rows from `initialValues.repositoryId` synchronously races with
    // this microtask — the microtask captured rows.length === 0 at queue time
    // and would blindly clobber the initialValues-seeded row.
    void Promise.resolve().then(() => {
      setRepositories((prev) => {
        if (prev.length > 0) return prev;
        return [
          pickId
            ? { key: "row-0", repositoryId: pickId, branch: "" }
            : { key: "row-0", branch: "" },
        ];
      });
    });
  }, [open, repositories, rows, useRemote, workspaceId, setRepositories]);
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
 * skip the worktree executor (it needs a repo); other modes use the workspace
 * default → DEFAULT_LOCAL_EXECUTOR_TYPE → first available, in priority order.
 */
function pickDefaultExecutorId(
  executors: Executor[],
  workspaceDefaults: { default_executor_id?: string | null } | null | undefined,
  noRepository: boolean,
): string | null {
  const eligible = noRepository
    ? executors.filter((e: Executor) => e.type !== "worktree")
    : executors;
  if (eligible.length === 0) return null;
  const defId = workspaceDefaults?.default_executor_id ?? null;
  if (defId && eligible.some((e: Executor) => e.id === defId)) return defId;
  const local = eligible.find((e: Executor) => e.type === DEFAULT_LOCAL_EXECUTOR_TYPE);
  return local?.id ?? eligible[0].id;
}

export function useDefaultSelectionsEffect(
  fs: DialogFormState,
  open: boolean,
  sel: StoreSelections,
  workflows: Array<{ id: string; agent_profile_id?: string }>,
) {
  const { executors, workspaceDefaults } = sel;
  const { executorId, executorProfileId, setExecutorId, setExecutorProfileId, noRepository } = fs;
  useAgentProfileAutopickEffect(fs, open, sel, workflows);

  useEffect(() => {
    if (!open || executorId || executors.length === 0) return;
    const pick = pickDefaultExecutorId(executors, workspaceDefaults, noRepository);
    if (pick) void Promise.resolve().then(() => setExecutorId(pick));
  }, [open, executorId, executors, workspaceDefaults, setExecutorId, noRepository]);

  useEffect(() => {
    // Auto-select executor profile: last used (localStorage) → first available
    if (!open || executorProfileId || executors.length === 0) return;
    const allProfiles = executors.flatMap((e) =>
      (e.profiles ?? []).map((p) => ({ ...p, _executorId: e.id })),
    );
    if (allProfiles.length === 0) return;
    const lastId = getLocalStorage<string | null>(STORAGE_KEYS.LAST_EXECUTOR_PROFILE_ID, null);
    const pick = lastId && allProfiles.some((p) => p.id === lastId) ? lastId : allProfiles[0].id;
    void Promise.resolve().then(() => setExecutorProfileId(pick));
  }, [open, executorProfileId, executors, setExecutorProfileId]);

  // Derive executorId from the selected executor profile
  useEffect(() => {
    if (!executorProfileId) return;
    for (const executor of executors) {
      const match = (executor.profiles ?? []).find((p) => p.id === executorProfileId);
      if (match) {
        void Promise.resolve().then(() => setExecutorId(executor.id));
        return;
      }
    }
  }, [executorProfileId, executors, setExecutorId]);

  // Multi-repo guard: when 2+ repos are selected, only worktree profiles can
  // run the task (Docker / Sprites / standalone don't yet provision sibling
  // repos under one task root). If the current profile is non-worktree, swap
  // to a worktree profile — preferring the last-used worktree, otherwise the
  // first one available. Single-repo selections leave the profile alone.
  useEffect(() => {
    if (!open || !executorProfileId || executors.length === 0) return;
    const namedRepos = fs.repositories.filter((r) => r.repositoryId || r.localPath);
    if (namedRepos.length <= 1) return;
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
  }, [open, executorProfileId, executors, fs.repositories, setExecutorProfileId]);
}

/**
 * Auto-selects a sensible branch in the GitHub URL flow. Per-repo (workspace
 * or discovered) branch auto-select happens inside RepoChip when a row's
 * branches load — that keeps the per-row state confined to the chip.
 *
 * Branches are sourced from the per-URL `branchesByUrl` cache, keyed by the
 * first remote-repo row's URL (legacy single-URL flow).
 */
export function useBranchAutoSelectEffect(fs: DialogFormState) {
  const { githubBranch, useRemote, setGitHubBranch, githubPrHeadBranch } = fs;
  const firstUrl = fs.remoteRepos[0]?.url ?? "";
  const githubBranches = fs.branchesByUrl.branches(firstUrl);
  useEffect(() => {
    if (!useRemote || githubBranch) return;
    // PR head wins regardless of whether it appears in the base repo's branch
    // list. Fork PRs (head lives only on the contributor's fork) won't match
    // anything in githubBranches, but we still want the pill to surface the
    // PR's branch name — the worktree will materialize it from the PR refspec
    // on the backend, and falling through to "main" would visually contradict
    // the URL the user just pasted.
    if (githubPrHeadBranch) {
      setGitHubBranch(githubPrHeadBranch);
      return;
    }
    if (githubBranches.length === 0) return;
    // GitHub URL branches are referenced by name only (no remote prefix);
    // selectPreferredBranch expects origin-prefixed remotes, so pick directly.
    const preferred =
      githubBranches.find((b) => b.name === "main") ??
      githubBranches.find((b) => b.name === "master") ??
      githubBranches[0];
    if (preferred) setGitHubBranch(preferred.name);
  }, [githubBranch, githubBranches, useRemote, setGitHubBranch, githubPrHeadBranch]);
}

/** Parse a GitHub URL to extract owner, repo, and optional PR number. Returns null if invalid. */
function parseGitHubUrl(url: string): { owner: string; repo: string; prNumber?: number } | null {
  const trimmed = url.trim();
  if (!trimmed) return null;
  // Try PR URL first: github.com/owner/repo/pull/123 (with optional trailing path/hash like /files#diff-...)
  const prMatch = trimmed.match(
    /(?:https?:\/\/)?(?:www\.)?github\.com\/([A-Za-z0-9_.-]+)\/([A-Za-z0-9_.-]+)\/pull\/(\d+)(?:[/?#].*)?$/,
  );
  if (prMatch) {
    return { owner: prMatch[1], repo: prMatch[2], prNumber: parseInt(prMatch[3], 10) };
  }
  // Fall back to plain repo URL: github.com/owner/repo
  return parseGitHubRepoUrl(trimmed);
}

/**
 * Returns a callback that auto-fills the task name with `PR #N: <title>` when
 * a PR URL is pasted, leaving anything the user typed themselves alone. The
 * callback reads the latest taskName via a ref so the fetch effect doesn't
 * need to list taskName as a dep (which would re-fire the branches/PR fetch on
 * every keystroke in the title input).
 *
 * NOTE: Callers must omit the returned callback from `useEffect` dependency
 * arrays — including it causes the fetch to re-fire on every keystroke,
 * defeating the purpose of the ref. The call site uses
 * `eslint-disable react-hooks/exhaustive-deps` intentionally.
 */
export function useAutoFillTaskNameFromPR(fs: DialogFormState) {
  const { taskName, setTaskName, setHasTitle } = fs;
  const lastAutoFilledTitleRef = useRef("");
  const taskNameRef = useRef(taskName);
  useEffect(() => {
    taskNameRef.current = taskName;
    if (!taskName.trim()) {
      lastAutoFilledTitleRef.current = "";
    }
  }, [taskName]);
  return (prNumber: number, prTitle: string) => {
    const next = `PR #${prNumber}: ${prTitle}`;
    const current = taskNameRef.current;
    if (!current.trim() || current === lastAutoFilledTitleRef.current) {
      lastAutoFilledTitleRef.current = next;
      setTaskName(next);
      setHasTitle(true);
    }
  };
}

export function useGitHubUrlBranchesEffect(fs: DialogFormState, open: boolean) {
  const {
    useRemote,
    setGitHubUrlError,
    setGitHubPrHeadBranch,
    setGitHubPrBaseBranch,
    branchesByUrl,
  } = fs;
  const githubUrl = fs.remoteRepos[0]?.url ?? "";
  const autoFillTitle = useAutoFillTaskNameFromPR(fs);
  useEffect(() => {
    if (!open || !useRemote) return;
    const trimmed = githubUrl.trim();
    if (!trimmed) {
      setGitHubUrlError(null);
      return;
    }
    const parsed = parseGitHubUrl(githubUrl);
    if (!parsed) {
      setGitHubPrHeadBranch(null);
      setGitHubPrBaseBranch(null);
      setGitHubUrlError("Invalid GitHub URL — expected github.com/owner/repo or .../pull/123");
      return;
    }
    // Branches load via the per-URL hook cache (multi-row support via Task 5).
    branchesByUrl.ensure(githubUrl);
    let cancelled = false;
    setGitHubUrlError(null);
    setGitHubPrHeadBranch(null);
    setGitHubPrBaseBranch(null);

    // PR-info fetch stays here (autofills task name + carries head/base on the
    // payload). Only fires for PR URLs.
    const controller = new AbortController();
    const timeoutId = setTimeout(() => controller.abort(), 15_000);
    const prPromise = parsed.prNumber
      ? fetchPRInfo(parsed.owner, parsed.repo, parsed.prNumber, {
          init: { signal: controller.signal },
        }).catch((err: unknown) => {
          if (err instanceof DOMException && err.name === "AbortError") return null;
          const isNotConfigured = err instanceof Error && err.message.includes("not configured");
          if (!cancelled) {
            setGitHubUrlError(
              isNotConfigured
                ? "GitHub is not configured. Set up a token in Settings > GitHub."
                : "Repository not found or not accessible",
            );
          }
          return null;
        })
      : Promise.resolve(null);

    prPromise
      .then((prInfo) => {
        if (cancelled || !prInfo) return;
        setGitHubPrHeadBranch(prInfo.head_branch);
        setGitHubPrBaseBranch(prInfo.base_branch);
        autoFillTitle(prInfo.number, prInfo.title);
      })
      .finally(() => clearTimeout(timeoutId));
    return () => {
      cancelled = true;
      clearTimeout(timeoutId);
      controller.abort();
    };
    // autoFillTitle is intentionally omitted: it's a fresh closure each render
    // but reads the latest taskName via ref, so excluding it keeps the fetch
    // from re-firing on every keystroke. branchesByUrl reference is stable
    // (hook returns the same object identity); ensure() is idempotent per URL.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open, useRemote, githubUrl, setGitHubUrlError, setGitHubPrHeadBranch, setGitHubPrBaseBranch]);
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
  useRepositoryAutoSelectEffect(fs, open, workspaceId, repositories);
  useDiscoverReposEffect(fs, open, workspaceId, repositoriesLoading, toast);
  useBranchAutoSelectEffect(fs);
  useCurrentLocalBranchEffect(fs, open, workspaceId, repositories);
  useResetBranchOnLocalSwitchEffect(fs, isLocalExecutor, args.preserveBranch);
  useDefaultSelectionsEffect(
    fs,
    open,
    { agentProfiles, compatibleAgentProfiles, authLoaded, executors, workspaceDefaults },
    workflows,
  );
  useGitHubUrlBranchesEffect(fs, open);
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
