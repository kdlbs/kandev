"use client";

import { useEffect } from "react";
import type { Repository, Executor } from "@/lib/types/http";
import type { AgentProfileOption } from "@/lib/state/slices";
import { DEFAULT_LOCAL_EXECUTOR_TYPE } from "@/lib/utils";
import { useToast } from "@/components/toast-provider";
import {
  discoverRepositoriesAction,
  getLocalRepositoryStatusAction,
} from "@/app/actions/workspaces";
import { getLocalStorage } from "@/lib/local-storage";
import { STORAGE_KEYS } from "@/lib/settings/constants";
import { listWorkflowSteps } from "@/lib/api/domains/workflow-api";
import { fetchRepoBranches, fetchPRInfo } from "@/lib/api/domains/github-api";
import type {
  DialogFormState,
  StoreSelections,
  TaskCreateEffectsArgs,
} from "@/components/task-create-dialog-types";

export function useWorkflowAgentProfileEffect(
  fs: DialogFormState,
  workflows: Array<{ id: string; agent_profile_id?: string }>,
  agentProfiles: AgentProfileOption[],
) {
  const { selectedWorkflowId, setAgentProfileId, setWorkflowAgentProfileId } = fs;
  useEffect(() => {
    if (!selectedWorkflowId) {
      setWorkflowAgentProfileId("");
      return;
    }
    const workflow = workflows.find((w) => w.id === selectedWorkflowId);
    if (workflow?.agent_profile_id) {
      // Always lock the selector when the workflow specifies an agent profile.
      // This prevents the race condition where agentProfiles hasn't loaded yet.
      setWorkflowAgentProfileId(workflow.agent_profile_id);
      // Only set the agentProfileId once the profile is confirmed available.
      const profileExists = agentProfiles.some((p) => p.id === workflow.agent_profile_id);
      if (profileExists) {
        setAgentProfileId(workflow.agent_profile_id);
      }
    } else {
      setWorkflowAgentProfileId("");
      // Restore the user's last-used agent profile when unlocking
      const lastId = getLocalStorage<string | null>(STORAGE_KEYS.LAST_AGENT_PROFILE_ID, null);
      setAgentProfileId(lastId ?? "");
    }
  }, [selectedWorkflowId, workflows, agentProfiles, setAgentProfileId, setWorkflowAgentProfileId]);
}

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
  const { repositories: rows, useGitHubUrl, setRepositories } = fs;
  useEffect(() => {
    if (!open || !workspaceId || useGitHubUrl) return;
    if (rows.length > 0) return;
    const lastUsedRepoId = getLocalStorage<string | null>(STORAGE_KEYS.LAST_REPOSITORY_ID, null);
    let pickId: string | null = null;
    if (lastUsedRepoId && repositories.some((r: Repository) => r.id === lastUsedRepoId)) {
      pickId = lastUsedRepoId;
    } else if (repositories.length === 1) {
      pickId = repositories[0].id;
    }
    void Promise.resolve().then(() =>
      setRepositories([
        pickId ? { key: "row-0", repositoryId: pickId, branch: "" } : { key: "row-0", branch: "" },
      ]),
    );
  }, [open, repositories, rows, useGitHubUrl, workspaceId, setRepositories]);
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
  const { repositories: rows, useGitHubUrl, setCurrentLocalBranch } = fs;
  useEffect(() => {
    if (!open || !workspaceId || useGitHubUrl || rows.length !== 1) {
      setCurrentLocalBranch("");
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
      return;
    }
    let cancelled = false;
    getLocalRepositoryStatusAction(workspaceId, path)
      .then((r) => {
        if (!cancelled) setCurrentLocalBranch(r.current_branch ?? "");
      })
      .catch(() => {
        if (!cancelled) setCurrentLocalBranch("");
      });
    return () => {
      cancelled = true;
    };
  }, [open, workspaceId, useGitHubUrl, rows, repositories, setCurrentLocalBranch]);
}

export function useDefaultSelectionsEffect(
  fs: DialogFormState,
  open: boolean,
  sel: StoreSelections,
  workflows: Array<{ id: string; agent_profile_id?: string }>,
) {
  const { agentProfiles, executors, workspaceDefaults } = sel;
  const {
    agentProfileId,
    workflowAgentProfileId,
    selectedWorkflowId,
    executorId,
    executorProfileId,
    setAgentProfileId,
    setExecutorId,
    setExecutorProfileId,
  } = fs;
  useEffect(() => {
    // Check synchronously whether the selected workflow has an agent override.
    // This avoids a race condition where workflowAgentProfileId state hasn't
    // been committed yet by the workflow effect running in the same cycle.
    const workflowHasAgent = selectedWorkflowId
      ? workflows.some((w) => w.id === selectedWorkflowId && w.agent_profile_id)
      : false;
    if (
      !open ||
      agentProfileId ||
      workflowAgentProfileId ||
      workflowHasAgent ||
      agentProfiles.length === 0
    )
      return;
    const lastId = getLocalStorage<string | null>(STORAGE_KEYS.LAST_AGENT_PROFILE_ID, null);
    if (lastId && agentProfiles.some((p: AgentProfileOption) => p.id === lastId)) {
      void Promise.resolve().then(() => setAgentProfileId(lastId));
      return;
    }
    const defId = workspaceDefaults?.default_agent_profile_id ?? null;
    if (defId && agentProfiles.some((p: AgentProfileOption) => p.id === defId)) {
      void Promise.resolve().then(() => setAgentProfileId(defId));
      return;
    }
    void Promise.resolve().then(() => setAgentProfileId(agentProfiles[0].id));
  }, [
    open,
    agentProfileId,
    workflowAgentProfileId,
    selectedWorkflowId,
    workflows,
    agentProfiles,
    workspaceDefaults,
    setAgentProfileId,
  ]);

  useEffect(() => {
    if (!open || executorId || executors.length === 0) return;
    const defId = workspaceDefaults?.default_executor_id ?? null;
    if (defId && executors.some((e: Executor) => e.id === defId)) {
      void Promise.resolve().then(() => setExecutorId(defId));
      return;
    }
    const local = executors.find((e: Executor) => e.type === DEFAULT_LOCAL_EXECUTOR_TYPE);
    void Promise.resolve().then(() => setExecutorId(local?.id ?? executors[0].id));
  }, [open, executorId, executors, workspaceDefaults, setExecutorId]);

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
 */
export function useBranchAutoSelectEffect(fs: DialogFormState) {
  const { githubBranch, githubBranches, useGitHubUrl, setGitHubBranch, githubPrHeadBranch } = fs;
  useEffect(() => {
    if (!useGitHubUrl || githubBranches.length === 0 || githubBranch) return;
    if (githubPrHeadBranch) {
      const prBranch = githubBranches.find((b) => b.name === githubPrHeadBranch);
      if (prBranch) {
        setGitHubBranch(prBranch.name);
        return;
      }
    }
    // GitHub URL branches are referenced by name only (no remote prefix);
    // selectPreferredBranch expects origin-prefixed remotes, so pick directly.
    const preferred =
      githubBranches.find((b) => b.name === "main") ??
      githubBranches.find((b) => b.name === "master") ??
      githubBranches[0];
    if (preferred) setGitHubBranch(preferred.name);
  }, [githubBranch, githubBranches, useGitHubUrl, setGitHubBranch, githubPrHeadBranch]);
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
  // Fall back to repo URL: github.com/owner/repo
  const match = trimmed.match(
    /(?:https?:\/\/)?(?:www\.)?github\.com\/([A-Za-z0-9_.-]+)\/([A-Za-z0-9_.-]+?)(?:\.git)?\/?$/,
  );
  if (!match) return null;
  return { owner: match[1], repo: match[2] };
}

export function useGitHubUrlBranchesEffect(fs: DialogFormState, open: boolean) {
  const {
    useGitHubUrl,
    githubUrl,
    setGitHubBranches,
    setGitHubBranchesLoading,
    setGitHubUrlError,
    setGitHubPrHeadBranch,
  } = fs;
  useEffect(() => {
    if (!open || !useGitHubUrl) {
      setGitHubBranchesLoading(false);
      return;
    }
    const trimmed = githubUrl.trim();
    if (!trimmed) {
      setGitHubBranches([]);
      setGitHubBranchesLoading(false);
      setGitHubUrlError(null);
      return;
    }
    const parsed = parseGitHubUrl(githubUrl);
    if (!parsed) {
      setGitHubBranches([]);
      setGitHubPrHeadBranch(null);
      setGitHubBranchesLoading(false);
      setGitHubUrlError("Invalid GitHub URL — expected github.com/owner/repo or .../pull/123");
      return;
    }
    let cancelled = false;
    setGitHubUrlError(null);
    setGitHubBranchesLoading(true);
    setGitHubPrHeadBranch(null);

    const controller = new AbortController();
    const timeoutId = setTimeout(() => controller.abort(), 15_000);
    const fetchOpts = { init: { signal: controller.signal } };

    const branchesPromise = fetchRepoBranches(parsed.owner, parsed.repo, fetchOpts);
    const prPromise = parsed.prNumber
      ? fetchPRInfo(parsed.owner, parsed.repo, parsed.prNumber, fetchOpts).catch(() => null)
      : Promise.resolve(null);

    Promise.all([branchesPromise, prPromise])
      .then(([branchesRes, prInfo]) => {
        if (cancelled) return;
        setGitHubBranches(
          branchesRes.branches.map((b) => ({ name: b.name, type: "remote" as const })),
        );
        setGitHubUrlError(null);
        if (prInfo) {
          setGitHubPrHeadBranch(prInfo.head_branch);
        }
      })
      .catch((err) => {
        if (cancelled) return;
        const isAbort = err instanceof DOMException && err.name === "AbortError";
        const isNotConfigured = err instanceof Error && err.message.includes("not configured");
        let errorMessage = "Repository not found or not accessible";
        if (isAbort) {
          errorMessage = "Request timed out. Check your GitHub configuration in Settings.";
        } else if (isNotConfigured) {
          errorMessage = "GitHub is not configured. Set up a token in Settings > GitHub.";
        }
        setGitHubUrlError(errorMessage);
        setGitHubBranches([]);
      })
      .finally(() => {
        clearTimeout(timeoutId);
        if (!cancelled) setGitHubBranchesLoading(false);
      });
    return () => {
      cancelled = true;
      clearTimeout(timeoutId);
      controller.abort();
    };
  }, [
    open,
    useGitHubUrl,
    githubUrl,
    setGitHubBranches,
    setGitHubBranchesLoading,
    setGitHubUrlError,
    setGitHubPrHeadBranch,
  ]);
}

export function useTaskCreateDialogEffects(fs: DialogFormState, args: TaskCreateEffectsArgs) {
  const { open, workspaceId, workflowId, repositories, repositoriesLoading } = args;
  const { agentProfiles, executors, workspaceDefaults, toast, workflows } = args;
  useWorkflowStepsEffect(fs, workflowId);
  useWorkflowAgentProfileEffect(fs, workflows, agentProfiles);
  useRepositoryAutoSelectEffect(fs, open, workspaceId, repositories);
  useDiscoverReposEffect(fs, open, workspaceId, repositoriesLoading, toast);
  useBranchAutoSelectEffect(fs);
  useCurrentLocalBranchEffect(fs, open, workspaceId, repositories);
  useDefaultSelectionsEffect(fs, open, { agentProfiles, executors, workspaceDefaults }, workflows);
  useGitHubUrlBranchesEffect(fs, open);
}
