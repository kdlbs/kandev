"use client";

import { useCallback } from "react";
import type { Repository } from "@/lib/types/http";
import type { DialogFormState } from "@/components/task-create-dialog-types";
import { setLocalStorage } from "@/lib/local-storage";
import { STORAGE_KEYS } from "@/lib/settings/constants";

/**
 * Centralizes form-field change handlers for the task-create dialog. Lives
 * in its own file so the main state hook stays under the file-length lint cap.
 */
export function useDialogHandlers(fs: DialogFormState, repositories: Repository[]) {
  const handleSelectLocalRepository = useCallback(
    (path: string) => {
      fs.setDiscoveredRepoPath(path);
      fs.setSelectedLocalRepo(fs.discoveredRepositories.find((r) => r.path === path) ?? null);
      fs.setRepositoryId("");
      fs.setBranch("");
      fs.setLocalBranches([]);
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
        return;
      }
      handleSelectLocalRepository(value);
    },
    [repositories, fs, handleSelectLocalRepository],
  );

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
    (value: string) => {
      fs.setSelectedWorkflowId(value);
    },
    [fs],
  );

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
  }, [fs]);

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
    handleRepositoryChange,
    handleAgentProfileChange,
    handleExecutorProfileChange,
    handleTaskNameChange,
    handleBranchChange,
    handleWorkflowChange,
    handleToggleGitHubUrl,
    handleGitHubUrlChange,
  };
}
