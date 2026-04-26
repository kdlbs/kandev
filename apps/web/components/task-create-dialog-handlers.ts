"use client";

import { useCallback } from "react";
import type { Repository } from "@/lib/types/http";
import type { DialogFormState, TaskRepoRow } from "@/components/task-create-dialog-types";
import { setLocalStorage } from "@/lib/local-storage";
import { STORAGE_KEYS } from "@/lib/settings/constants";

/**
 * Centralizes form-field change handlers for the task-create dialog.
 *
 * The dialog now stores all repos in a single `fs.repositories` list (no
 * "primary vs extras" split), so the per-row handlers are uniform: changing
 * a repo on row N is the same op whether N==0 or N==5.
 */
export function useDialogHandlers(fs: DialogFormState, repositories: Repository[]) {
  /**
   * Resolves a picker value into the right shape for a row:
   * - If it matches a workspace repo id → `{ repositoryId, localPath: undefined }`.
   * - Otherwise treat as a discovered on-machine path → `{ localPath, repositoryId: undefined }`.
   * The branch is reset because the previous branch may not exist on the new repo.
   */
  const handleRowRepositoryChange = useCallback(
    (key: string, value: string) => {
      const isWorkspaceRepo = repositories.some((r: Repository) => r.id === value);
      const patch: Partial<TaskRepoRow> = isWorkspaceRepo
        ? { repositoryId: value, localPath: undefined, branch: "" }
        : { repositoryId: undefined, localPath: value, branch: "" };
      fs.updateRepository(key, patch);
      if (isWorkspaceRepo) setLocalStorage(STORAGE_KEYS.LAST_REPOSITORY_ID, value);
    },
    [repositories, fs],
  );

  const handleRowBranchChange = useCallback(
    (key: string, value: string) => {
      fs.updateRepository(key, { branch: value });
      setLocalStorage(STORAGE_KEYS.LAST_BRANCH, value);
    },
    [fs],
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
  const handleGitHubBranchChange = useCallback(
    (value: string) => {
      fs.setGitHubBranch(value);
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

  /**
   * Toggles between "repo chips" mode and "GitHub URL" mode. URL mode
   * replaces the chip row with a single URL input; flipping back restores
   * the chip flow with whatever rows were already there.
   */
  const handleToggleGitHubUrl = useCallback(() => {
    const next = !fs.useGitHubUrl;
    fs.setUseGitHubUrl(next);
    if (next) {
      // Stash the rows aside while in URL mode; flipping back keeps them.
      // (Nothing to do here — chips re-render off `fs.repositories` once
      // useGitHubUrl flips back to false.)
    } else {
      fs.setGitHubUrl("");
      fs.setGitHubBranches([]);
      fs.setGitHubUrlError(null);
      fs.setGitHubPrHeadBranch(null);
    }
    fs.setGitHubBranch("");
  }, [fs]);

  const handleGitHubUrlChange = useCallback(
    (value: string) => {
      fs.setGitHubUrl(value);
      fs.setGitHubBranch("");
      fs.setGitHubBranches([]);
      fs.setGitHubUrlError(null);
      fs.setGitHubPrHeadBranch(null);
    },
    [fs],
  );

  return {
    handleRowRepositoryChange,
    handleRowBranchChange,
    handleAgentProfileChange,
    handleExecutorProfileChange,
    handleTaskNameChange,
    handleGitHubBranchChange,
    handleWorkflowChange,
    handleToggleGitHubUrl,
    handleGitHubUrlChange,
  };
}
