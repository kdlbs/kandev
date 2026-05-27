"use client";

import { useCallback } from "react";
import type { Dispatch, SetStateAction } from "react";
import type { Repository } from "@/lib/types/http";
import type {
  DialogFormState,
  TaskRemoteRepoRow,
  TaskRepoRow,
} from "@/components/task-create-dialog-types";
import { setLocalStorage } from "@/lib/local-storage";
import { STORAGE_KEYS } from "@/lib/settings/constants";

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
      if (isWorkspaceRepo) setLocalStorage(STORAGE_KEYS.LAST_REPOSITORY_ID, value);
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
    (value: string) => fs.setSelectedWorkflowId(value),
    [fs],
  );
  return {
    handleAgentProfileChange,
    handleExecutorProfileChange,
    handleTaskNameChange,
    handleGitHubBranchChange,
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
    fs.setUseRemote(!fs.useRemote);
    fs.setGitHubBranch("");
    fs.setGitHubUrlError(null);
    fs.setGitHubPrHeadBranch(null);
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
   * Routes the legacy single-URL onChange into the first remoteRepos row,
   * creating one if the list is empty. Mirrors the previous semantics
   * (clears the branch + PR-info on every URL change).
   */
  const handleGitHubUrlChange = useCallback(
    (value: string) => {
      const first = fs.remoteRepos[0];
      if (first) {
        fs.updateRemoteRepo(first.key, { url: value });
      } else {
        fs.setRemoteRepos([{ key: "remote-0", url: value, branch: "", source: "paste" }]);
      }
      fs.setGitHubBranch("");
      fs.setGitHubUrlError(null);
      fs.setGitHubPrHeadBranch(null);
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
      fs.setGitHubBranch("");
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
    handleGitHubUrlChange,
    handleToggleNoRepository,
    handleWorkspacePathChange,
  };
}

// --- Remote chip row helpers --------------------------------------------------
// Pure prop-builders for the new RemoteRepoChipsRow. They wrap the `setRemoteRepos`
// dispatch returned by `useRemoteReposState` into the change/add/remove
// callbacks the chip row expects, so the dialog can hand off without
// re-implementing the set-state shape at the call site.
//
// Key generation for `makeRemoteRowAdd` uses a module-level monotonic counter
// (paired with `Date.now()` to survive HMR), matching the stable React-key
// contract `useRemoteReposState` already establishes.

let remoteRowKeyCounter = 0;

function nextRemoteRowKey(): string {
  remoteRowKeyCounter += 1;
  return `remote-${Date.now().toString(36)}-${remoteRowKeyCounter}`;
}

export function makeRemoteRowChange(
  setRemoteRepos: Dispatch<SetStateAction<TaskRemoteRepoRow[]>>,
): (key: string, update: Partial<TaskRemoteRepoRow>) => void {
  return (key, update) => {
    setRemoteRepos((rows) => rows.map((r) => (r.key === key ? { ...r, ...update } : r)));
  };
}

export function makeRemoteRowAdd(
  setRemoteRepos: Dispatch<SetStateAction<TaskRemoteRepoRow[]>>,
): () => void {
  return () => {
    setRemoteRepos((rows) => [
      ...rows,
      { key: nextRemoteRowKey(), url: "", branch: "", source: "paste" },
    ]);
  };
}

export function makeRemoteRowRemove(
  setRemoteRepos: Dispatch<SetStateAction<TaskRemoteRepoRow[]>>,
): (key: string) => void {
  return (key) => {
    setRemoteRepos((rows) => rows.filter((r) => r.key !== key));
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
