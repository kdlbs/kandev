import type { StoreApi } from "zustand";
import type { AppState } from "@/lib/state/store";
import type { WsHandlers } from "@/lib/ws/handlers/types";
import type { GitStatusEntry } from "@/lib/state/slices/session-runtime/types";
import type {
  GitEventPayload,
  GitStatusUpdateEvent,
  GitCommitCreatedEvent,
  GitCommitsResetEvent,
  GitBranchSwitchedEvent,
} from "@/lib/types/git-events";
import { invalidateCumulativeDiffCache } from "@/hooks/domains/session/use-cumulative-diff";
import { createDebugLogger, isDebug } from "@/lib/debug/log";
import { acceptsGitStatusOrdering } from "@/lib/state/slices/session-runtime/git-status-state";

const debug = createDebugLogger("git-status:ws");

function logStatusUpdate(event: GitStatusUpdateEvent, changed: boolean) {
  if (!isDebug()) return;
  const taskEnvironmentId = event.task_environment_id;
  const statusState = event.status.status_state;
  const filesComplete =
    event.status.files_complete ?? (statusState === undefined && event.status.files !== undefined);
  debug("status_update", {
    sessionId: event.session_id,
    repositoryName: event.status.repository_name ?? null,
    branch: event.status.branch,
    fileCount: Object.keys(event.status.files ?? {}).length,
    modified: event.status.modified?.length ?? 0,
    added: event.status.added?.length ?? 0,
    deleted: event.status.deleted?.length ?? 0,
    untracked: event.status.untracked?.length ?? 0,
    ahead: event.status.ahead,
    behind: event.status.behind,
    remoteAhead: event.status.remote_ahead,
    remoteBehind: event.status.remote_behind,
    headCommit: event.status.head_commit,
    baseCommit: event.status.base_commit,
    remoteHeadCommit: event.status.remote_head_commit,
    envKey: taskEnvironmentId,
    envMapped: false,
    statusState,
    filesComplete,
    changed,
  });
}

// Handler functions for each event type
type GitEventHandlers = {
  status_update: (store: StoreApi<AppState>, event: GitStatusUpdateEvent) => void;
  commit_created: (store: StoreApi<AppState>, event: GitCommitCreatedEvent) => void;
  commits_reset: (store: StoreApi<AppState>, event: GitCommitsResetEvent) => void;
  branch_switched: (store: StoreApi<AppState>, event: GitBranchSwitchedEvent) => void;
};

/** Resolve sessionId → environmentId for non-status Git event cache keying. */
function resolveEnvKey(store: StoreApi<AppState>, sessionId: string): string {
  return store.getState().environmentIdBySessionId[sessionId] ?? sessionId;
}

function buildGitStatusEntry(event: GitStatusUpdateEvent): GitStatusEntry {
  return {
    status_state: event.status.status_state,
    files_complete: event.status.files_complete,
    detail_state: event.status.detail_state,
    error_code: event.status.error_code,
    tracker_id: event.status.tracker_id,
    tracker_epoch: event.status.tracker_epoch,
    snapshot_revision: event.status.snapshot_revision,
    branch: event.status.branch,
    remote_branch: event.status.remote_branch,
    modified: event.status.modified,
    added: event.status.added,
    deleted: event.status.deleted,
    untracked: event.status.untracked,
    renamed: event.status.renamed,
    ahead: event.status.ahead,
    behind: event.status.behind,
    head_commit: event.status.head_commit,
    base_commit: event.status.base_commit,
    comparison_target: event.status.comparison_target,
    comparison_status: event.status.comparison_status,
    comparison_error_code: event.status.comparison_error_code,
    remote_ahead: event.status.remote_ahead,
    remote_behind: event.status.remote_behind,
    remote_head_commit: event.status.remote_head_commit,
    files: event.status.files ?? {},
    timestamp: event.timestamp,
    branch_additions: event.status.branch_additions,
    branch_deletions: event.status.branch_deletions,
    // Multi-repo workspaces tag each status with the repository it belongs to;
    // setGitStatus routes the entry into byEnvironmentRepo accordingly.
    repository_name: event.status.repository_name,
    is_submodule: event.status.is_submodule,
  };
}

/** Applies only complete membership snapshots; transient frames update refresh state. */
export function applyGitStatusUpdate(
  store: StoreApi<AppState>,
  event: GitStatusUpdateEvent,
): boolean {
  const taskEnvironmentId = event.task_environment_id;
  if (!taskEnvironmentId) return false;
  const state = store.getState();

  const repositoryName = event.status.repository_name ?? "";
  const existing = getAcceptedGitStatus(state, taskEnvironmentId, repositoryName);
  const refresh = getGitStatusRefresh(state, taskEnvironmentId, repositoryName);
  const ordering = statusOrdering(event);
  if (
    !acceptsGitStatusOrdering(existing, ordering) ||
    !acceptsGitStatusOrdering(refresh, ordering)
  ) {
    return false;
  }
  return applyAcceptedGitStatus(store, event, taskEnvironmentId, repositoryName, ordering);
}

function getAcceptedGitStatus(state: AppState, environmentId: string, repositoryName: string) {
  return repositoryName
    ? state.gitStatus.byEnvironmentRepo[environmentId]?.[repositoryName]
    : state.gitStatus.byEnvironmentId[environmentId];
}

function getGitStatusRefresh(state: AppState, environmentId: string, repositoryName: string) {
  return repositoryName
    ? state.gitStatus.refreshByEnvironmentRepo?.[environmentId]?.[repositoryName]
    : (state.gitStatus.refreshByEnvironmentRepo?.[environmentId]?.[""] ??
        state.gitStatus.refreshByEnvironmentId?.[environmentId]);
}

function statusOrdering(event: GitStatusUpdateEvent) {
  return {
    tracker_id: event.status.tracker_id,
    tracker_epoch: event.status.tracker_epoch,
    snapshot_revision: event.status.snapshot_revision,
    timestamp: event.timestamp,
  };
}

function applyAcceptedGitStatus(
  store: StoreApi<AppState>,
  event: GitStatusUpdateEvent,
  environmentId: string,
  repositoryName: string,
  ordering: ReturnType<typeof statusOrdering>,
): boolean {
  const state = store.getState();
  if (isTransientStatus(event)) {
    state.setGitStatusRefresh(environmentId, repositoryName, {
      state: event.status.status_state === "unavailable" ? "unavailable" : "pending",
      error_code: event.status.error_code,
      ...ordering,
    });
    return false;
  }
  const filesComplete =
    event.status.files_complete ??
    (event.status.status_state === undefined && event.status.files !== undefined);
  if (!filesComplete) {
    state.setGitStatusRefresh(environmentId, repositoryName, {
      state: "pending",
      error_code: event.status.error_code,
      ...ordering,
    });
    return false;
  }

  const changed = state.setGitStatus(environmentId, buildGitStatusEntry(event));
  state.setGitStatusRefresh(
    environmentId,
    repositoryName,
    refreshForAcceptedStatus(event, ordering),
  );
  if (repositoryName === "") {
    state.setGitStatusRefresh(environmentId, undefined, null);
  }
  return changed;
}

function isTransientStatus(event: GitStatusUpdateEvent): boolean {
  return event.status.status_state === "unavailable" || event.status.status_state === "loading";
}

function refreshForAcceptedStatus(
  event: GitStatusUpdateEvent,
  ordering: ReturnType<typeof statusOrdering>,
) {
  return event.status.detail_state === "unavailable"
    ? { state: "unavailable" as const, error_code: event.status.error_code, ...ordering }
    : null;
}

const gitEventHandlers: GitEventHandlers = {
  status_update: (store, event) => {
    const taskEnvironmentId = event.task_environment_id;
    if (!taskEnvironmentId) {
      return;
    }
    // setGitStatus performs the deep change comparison once and reports back
    // whether anything changed; reuse that instead of comparing again here.
    // Under a massive rebase the comparison is the dominant CPU cost.
    const changed = applyGitStatusUpdate(store, event);
    logStatusUpdate(event, changed);
    if (changed) {
      invalidateCumulativeDiffCache(store, taskEnvironmentId);
    }
  },

  commit_created: (store, event) => {
    if (isDebug()) {
      debug("commit_created", {
        sessionId: event.session_id,
        sha: event.commit.commit_sha,
        repositoryName: event.commit.repository_name ?? null,
        filesChanged: event.commit.files_changed,
      });
    }
    store.getState().addSessionCommit(event.session_id, {
      id: event.commit.id,
      session_id: event.session_id,
      commit_sha: event.commit.commit_sha,
      parent_sha: event.commit.parent_sha,
      commit_message: event.commit.commit_message,
      author_name: event.commit.author_name,
      author_email: event.commit.author_email,
      files_changed: event.commit.files_changed,
      insertions: event.commit.insertions,
      deletions: event.commit.deletions,
      committed_at: event.commit.committed_at,
      created_at: event.commit.created_at ?? event.timestamp,
      // Multi-repo: tag the commit so the Commits panel can group per repo.
      repository_name: event.commit.repository_name,
    });
    // Invalidate cumulative diff cache when new commit is created
    invalidateCumulativeDiffCache(store, resolveEnvKey(store, event.session_id));
  },

  commits_reset: (store, event) => {
    if (isDebug())
      debug("commits_reset", {
        sessionId: event.session_id,
        repositoryName: event.reset.repository_name ?? null,
      });
    // Trigger a refetch without clearing the visible commits — the Changes
    // panel would otherwise flicker through its empty state ("Your changed
    // files will appear here") while the refetch is in flight, because
    // useSessionCommits returns `commits ?? []` and the panel's hasAnything
    // gate flips to false the moment commits goes undefined.
    store.getState().bumpSessionCommitsRefetch(event.session_id);
    store
      .getState()
      .bumpSessionGitCheckoutGeneration(event.session_id, event.reset.repository_name);
    // Invalidate cumulative diff cache when commits are reset
    invalidateCumulativeDiffCache(store, resolveEnvKey(store, event.session_id));
  },

  branch_switched: (store, event) => {
    if (isDebug())
      debug("branch_switched", {
        sessionId: event.session_id,
        repositoryName: event.branch_switch.repository_name ?? null,
      });
    // Stale-while-revalidate (see commits_reset above): refetch with the new
    // base commit but keep the old list visible until the new one arrives.
    store.getState().bumpSessionCommitsRefetch(event.session_id);
    store
      .getState()
      .bumpSessionGitCheckoutGeneration(event.session_id, event.branch_switch.repository_name);
    // Invalidate cumulative diff cache when branch switches
    invalidateCumulativeDiffCache(store, resolveEnvKey(store, event.session_id));
  },
};

export function registerGitStatusHandlers(store: StoreApi<AppState>): WsHandlers {
  return {
    "session.git.event": (message) => {
      const payload = message.payload as GitEventPayload;
      if (!payload.session_id || !payload.type) {
        return;
      }

      // Use switch for proper type narrowing
      switch (payload.type) {
        case "status_update":
          if (!payload.task_environment_id) {
            return;
          }
          gitEventHandlers.status_update(store, payload);
          break;
        case "commit_created":
          gitEventHandlers.commit_created(store, payload);
          break;
        case "commits_reset":
          gitEventHandlers.commits_reset(store, payload);
          break;
        case "branch_switched":
          gitEventHandlers.branch_switched(store, payload);
          break;
      }
    },
  };
}
