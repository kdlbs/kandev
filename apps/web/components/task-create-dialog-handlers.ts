"use client";

import { useCallback } from "react";
import type { Repository } from "@/lib/types/http";
import type { DialogFormState, TaskRepoRow } from "@/components/task-create-dialog-types";
import { setLocalStorage } from "@/lib/local-storage";
import { STORAGE_KEYS } from "@/lib/settings/constants";
import { updateUserSettings } from "@/lib/api/domains/settings-api";

function syncTaskCreateLastUsed(patch: {
  repository_id?: string;
  branch?: string;
  agent_profile_id?: string;
  executor_profile_id?: string;
}) {
  updateUserSettings({ task_create_last_used: patch }).catch(() => {});
}

/**
 * Centralizes form-field change handlers for the task-create dialog.
 *
 * The dialog stores all repos in a single `fs.repositories` list (no
 * "primary vs extras" split), so per-row handlers are uniform: changing
 * a repo on row N is the same op whether N==0 or N==5.
 *
 * Fresh-branch (local-executor opt-in: discard local changes and start on
 * a new branch) is a separate concern that lives alongside.
 */
function clearFreshBranch(fs: DialogFormState) {
  fs.setFreshBranchEnabled(false);
  fs.setCurrentLocalBranch("");
  // Set loading=true synchronously alongside the clear so the chip's
  // autoselect effect (which runs bottom-up before useCurrentLocalBranchEffect
  // can re-fire and set loading itself) sees the gate and skips. Otherwise
  // the autoselect lands a last-used / preferred-default branch in row.branch
  // before currentLocalBranch resolves, then the prefix logic computes
  // "will switch to: master" instead of "current: master".
  fs.setCurrentLocalBranchLoading(true);
}

function useRepositoryHandlers(fs: DialogFormState, repositories: Repository[]) {
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
      if (isWorkspaceRepo) {
        setLocalStorage(STORAGE_KEYS.LAST_REPOSITORY_ID, value);
        syncTaskCreateLastUsed({ repository_id: value });
      }
      // Switching the repo invalidates whatever local-status the fresh-branch
      // panel had cached.
      clearFreshBranch(fs);
    },
    [repositories, fs],
  );

  const handleRowBranchChange = useCallback(
    (key: string, value: string) => {
      fs.updateRepository(key, { branch: value });
      setLocalStorage(STORAGE_KEYS.LAST_BRANCH, value);
      syncTaskCreateLastUsed({ branch: value });
    },
    [fs],
  );

  return { handleRowRepositoryChange, handleRowBranchChange };
}

function useProfileAndNameHandlers(fs: DialogFormState) {
  const handleAgentProfileChange = useCallback(
    (value: string) => {
      fs.setAgentProfileId(value);
      setLocalStorage(STORAGE_KEYS.LAST_AGENT_PROFILE_ID, value);
      syncTaskCreateLastUsed({ agent_profile_id: value });
    },
    [fs],
  );
  const handleExecutorProfileChange = useCallback(
    (value: string) => {
      fs.setExecutorProfileId(value);
      setLocalStorage(STORAGE_KEYS.LAST_EXECUTOR_PROFILE_ID, value);
      syncTaskCreateLastUsed({ executor_profile_id: value });
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
  const handleWorkflowChange = useCallback(
    (value: string) => fs.setSelectedWorkflowId(value),
    [fs],
  );
  return {
    handleAgentProfileChange,
    handleExecutorProfileChange,
    handleTaskNameChange,
    handleWorkflowChange,
  };
}

function useGitHubAndFreshBranchHandlers(fs: DialogFormState) {
  /**
   * Toggles between "repo chips" mode and "GitHub Remote (URL)" mode. URL mode
   * replaces the chip row with a URL input; flipping back leaves
   * `remoteRepos` alone (toggle-back is non-destructive — Task 4 spec). The
   * seed effect in state.ts inserts a single empty row on the first toggle
   * into Remote mode.
   */
  const handleToggleRemote = useCallback(() => {
    const next = !fs.useRemote;
    fs.setUseRemote(next);
    fs.setGitHubUrlError(null);
    // Remote and no-repository are mutually exclusive source modes. Without
    // this, the user could land on both true at once (toggle no-repo on, then
    // toggle Remote on) and the submit gate's mode-aware checks would produce
    // confusing results. Mirror the no-repo handler which already clears
    // useRemote when flipping the other way.
    if (next) fs.setNoRepository(false);
    clearFreshBranch(fs);
  }, [fs]);

  const handleToggleFreshBranch = useCallback(
    (enabled: boolean) => {
      fs.setFreshBranchEnabled(enabled);
      // Clearing fs.repositories[].branch on toggle would force a re-pick from
      // the per-row branch list; for simplicity leave whatever the user picked.
      // The submit path re-validates anyway.
    },
    [fs],
  );

  /**
   * Toggles "no repository" mode. Replaces the chip row with a folder picker.
   * Clears the URL-mode flag and the workspace_path so flipping back returns
   * the user to a clean slate (the remoteRepos array itself is preserved).
   */
  const handleToggleNoRepository = useCallback(() => {
    const next = !fs.noRepository;
    fs.setNoRepository(next);
    if (next) {
      fs.setUseRemote(false);
      // Clear the executor selection so the auto-fill effect re-picks a
      // non-worktree default (worktree is unworkable in no-repo mode).
      fs.setExecutorId("");
      fs.setExecutorProfileId("");
    } else {
      fs.setWorkspacePath("");
    }
  }, [fs]);

  const handleWorkspacePathChange = useCallback(
    (value: string) => {
      fs.setWorkspacePath(value);
    },
    [fs],
  );

  return {
    handleToggleRemote,
    handleToggleFreshBranch,
    handleToggleNoRepository,
    handleWorkspacePathChange,
  };
}

export function useDialogHandlers(fs: DialogFormState, repositories: Repository[]) {
  const repo = useRepositoryHandlers(fs, repositories);
  const profile = useProfileAndNameHandlers(fs);
  const gh = useGitHubAndFreshBranchHandlers(fs);
  return {
    ...repo,
    ...profile,
    ...gh,
  };
}
