"use client";

import { useCallback } from "react";
import type { Executor, Repository } from "@/lib/types/http";
import type { DialogFormState, TaskRepoRow } from "@/components/task-create-dialog-types";
import { createDebugLogger } from "@/lib/debug/log";
import type { TaskCreateLastUsedState } from "@/lib/state/slices/settings/types";

type TaskCreateLastUsedPatch = {
  repository_id?: string | null;
  branch?: string | null;
  agent_profile_id?: string | null;
  executor_profile_id?: string | null;
};

type TaskCreateLastUsedPayload = {
  repositories?: Array<{
    repository_id?: string;
    base_branch?: string;
    checkout_branch?: string;
    fresh_branch?: boolean;
  }>;
  agent_profile_id?: string;
  executor_profile_id?: string;
};

export type DirectLocalExecutorSelection = {
  executorId: string;
  executorProfileId: string;
  executorProfileName: string;
  requiresSwitch: boolean;
};

function isDirectLocalExecutorType(type: string | undefined): boolean {
  return type === "local" || type === "local_pc";
}

export function findDirectLocalExecutorProfile(
  executors: Executor[],
  currentProfileId: string,
): DirectLocalExecutorSelection | null {
  const candidates = executors.flatMap((executor) =>
    (executor.profiles ?? [])
      .filter((profile) => isDirectLocalExecutorType(profile.executor_type ?? executor.type))
      .map((profile) => ({
        executorId: executor.id,
        executorProfileId: profile.id,
        executorProfileName: profile.name,
        requiresSwitch: profile.id !== currentProfileId,
      })),
  );
  return (
    candidates.find((candidate) => candidate.executorProfileId === currentProfileId) ??
    candidates[0] ??
    null
  );
}

type CreatedLocalRepositoryForm = Pick<
  DialogFormState,
  "updateRepository" | "setExecutorId" | "setExecutorProfileId"
>;

export function applyCreatedLocalRepository({
  fs,
  rowKey,
  repository,
  workspaceId,
  upsertWorkspaceRepository,
  executorSelection,
}: {
  fs: CreatedLocalRepositoryForm;
  rowKey: string;
  repository: Repository;
  workspaceId: string;
  upsertWorkspaceRepository: (workspaceId: string, repository: Repository) => void;
  executorSelection: DirectLocalExecutorSelection;
}) {
  fs.updateRepository(rowKey, {
    repositoryId: repository.id,
    localPath: undefined,
    branch: "main",
  });
  fs.setExecutorId(executorSelection.executorId);
  fs.setExecutorProfileId(executorSelection.executorProfileId);
  upsertWorkspaceRepository(workspaceId, repository);
  syncTaskCreateLastUsed({
    repository_id: repository.id,
    branch: "main",
    executor_profile_id: executorSelection.executorProfileId,
  });
}

let lastQueuedLastUsed: Partial<TaskCreateLastUsedState> = {};
const lastUsedDebug = createDebugLogger("task-create:last-used");

/**
 * Clears task-create last-used overlay state.
 * Pass `clearQueued` when test setup or teardown should also wipe the queued
 * overlay that protects settings fetches from stale server values.
 */
export function resetTaskCreateLastUsedSync(
  options: {
    clearQueued?: boolean;
    syncedSettings?: TaskCreateLastUsedState | null | undefined;
  } = {},
) {
  if (options.clearQueued) {
    lastQueuedLastUsed = {};
  } else if (taskCreateLastUsedSettingsMatchQueue(options.syncedSettings)) {
    lastQueuedLastUsed = {};
  }
  lastUsedDebug("overlay-reset");
}

export function readQueuedTaskCreateLastUsedState(): Partial<TaskCreateLastUsedState> {
  return lastQueuedLastUsed;
}

export function clearQueuedTaskCreateLastUsedIfSynced(
  settings: TaskCreateLastUsedState | null | undefined,
) {
  if (!hasQueuedTaskCreateLastUsed()) return;
  if (!taskCreateLastUsedSettingsMatchQueue(settings)) return;
  lastQueuedLastUsed = {};
  lastUsedDebug("overlay-cleared-after-settings-sync");
}

function hasQueuedTaskCreateLastUsed() {
  return Object.values(lastQueuedLastUsed).some((value) => value !== undefined);
}

function taskCreateLastUsedSettingsMatchQueue(
  settings: TaskCreateLastUsedState | null | undefined,
) {
  return Object.entries(lastQueuedLastUsed).every(([key, value]) => {
    if (value === undefined) return true;
    return settings?.[key as keyof TaskCreateLastUsedState] === value;
  });
}

function mapTaskCreateLastUsedPatch(
  pending: TaskCreateLastUsedPatch,
): Partial<TaskCreateLastUsedState> {
  return {
    repositoryId: pending.repository_id,
    branch: pending.branch,
    agentProfileId: pending.agent_profile_id,
    executorProfileId: pending.executor_profile_id,
  };
}

function compactTaskCreateLastUsedState(state: Partial<TaskCreateLastUsedState>) {
  return Object.fromEntries(
    Object.entries(state).filter(([, value]) => value !== undefined),
  ) as Partial<TaskCreateLastUsedState>;
}

export function syncTaskCreateLastUsed(patch: TaskCreateLastUsedPatch) {
  lastQueuedLastUsed = {
    ...lastQueuedLastUsed,
    ...compactTaskCreateLastUsedState(mapTaskCreateLastUsedPatch(patch)),
  };
  lastUsedDebug("overlay-updated", { patch, queued: lastQueuedLastUsed });
}

export function replaceQueuedTaskCreateLastUsed(patch: TaskCreateLastUsedPatch) {
  lastQueuedLastUsed = compactTaskCreateLastUsedState(mapTaskCreateLastUsedPatch(patch));
  lastUsedDebug("overlay-replaced", { patch, queued: lastQueuedLastUsed });
}

export function queueTaskCreateLastUsedFromPayload(
  payload: TaskCreateLastUsedPayload | null | undefined,
) {
  if (!payload) return;
  const firstWorkspaceRepo = payload.repositories?.find((repo) => repo.repository_id);
  replaceQueuedTaskCreateLastUsed({
    repository_id: firstWorkspaceRepo?.repository_id,
    branch: firstWorkspaceRepo ? taskCreateLastUsedPayloadBranch(firstWorkspaceRepo) : undefined,
    agent_profile_id: payload.agent_profile_id,
    executor_profile_id: payload.executor_profile_id,
  });
}

function taskCreateLastUsedPayloadBranch(
  repo: NonNullable<TaskCreateLastUsedPayload["repositories"]>[number],
) {
  if (repo.fresh_branch) return firstNonEmpty(repo.base_branch, repo.checkout_branch);
  return firstNonEmpty(repo.checkout_branch, repo.base_branch);
}

function firstNonEmpty(...values: Array<string | undefined>) {
  return values.find((value) => value) ?? undefined;
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
        syncTaskCreateLastUsed({ repository_id: value, branch: null });
      } else {
        syncTaskCreateLastUsed({ repository_id: null, branch: null });
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
      syncTaskCreateLastUsed({ agent_profile_id: value });
    },
    [fs],
  );
  const handleExecutorProfileChange = useCallback(
    (value: string) => {
      fs.setExecutorProfileId(value);
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
    if (next) {
      fs.setNoRepository(false);
      syncTaskCreateLastUsed({ repository_id: null, branch: null });
    }
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
      syncTaskCreateLastUsed({
        repository_id: null,
        branch: null,
        executor_profile_id: null,
      });
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

export function useDialogHandlers(
  fs: DialogFormState,
  repositories: Repository[],
  context?: {
    workspaceId: string | null;
    executors: Executor[];
    upsertWorkspaceRepository: (workspaceId: string, repository: Repository) => void;
  },
) {
  const repo = useRepositoryHandlers(fs, repositories);
  const profile = useProfileAndNameHandlers(fs);
  const gh = useGitHubAndFreshBranchHandlers(fs);
  const directLocalExecutorSelection = findDirectLocalExecutorProfile(
    context?.executors ?? [],
    fs.executorProfileId,
  );
  const handleLocalRepositoryCreated = useCallback(
    (rowKey: string, repository: Repository) => {
      if (!context?.workspaceId || !directLocalExecutorSelection) return;
      applyCreatedLocalRepository({
        fs,
        rowKey,
        repository,
        workspaceId: context.workspaceId,
        upsertWorkspaceRepository: context.upsertWorkspaceRepository,
        executorSelection: directLocalExecutorSelection,
      });
      clearFreshBranch(fs);
    },
    [context, directLocalExecutorSelection, fs],
  );
  return {
    ...repo,
    ...profile,
    ...gh,
    directLocalExecutorSelection,
    handleLocalRepositoryCreated,
  };
}
