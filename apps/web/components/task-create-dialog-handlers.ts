"use client";

import { useCallback } from "react";
import type { Repository } from "@/lib/types/http";
import { setLocalStorage } from "@/lib/local-storage";
import { STORAGE_KEYS } from "@/lib/settings/constants";
import type { DialogFormState } from "@/components/task-create-dialog-types";

function clearFreshBranch(fs: DialogFormState) {
  fs.setFreshBranchEnabled(false);
  fs.setCurrentLocalBranch("");
}

function useRepositoryHandlers(fs: DialogFormState, repositories: Repository[]) {
  const handleSelectLocalRepository = useCallback(
    (path: string) => {
      fs.setDiscoveredRepoPath(path);
      fs.setSelectedLocalRepo(fs.discoveredRepositories.find((r) => r.path === path) ?? null);
      fs.setRepositoryId("");
      fs.setBranch("");
      fs.setLocalBranches([]);
      clearFreshBranch(fs);
    },
    [fs],
  );

  const handleRepositoryChange = useCallback(
    (value: string) => {
      if (repositories.find((r: Repository) => r.id === value)) {
        fs.setRepositoryId(value);
        setLocalStorage(STORAGE_KEYS.LAST_REPOSITORY_ID, value);
        fs.setDiscoveredRepoPath("");
        fs.setSelectedLocalRepo(null);
        fs.setLocalBranches([]);
        fs.setBranch("");
        fs.setUseGitHubUrl(false);
        fs.setGitHubUrl("");
        fs.setGitHubBranches([]);
        clearFreshBranch(fs);
        return;
      }
      handleSelectLocalRepository(value);
    },
    [repositories, fs, handleSelectLocalRepository],
  );

  return { handleRepositoryChange, handleSelectLocalRepository };
}

function useBranchAndProfileHandlers(fs: DialogFormState) {
  const handleAgentProfileChange = useCallback(
    (value: string) => {
      fs.setAgentProfileId(value);
      setLocalStorage(STORAGE_KEYS.LAST_AGENT_PROFILE_ID, value);
    },
    [fs],
  );
  const handleExecutorProfileChange = useCallback(
    (value: string) => {
      fs.setExecutorProfileId(value);
      setLocalStorage(STORAGE_KEYS.LAST_EXECUTOR_PROFILE_ID, value);
    },
    [fs],
  );
  const handleTaskNameChange = useCallback(
    (value: string) => {
      fs.setTaskName(value);
      fs.setHasTitle(value.trim().length > 0);
    },
    [fs],
  );
  const handleBranchChange = useCallback(
    (value: string) => {
      fs.setBranch(value);
      setLocalStorage(STORAGE_KEYS.LAST_BRANCH, value);
    },
    [fs],
  );
  const handleWorkflowChange = useCallback(
    (value: string) => fs.setSelectedWorkflowId(value),
    [fs],
  );
  return {
    handleAgentProfileChange,
    handleExecutorProfileChange,
    handleTaskNameChange,
    handleBranchChange,
    handleWorkflowChange,
  };
}

function useGitHubAndFreshBranchHandlers(fs: DialogFormState) {
  const handleToggleGitHubUrl = useCallback(() => {
    const next = !fs.useGitHubUrl;
    fs.setUseGitHubUrl(next);
    if (next) {
      fs.setRepositoryId("");
      fs.setDiscoveredRepoPath("");
      fs.setSelectedLocalRepo(null);
      fs.setLocalBranches([]);
    } else {
      fs.setGitHubUrl("");
      fs.setGitHubBranches([]);
      fs.setGitHubUrlError(null);
      fs.setGitHubPrHeadBranch(null);
    }
    fs.setBranch("");
    clearFreshBranch(fs);
  }, [fs]);

  const handleToggleFreshBranch = useCallback(
    (enabled: boolean) => {
      fs.setFreshBranchEnabled(enabled);
      // Always clear the branch so we don't carry a value over from a
      // different executor (e.g. "develop" picked under worktree) into the
      // newly-enabled fresh-branch picker.
      fs.setBranch("");
    },
    [fs],
  );

  const handleGitHubUrlChange = useCallback(
    (value: string) => {
      fs.setGitHubUrl(value);
      fs.setBranch("");
      fs.setGitHubBranches([]);
      fs.setGitHubUrlError(null);
      fs.setGitHubPrHeadBranch(null);
    },
    [fs],
  );

  return {
    handleToggleGitHubUrl,
    handleToggleFreshBranch,
    handleGitHubUrlChange,
  };
}

export function useDialogHandlers(fs: DialogFormState, repositories: Repository[]) {
  const repo = useRepositoryHandlers(fs, repositories);
  const profile = useBranchAndProfileHandlers(fs);
  const gh = useGitHubAndFreshBranchHandlers(fs);
  return {
    handleRepositoryChange: repo.handleRepositoryChange,
    ...profile,
    ...gh,
  };
}
