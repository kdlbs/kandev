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

const debug = createDebugLogger("git-status:ws");

// Handler functions for each event type
type GitEventHandlers = {
  status_update: (store: StoreApi<AppState>, event: GitStatusUpdateEvent) => void;
  commit_created: (store: StoreApi<AppState>, event: GitCommitCreatedEvent) => void;
  commits_reset: (store: StoreApi<AppState>, event: GitCommitsResetEvent) => void;
  branch_switched: (store: StoreApi<AppState>, event: GitBranchSwitchedEvent) => void;
};

/** Resolve sessionId → environmentId for cache keying. */
function resolveEnvKey(store: StoreApi<AppState>, sessionId: string): string {
  return store.getState().environmentIdBySessionId[sessionId] ?? sessionId;
}

function buildGitStatusEntry(event: GitStatusUpdateEvent): GitStatusEntry {
  return {
    branch: event.status.branch,
    remote_branch: event.status.remote_branch,
    modified: event.status.modified,
    added: event.status.added,
    deleted: event.status.deleted,
    untracked: event.status.untracked,
    renamed: event.status.renamed,
    ahead: event.status.ahead,
    behind: event.status.behind,
    files: event.status.files,
    timestamp: event.timestamp,
    branch_additions: event.status.branch_additions,
    branch_deletions: event.status.branch_deletions,
    // Multi-repo workspaces tag each status with the repository it belongs to;
    // setGitStatus routes the entry into byEnvironmentRepo accordingly.
    repository_name: event.status.repository_name,
    // HEAD + merge-base SHAs power hasGitStatusChanged's fast-path fingerprint.
    head_commit: event.status.head_commit,
    base_commit: event.status.base_commit,
  };
}

const gitEventHandlers: GitEventHandlers = {
  status_update: (store, event) => {
    const gitStatus = buildGitStatusEntry(event);
    // setGitStatus performs the deep change comparison once and reports back
    // whether anything changed; reuse that instead of comparing again here.
    // Under a massive rebase the comparison is the dominant CPU cost.
    const changed = store.getState().setGitStatus(event.session_id, gitStatus);
    if (isDebug()) {
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
        envKey: resolveEnvKey(store, event.session_id),
        envMapped: event.session_id in store.getState().environmentIdBySessionId,
        changed,
      });
    }
    if (changed) {
      invalidateCumulativeDiffCache(resolveEnvKey(store, event.session_id));
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
    invalidateCumulativeDiffCache(resolveEnvKey(store, event.session_id));
  },

  commits_reset: (store, event) => {
    if (isDebug()) debug("commits_reset", { sessionId: event.session_id });
    // Trigger a refetch without clearing the visible commits — the Changes
    // panel would otherwise flicker through its empty state ("Your changed
    // files will appear here") while the refetch is in flight, because
    // useSessionCommits returns `commits ?? []` and the panel's hasAnything
    // gate flips to false the moment commits goes undefined.
    store.getState().bumpSessionCommitsRefetch(event.session_id);
    // Invalidate cumulative diff cache when commits are reset
    invalidateCumulativeDiffCache(resolveEnvKey(store, event.session_id));
  },

  branch_switched: (store, event) => {
    if (isDebug()) debug("branch_switched", { sessionId: event.session_id });
    // Stale-while-revalidate (see commits_reset above): refetch with the new
    // base commit but keep the old list visible until the new one arrives.
    store.getState().bumpSessionCommitsRefetch(event.session_id);
    // Invalidate cumulative diff cache when branch switches
    invalidateCumulativeDiffCache(resolveEnvKey(store, event.session_id));
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
