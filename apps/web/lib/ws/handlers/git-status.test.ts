import { describe, it, expect, vi, beforeEach } from "vitest";
import { create, type StoreApi } from "zustand";
import { immer } from "zustand/middleware/immer";
import { createSessionRuntimeSlice } from "@/lib/state/slices/session-runtime/session-runtime-slice";
import type { SessionRuntimeSlice } from "@/lib/state/slices/session-runtime/types";
import type { AppState } from "@/lib/state/store";
import type {
  GitCommitsResetEvent,
  GitBranchSwitchedEvent,
  GitStatusUpdateEvent,
} from "@/lib/types/git-events";
import { invalidateCumulativeDiffCache } from "@/hooks/domains/session/use-cumulative-diff";
import { registerGitStatusHandlers } from "./git-status";

// invalidateCumulativeDiffCache lives in a hook module that pulls React in via
// its imports. Stub it out so this test can run as a pure unit test against
// the slice + handler without dragging in React.
vi.mock("@/hooks/domains/session/use-cumulative-diff", () => ({
  invalidateCumulativeDiffCache: vi.fn(),
}));

const SESSION = "sess-1";
const STATUS_TIME_1 = "2026-05-28T00:00:01Z";
const STATUS_TIME_2 = "2026-05-28T00:00:02Z";
const MISSING_HANDLER_MESSAGE = "session.git.event handler is missing";
const invalidateCumulativeDiffCacheMock = vi.mocked(invalidateCumulativeDiffCache);

function makeStore() {
  // The handler only touches session-runtime state and environmentIdBySessionId.
  // We don't need the full AppState — cast through unknown so the handler
  // signature is satisfied without standing up unrelated slices.
  return create<SessionRuntimeSlice>()(
    immer((set, get, store) => createSessionRuntimeSlice(set, get, store)),
  ) as unknown as StoreApi<AppState>;
}

function gitEvent(payload: GitCommitsResetEvent | GitBranchSwitchedEvent | GitStatusUpdateEvent) {
  return {
    id: "msg",
    type: "notification" as const,
    action: "session.git.event" as const,
    timestamp: payload.timestamp,
    payload,
  };
}

function gitStatusHandler(store: StoreApi<AppState>) {
  const handler = registerGitStatusHandlers(store)["session.git.event"];
  if (!handler) throw new Error(MISSING_HANDLER_MESSAGE);
  return handler;
}

function statusUpdateEvent(timestamp: string, diff = "-old\n+new"): GitStatusUpdateEvent {
  return {
    type: "status_update",
    session_id: SESSION,
    timestamp,
    status: {
      branch: "main",
      remote_branch: null,
      modified: ["a.ts"],
      added: [],
      deleted: [],
      untracked: [],
      renamed: [],
      ahead: 0,
      behind: 0,
      files: {
        "a.ts": {
          path: "a.ts",
          status: "modified",
          staged: false,
          additions: 1,
          deletions: 1,
          diff,
        },
      },
    },
  };
}

describe("git-status WS handler — stale-while-revalidate", () => {
  let store: StoreApi<AppState>;

  beforeEach(() => {
    invalidateCumulativeDiffCacheMock.mockClear();
    store = makeStore();
    store.getState().setSessionCommits(SESSION, [
      {
        id: "id",
        session_id: SESSION,
        commit_sha: "old",
        parent_sha: "parent",
        commit_message: "msg",
        author_name: "a",
        author_email: "a@a",
        files_changed: 0,
        insertions: 0,
        deletions: 0,
        committed_at: "2026-05-28T00:00:00Z",
        created_at: "2026-05-28T00:00:00Z",
      },
    ]);
  });

  it("commits_reset bumps refetchTrigger and keeps existing commits visible", () => {
    const handler = gitStatusHandler(store);

    handler(
      gitEvent({
        type: "commits_reset",
        session_id: SESSION,
        timestamp: "2026-05-28T00:00:01Z",
        reset: { previous_head: "old-head", current_head: "new-head", deleted_count: 1 },
      }),
    );

    const state = store.getState();
    // Trigger bumped — useSessionCommits will refetch.
    expect(state.sessionCommits.refetchTrigger[SESSION]).toBe(1);
    // Existing commits remain — this is the whole point. Clearing would make
    // the Changes panel briefly render its empty state until the refetch
    // resolved.
    expect(state.sessionCommits.byEnvironmentId[SESSION]).toHaveLength(1);
    expect(state.sessionCommits.byEnvironmentId[SESSION][0].commit_sha).toBe("old");
  });

  it("branch_switched bumps refetchTrigger and keeps existing commits visible", () => {
    const handler = gitStatusHandler(store);

    handler(
      gitEvent({
        type: "branch_switched",
        session_id: SESSION,
        timestamp: "2026-05-28T00:00:02Z",
        branch_switch: {
          previous_branch: "old",
          current_branch: "new",
          current_head: "head",
          base_commit: "base",
        },
      }),
    );

    const state = store.getState();
    expect(state.sessionCommits.refetchTrigger[SESSION]).toBe(1);
    expect(state.sessionCommits.byEnvironmentId[SESSION]).toHaveLength(1);
  });

  it("does not invalidate cumulative diff for duplicate status snapshots", () => {
    const handler = gitStatusHandler(store);

    handler(gitEvent(statusUpdateEvent(STATUS_TIME_1)));
    handler(gitEvent(statusUpdateEvent(STATUS_TIME_2)));

    expect(invalidateCumulativeDiffCacheMock).toHaveBeenCalledTimes(1);
  });

  it("invalidates cumulative diff when status diff content changes", () => {
    const handler = gitStatusHandler(store);

    handler(gitEvent(statusUpdateEvent(STATUS_TIME_1, "-old\n+new")));
    handler(gitEvent(statusUpdateEvent(STATUS_TIME_2, "-old\n+newer")));

    expect(invalidateCumulativeDiffCacheMock).toHaveBeenCalledTimes(2);
  });
});
