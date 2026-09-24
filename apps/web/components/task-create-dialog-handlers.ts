"use client";

import { useCallback } from "react";
import type { Executor, Repository } from "@/lib/types/http";
import type { WorkspaceSourceRequest } from "@/lib/types/http-workspace-sources";
import type {
  DialogFormState,
  TaskRepoRow,
  TaskRepositorySelection,
} from "@/components/task-create-dialog-types";
import { resolveRepositorySelections } from "@/components/task-create-dialog-repositories-state";
import { createDebugLogger } from "@/lib/debug/log";
import type { TaskCreateLastUsedState } from "@/lib/state/slices/settings/types";
import { clampTaskTitleInput } from "@/lib/task-title";

type TaskCreateLastUsedPatch = {
  repository_id?: string | null;
  branch?: string | null;
  agent_profile_id?: string | null;
  executor_profile_id?: string | null;
  workspace_id?: string | null;
  workflow_id?: string | null;
  workspace_sources?: NonNullable<TaskCreateLastUsedState["workspaceSourcesByWorkspace"]>[string];
};

type TaskCreateLastUsedPayload = {
  workspace_id?: string;
  workflow_id?: string;
  repositories?: Array<{
    repository_id?: string;
    base_branch?: string;
    checkout_branch?: string;
    fresh_branch?: boolean;
  }>;
  workspace_sources?: WorkspaceSourceRequest[];
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
  "repositories" | "updateRepository" | "setExecutorId" | "setExecutorProfileId"
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
  executorSelection: DirectLocalExecutorSelection | null;
}) {
  upsertWorkspaceRepository(workspaceId, repository);
  if (!fs.repositories.some((row) => row.key === rowKey)) return;
  const selection = fs.repositories.length === 1 ? executorSelection : null;
  fs.updateRepository(rowKey, {
    repositoryId: repository.id,
    localPath: undefined,
    branch: "main",
    branchPolicyId: undefined,
  });
  if (selection) {
    fs.setExecutorId(selection.executorId);
    fs.setExecutorProfileId(selection.executorProfileId);
  }
  syncTaskCreateLastUsed({
    repository_id: repository.id,
    branch: "main",
    ...(selection ? { executor_profile_id: selection.executorProfileId } : {}),
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
    if (key === "workflowIdsByWorkspace") {
      const queued = value as Record<string, string>;
      const synced = settings?.workflowIdsByWorkspace ?? {};
      return Object.entries(queued).every(([workspaceId, workflowId]) => {
        return synced[workspaceId] === workflowId;
      });
    }
    if (key === "workspaceSourcesByWorkspace") {
      const queued = value as NonNullable<TaskCreateLastUsedState["workspaceSourcesByWorkspace"]>;
      const synced = settings?.workspaceSourcesByWorkspace ?? {};
      return Object.entries(queued).every(([workspaceId, sources]) => {
        if (!Object.prototype.hasOwnProperty.call(synced, workspaceId)) return false;
        return JSON.stringify(synced[workspaceId] ?? []) === JSON.stringify(sources);
      });
    }
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
    workflowIdsByWorkspace:
      pending.workspace_id && pending.workflow_id
        ? { [pending.workspace_id]: pending.workflow_id }
        : undefined,
    workspaceSourcesByWorkspace:
      pending.workspace_id && pending.workspace_sources !== undefined
        ? { [pending.workspace_id]: pending.workspace_sources }
        : undefined,
  };
}

function compactTaskCreateLastUsedState(state: Partial<TaskCreateLastUsedState>) {
  return Object.fromEntries(
    Object.entries(state).filter(([, value]) => value !== undefined),
  ) as Partial<TaskCreateLastUsedState>;
}

export function syncTaskCreateLastUsed(patch: TaskCreateLastUsedPatch) {
  const mapped = compactTaskCreateLastUsedState(mapTaskCreateLastUsedPatch(patch));
  lastQueuedLastUsed = mergeTaskCreateLastUsedState(lastQueuedLastUsed, mapped);
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
  const previousWorkflowIdsByWorkspace = lastQueuedLastUsed.workflowIdsByWorkspace;
  const previousWorkspaceSourcesByWorkspace = lastQueuedLastUsed.workspaceSourcesByWorkspace;
  lastQueuedLastUsed = {
    ...(previousWorkflowIdsByWorkspace
      ? { workflowIdsByWorkspace: previousWorkflowIdsByWorkspace }
      : {}),
    ...(previousWorkspaceSourcesByWorkspace
      ? { workspaceSourcesByWorkspace: previousWorkspaceSourcesByWorkspace }
      : {}),
  };
  const firstWorkspaceRepo = payload.repositories?.find((repo) => repo.repository_id);
  const firstSourceRepo = payload.workspace_sources?.find(
    (source): source is Extract<WorkspaceSourceRequest, { kind: "repository" }> =>
      source.kind === "repository" && Boolean(source.repository_id),
  );
  const sourceBranch = payload.workspace_sources?.find((source) => source.kind === "repository");
  syncTaskCreateLastUsed({
    workspace_id: payload.workspace_id,
    workflow_id: payload.workflow_id,
    repository_id: firstWorkspaceRepo?.repository_id ?? firstSourceRepo?.repository_id,
    branch: taskCreateLastUsedBranch(firstWorkspaceRepo, sourceBranch),
    agent_profile_id: payload.agent_profile_id,
    executor_profile_id: payload.executor_profile_id,
    workspace_sources:
      payload.workspace_sources === undefined
        ? undefined
        : payload.workspace_sources.map(mapTaskCreateLastUsedSource),
  });
}

function mapTaskCreateLastUsedSource(
  source: WorkspaceSourceRequest,
): NonNullable<TaskCreateLastUsedState["workspaceSourcesByWorkspace"]>[string][number] {
  if (source.kind === "folder") {
    return {
      kind: source.kind,
      ...presentSourceFields({
        local_path: source.local_path,
        display_name: source.display_name,
      }),
    };
  }
  return {
    kind: source.kind,
    ...presentSourceFields({
      repository_id: source.repository_id,
      local_path: source.local_path,
      github_url: source.github_url,
      remote_url: source.remote_url,
      provider: source.provider,
      provider_host: source.provider_host,
      provider_scope: source.provider_scope,
      provider_repo_id: source.provider_repo_id,
      provider_owner: source.provider_owner,
      provider_name: source.provider_name,
      checkout_source: source.checkout_source,
      expected_origin: source.expected_origin,
      base_branch: source.base_branch,
      checkout_branch: source.checkout_branch,
      branch_policy_id: source.branch_policy_id,
      pr_number: source.pr_number,
    }),
  };
}

function presentSourceFields<T extends Record<string, unknown>>(fields: T): Partial<T> {
  return Object.fromEntries(Object.entries(fields).filter(sourceFieldIsPresent)) as Partial<T>;
}

function sourceFieldIsPresent([key, value]: [string, unknown]): boolean {
  if (key === "pr_number") return value !== undefined;
  return Boolean(value);
}

function mergeTaskCreateLastUsedState(
  previous: Partial<TaskCreateLastUsedState>,
  patch: Partial<TaskCreateLastUsedState>,
): Partial<TaskCreateLastUsedState> {
  const merged = { ...previous, ...patch };
  if (patch.workflowIdsByWorkspace) {
    merged.workflowIdsByWorkspace = {
      ...(previous.workflowIdsByWorkspace ?? {}),
      ...patch.workflowIdsByWorkspace,
    };
  }
  if (patch.workspaceSourcesByWorkspace) {
    merged.workspaceSourcesByWorkspace = {
      ...(previous.workspaceSourcesByWorkspace ?? {}),
      ...patch.workspaceSourcesByWorkspace,
    };
  }
  return merged;
}

function taskCreateLastUsedPayloadBranch(
  repo: NonNullable<TaskCreateLastUsedPayload["repositories"]>[number],
) {
  if (repo.fresh_branch) return firstNonEmpty(repo.base_branch, repo.checkout_branch);
  return firstNonEmpty(repo.checkout_branch, repo.base_branch);
}

function taskCreateLastUsedBranch(
  workspaceRepo: NonNullable<TaskCreateLastUsedPayload["repositories"]>[number] | undefined,
  sourceRepo: WorkspaceSourceRequest | undefined,
) {
  if (workspaceRepo) return taskCreateLastUsedPayloadBranch(workspaceRepo);
  if (sourceRepo?.kind === "repository") {
    return firstNonEmpty(sourceRepo.checkout_branch, sourceRepo.base_branch);
  }
  return undefined;
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
      const wasLocalPath = Boolean(fs.repositories.find((row) => row.key === key)?.localPath);
      const isLocalPath = !isWorkspaceRepo && Boolean(value);
      const patch: Partial<TaskRepoRow> = isWorkspaceRepo
        ? {
            repositoryId: value,
            localPath: undefined,
            branch: "",
            baseBranch: undefined,
            branchPolicyId: undefined,
            checkoutSource: undefined,
            expectedOrigin: undefined,
            remoteBranches: undefined,
          }
        : {
            repositoryId: undefined,
            localPath: value,
            branch: "",
            baseBranch: undefined,
            branchPolicyId: undefined,
            checkoutSource: undefined,
            expectedOrigin: undefined,
            remoteBranches: undefined,
          };
      fs.updateRepository(key, patch);
      if (wasLocalPath !== isLocalPath) {
        fs.setExecutorId("");
        fs.setExecutorProfileId("");
      }
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
      fs.updateRepository(key, { branch: value, branchPolicyId: undefined });
      syncTaskCreateLastUsed({ branch: value });
    },
    [fs],
  );

  const handleRowPolicyChange = useCallback(
    (key: string, policyId: string, baseBranch: string) => {
      fs.updateRepository(key, {
        branch: baseBranch,
        baseBranch,
        branchPolicyId: policyId,
      });
      syncTaskCreateLastUsed({ branch: baseBranch });
    },
    [fs],
  );

  return { handleRowRepositoryChange, handleRowBranchChange, handleRowPolicyChange };
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
      fs.setExecutorChoiceTouched?.(true);
      fs.setAutomaticExecutorRestore?.(null);
      fs.setFolderOnlyExecutorNotice?.(false);
      syncTaskCreateLastUsed({ executor_profile_id: value });
    },
    [fs],
  );
  const handleTaskNameChange = useCallback(
    (value: string) => {
      const boundedValue = clampTaskTitleInput(value);
      fs.setTaskName(boundedValue);
      fs.setHasTitle(boundedValue.trim().length > 0);
    },
    [fs],
  );
  const handleWorkflowChange = useCallback(
    (value: string) => {
      fs.setSelectedWorkflowId(value);
      fs.setWorkflowAgentOverrides({});
    },
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
      fs.setPreferLocalExecutor(false);
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
    // Clear the executor selection in both directions so the destination
    // source mode can resolve its own policy. None mode will re-pick Local;
    // Repo mode will re-pick the workspace default or Worktree fallback.
    fs.setExecutorId("");
    fs.setExecutorProfileId("");
    fs.setPreferLocalExecutor(false);
    fs.setWorkspacePath("");
    if (next) {
      fs.setUseRemote(false);
      // None mode excludes Worktree, so its auto-fill effect picks a
      // non-worktree default.
      syncTaskCreateLastUsed({
        repository_id: null,
        branch: null,
        executor_profile_id: null,
      });
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

type DialogHandlerContext = {
  workspaceId: string | null;
  executors: Executor[];
  upsertWorkspaceRepository: (workspaceId: string, repository: Repository) => void;
};

function resolveCurrentExecutorType(
  fs: DialogFormState,
  executors: Executor[],
): string | undefined {
  const executor = executors.find((candidate) => candidate.id === fs.executorId);
  return (
    executor?.profiles?.find((profile) => profile.id === fs.executorProfileId)?.executor_type ??
    executor?.type ??
    executors
      .flatMap((candidate) => candidate.profiles ?? [])
      .find((profile) => profile.id === fs.executorProfileId)?.executor_type
  );
}

function selectionHasRepository(selection: TaskRepositorySelection): boolean {
  if (selection.kind === "folder") return false;
  if (selection.kind === "remote") return Boolean(selection.url.trim());
  return Boolean(selection.repositoryId || selection.localPath);
}

function selectionHasFolder(selection: TaskRepositorySelection): boolean {
  return selection.kind === "folder" && Boolean(selection.localPath.trim());
}

function useFolderOnlyExecutorHandlers(fs: DialogFormState, context?: DialogHandlerContext) {
  const executors = context?.executors ?? [];
  const directLocalExecutorSelection = findDirectLocalExecutorProfile(
    executors,
    fs.executorProfileId,
  );
  const currentExecutorType = resolveCurrentExecutorType(fs, executors);
  const transitionToFolderOnly = useCallback(
    (remaining: TaskRepositorySelection[], folderAdded = false) => {
      if (fs.executorChoiceTouched || currentExecutorType !== "worktree") return;
      const hasRepository = remaining.some(selectionHasRepository);
      const hasFolder = folderAdded || remaining.some(selectionHasFolder);
      if (hasRepository || !hasFolder) return;
      fs.setFolderOnlyExecutorNotice?.(true);
      if (!directLocalExecutorSelection) return;
      fs.setAutomaticExecutorRestore?.({
        executorId: fs.executorId,
        executorProfileId: fs.executorProfileId,
      });
      fs.setExecutorId(directLocalExecutorSelection.executorId);
      fs.setExecutorProfileId(directLocalExecutorSelection.executorProfileId);
    },
    [currentExecutorType, directLocalExecutorSelection, fs],
  );
  const onFolderSelectionAdded = useCallback(
    (wasEmpty: boolean) => {
      if (!wasEmpty) return;
      transitionToFolderOnly(resolveRepositorySelections(fs), true);
    },
    [fs, transitionToFolderOnly],
  );
  const onRepositorySelectionRemoved = useCallback(
    (remaining: TaskRepositorySelection[]) => transitionToFolderOnly(remaining),
    [transitionToFolderOnly],
  );
  const onRepositorySelectionAdded = useCallback(
    (wasFolderOnly: boolean) => {
      if (!wasFolderOnly || fs.executorChoiceTouched || !fs.automaticExecutorRestore) return;
      const restore = fs.automaticExecutorRestore;
      fs.setExecutorId(restore.executorId);
      fs.setExecutorProfileId(restore.executorProfileId);
      fs.setAutomaticExecutorRestore?.(null);
      fs.setFolderOnlyExecutorNotice?.(false);
    },
    [fs],
  );
  const onAllWorkspaceSourcesRemoved = useCallback(() => {
    if (!fs.executorChoiceTouched && fs.automaticExecutorRestore) {
      const restore = fs.automaticExecutorRestore;
      fs.setExecutorId(restore.executorId);
      fs.setExecutorProfileId(restore.executorProfileId);
    }
    fs.setAutomaticExecutorRestore?.(null);
    fs.setFolderOnlyExecutorNotice?.(false);
  }, [fs]);
  return {
    directLocalExecutorSelection,
    onFolderSelectionAdded,
    onRepositorySelectionAdded,
    onAllWorkspaceSourcesRemoved,
    onRepositorySelectionRemoved,
  };
}

export function useDialogHandlers(
  fs: DialogFormState,
  repositories: Repository[],
  context?: DialogHandlerContext,
) {
  const repo = useRepositoryHandlers(fs, repositories);
  const profile = useProfileAndNameHandlers(fs);
  const gh = useGitHubAndFreshBranchHandlers(fs);
  const executorHandlers = useFolderOnlyExecutorHandlers(fs, context);
  const handleLocalRepositoryCreated = useCallback(
    (rowKey: string, repository: Repository) => {
      if (!context?.workspaceId) return;
      applyCreatedLocalRepository({
        fs,
        rowKey,
        repository,
        workspaceId: context.workspaceId,
        upsertWorkspaceRepository: context.upsertWorkspaceRepository,
        executorSelection: executorHandlers.directLocalExecutorSelection,
      });
      if (fs.repositories.length === 1 && fs.repositories.some((row) => row.key === rowKey)) {
        clearFreshBranch(fs);
      }
    },
    [context, executorHandlers.directLocalExecutorSelection, fs],
  );
  return {
    ...repo,
    ...profile,
    ...gh,
    directLocalExecutorSelection: executorHandlers.directLocalExecutorSelection,
    handleLocalRepositoryCreated,
    onFolderSelectionAdded: executorHandlers.onFolderSelectionAdded,
    onRepositorySelectionAdded: executorHandlers.onRepositorySelectionAdded,
    onAllWorkspaceSourcesRemoved: executorHandlers.onAllWorkspaceSourcesRemoved,
    onRepositorySelectionRemoved: executorHandlers.onRepositorySelectionRemoved,
  };
}
