"use client";

import { useEffect } from "react";
import type { Repository, Executor, Branch } from "@/lib/types/http";
import type { AgentProfileOption } from "@/lib/state/slices";
import { DEFAULT_LOCAL_EXECUTOR_TYPE } from "@/lib/utils";
import { useToast } from "@/components/toast-provider";
import {
  discoverRepositoriesAction,
  listLocalRepositoryBranchesAction,
} from "@/app/actions/workspaces";
import { getLocalStorage } from "@/lib/local-storage";
import { STORAGE_KEYS } from "@/lib/settings/constants";
import { listWorkflowSteps } from "@/lib/api/domains/workflow-api";
import type {
  DialogFormState,
  StoreSelections,
  TaskCreateEffectsArgs,
} from "@/components/task-create-dialog-types";
import { autoSelectBranch } from "@/components/task-create-dialog-helpers";

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
  const { repositoryId, selectedLocalRepo, setRepositoryId } = fs;
  useEffect(() => {
    if (!open || !workspaceId || repositoryId || selectedLocalRepo) return;
    const lastUsedRepoId = getLocalStorage<string | null>(STORAGE_KEYS.LAST_REPOSITORY_ID, null);
    if (lastUsedRepoId && repositories.some((r: Repository) => r.id === lastUsedRepoId)) {
      void Promise.resolve().then(() => setRepositoryId(lastUsedRepoId));
      return;
    }
    if (repositories.length === 1)
      void Promise.resolve().then(() => setRepositoryId(repositories[0].id));
  }, [open, repositories, repositoryId, selectedLocalRepo, workspaceId, setRepositoryId]);
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

export function useLocalBranchesEffect(
  fs: DialogFormState,
  open: boolean,
  workspaceId: string | null,
  toast: ReturnType<typeof useToast>["toast"],
) {
  const { selectedLocalRepo, setLocalBranches, setLocalBranchesLoading } = fs;
  useEffect(() => {
    if (!open || !workspaceId || !selectedLocalRepo) return;
    const repoPath = selectedLocalRepo.path;
    void Promise.resolve()
      .then(() => setLocalBranchesLoading(true))
      .then(() => listLocalRepositoryBranchesAction(workspaceId, repoPath))
      .then((r) => {
        setLocalBranches(r.branches);
      })
      .catch((e) => {
        toast({
          title: "Failed to load branches",
          description: e instanceof Error ? e.message : "Request failed",
          variant: "error",
        });
        setLocalBranches([]);
      })
      .finally(() => {
        setLocalBranchesLoading(false);
      });
  }, [open, selectedLocalRepo, toast, workspaceId, setLocalBranches, setLocalBranchesLoading]);
}

export function useDefaultSelectionsEffect(
  fs: DialogFormState,
  open: boolean,
  sel: StoreSelections,
) {
  const { agentProfiles, executors, workspaceDefaults } = sel;
  const {
    agentProfileId,
    executorId,
    executorProfileId,
    setAgentProfileId,
    setExecutorId,
    setExecutorProfileId,
  } = fs;
  useEffect(() => {
    if (!open || agentProfileId || agentProfiles.length === 0) return;
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
  }, [open, agentProfileId, agentProfiles, workspaceDefaults, setAgentProfileId]);

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
}

export function useBranchAutoSelectEffect(fs: DialogFormState, branches: Branch[]) {
  const { repositoryId, branch, localBranches, setBranch } = fs;
  useEffect(() => {
    if (!repositoryId || branch) return;
    autoSelectBranch(branches, setBranch);
  }, [branch, branches, repositoryId, setBranch]);
  useEffect(() => {
    if (repositoryId || localBranches.length === 0 || branch) return;
    autoSelectBranch(localBranches, setBranch);
  }, [branch, localBranches, repositoryId, setBranch]);
}

export function useTaskCreateDialogEffects(fs: DialogFormState, args: TaskCreateEffectsArgs) {
  const { open, workspaceId, workflowId, repositories, repositoriesLoading, branches } = args;
  const { agentProfiles, executors, workspaceDefaults, toast } = args;
  useWorkflowStepsEffect(fs, workflowId);
  useRepositoryAutoSelectEffect(fs, open, workspaceId, repositories);
  useDiscoverReposEffect(fs, open, workspaceId, repositoriesLoading, toast);
  useBranchAutoSelectEffect(fs, branches);
  useLocalBranchesEffect(fs, open, workspaceId, toast);
  useDefaultSelectionsEffect(fs, open, { agentProfiles, executors, workspaceDefaults });
}
