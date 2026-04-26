import type { StoreApi } from "zustand";
import type { AppState } from "@/lib/state/store";
import type { WsHandlers } from "@/lib/ws/handlers/types";
import type {
  GitEventPayload,
  GitStatusUpdateEvent,
  GitCommitCreatedEvent,
  GitCommitsResetEvent,
  GitBranchSwitchedEvent,
} from "@/lib/types/git-events";
import { invalidateCumulativeDiffCache } from "@/hooks/domains/session/use-cumulative-diff";

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

const gitEventHandlers: GitEventHandlers = {
  status_update: (store, event) => {
    store.getState().setGitStatus(event.session_id, {
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
    });
    // Invalidate cumulative diff cache when files change
    invalidateCumulativeDiffCache(resolveEnvKey(store, event.session_id));
  },

  commit_created: (store, event) => {
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
    // Clear commits to trigger refetch in useSessionCommits hook
    store.getState().clearSessionCommits(event.session_id);
    // Invalidate cumulative diff cache when commits are reset
    invalidateCumulativeDiffCache(resolveEnvKey(store, event.session_id));
  },

  branch_switched: (store, event) => {
    // Clear commits to trigger refetch with new base commit
    store.getState().clearSessionCommits(event.session_id);
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
