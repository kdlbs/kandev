import { describe, it, expect, vi, beforeEach } from "vitest";
import { create, type StoreApi } from "zustand";
import { immer } from "zustand/middleware/immer";
import { QueryClient } from "@tanstack/react-query";
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
import { registerSessionRuntimeBridge } from "@/lib/query/bridge/session-runtime";

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

function repoStatusUpdateEvent(
  timestamp: string,
  repositoryName: string,
  modifiedPath: string,
): GitStatusUpdateEvent {
  return {
    type: "status_update",
    session_id: SESSION,
    timestamp,
    status: {
      branch: "main",
      remote_branch: null,
      modified: [modifiedPath],
      added: [],
      deleted: [],
      untracked: [],
      renamed: [],
      ahead: 0,
      behind: 0,
      repository_name: repositoryName,
      files: {
        [modifiedPath]: {
          path: modifiedPath,
          status: "modified",
          staged: false,
          additions: 1,
          deletions: 0,
        },
      },
    },
  };
}

function seedSessionCommits(store: StoreApi<AppState>) {
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
}

describe("git-status WS handler — stale-while-revalidate", () => {
  let store: StoreApi<AppState>;

  beforeEach(() => {
    invalidateCumulativeDiffCacheMock.mockClear();
    store = makeStore();
    seedSessionCommits(store);
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
});

// ---------------------------------------------------------------------------
// status_update de-dupe contract (TQ bridge — bridge/session-runtime.ts)
//
// Post-migration the git-status DEDUPE responsibility lives in the bridge, not
// the Zustand handler. The backend re-emits IDENTICAL git data with a FRESH
// timestamp on focus/startup/poll — those duplicates must NOT invalidate the
// cumulative-diff cache (that would flicker the Changes panel). Only a genuine
// CONTENT change invalidates. These tests encode that contract directly against
// the bridge.
// ---------------------------------------------------------------------------

type Handler = (msg: { payload: Record<string, unknown>; timestamp?: string }) => void;

function makeFakeWs() {
  const listeners = new Map<string, Set<Handler>>();
  return {
    on: vi.fn((type: string, handler: Handler) => {
      const set = listeners.get(type) ?? new Set<Handler>();
      set.add(handler);
      listeners.set(type, set);
      return () => listeners.get(type)?.delete(handler);
    }),
    emitGit(event: GitStatusUpdateEvent) {
      const msg = gitEvent(event);
      listeners.get("session.git.event")?.forEach((h) =>
        h({
          payload: msg.payload as unknown as Record<string, unknown>,
          timestamp: msg.timestamp,
        }),
      );
    },
  };
}

describe("git-status bridge — cumulative-diff de-dupe contract", () => {
  beforeEach(() => {
    invalidateCumulativeDiffCacheMock.mockClear();
  });

  function setupBridge() {
    const ws = makeFakeWs();
    const qc = new QueryClient({ defaultOptions: { queries: { retry: false, gcTime: 0 } } });
    // identity env resolver: envKey === sessionId for the test.
    const cleanup = registerSessionRuntimeBridge(
      ws as unknown as Parameters<typeof registerSessionRuntimeBridge>[0],
      qc,
      (sid) => sid,
    );
    return { ws, qc, cleanup };
  }

  it("invalidates on the FIRST status_update (new content)", () => {
    const { ws, cleanup } = setupBridge();
    ws.emitGit(statusUpdateEvent(STATUS_TIME_1));
    expect(invalidateCumulativeDiffCacheMock).toHaveBeenCalledTimes(1);
    cleanup();
  });

  it("does NOT invalidate on a DUPLICATE status_update with a fresh timestamp", () => {
    const { ws, cleanup } = setupBridge();
    // Same content, different timestamp — the focus/startup/poll re-emit case.
    ws.emitGit(statusUpdateEvent(STATUS_TIME_1));
    ws.emitGit(statusUpdateEvent(STATUS_TIME_2));
    // Only the first (genuine new content) invalidated. The duplicate is a
    // no-op for the cumulative-diff cache.
    expect(invalidateCumulativeDiffCacheMock).toHaveBeenCalledTimes(1);
    cleanup();
  });

  it("invalidates again when the file CONTENT genuinely changes", () => {
    const { ws, cleanup } = setupBridge();
    ws.emitGit(statusUpdateEvent(STATUS_TIME_1, "-old\n+new"));
    ws.emitGit(statusUpdateEvent(STATUS_TIME_2, "-old\n+different"));
    expect(invalidateCumulativeDiffCacheMock).toHaveBeenCalledTimes(2);
    cleanup();
  });

  it("does NOT invalidate on DUPLICATE sibling-repo snapshots with fresh timestamps", () => {
    const { ws, cleanup } = setupBridge();
    // Two distinct repos: each first emit is genuine new content → invalidates.
    ws.emitGit(repoStatusUpdateEvent(STATUS_TIME_1, "frontend", "frontend.tsx"));
    ws.emitGit(repoStatusUpdateEvent(STATUS_TIME_1, "backend", "backend.go"));
    expect(invalidateCumulativeDiffCacheMock).toHaveBeenCalledTimes(2);

    // Re-emit BOTH repos verbatim with fresh timestamps — pure focus/poll
    // duplicates. Neither may invalidate.
    invalidateCumulativeDiffCacheMock.mockClear();
    ws.emitGit(repoStatusUpdateEvent(STATUS_TIME_2, "frontend", "frontend.tsx"));
    ws.emitGit(repoStatusUpdateEvent(STATUS_TIME_2, "backend", "backend.go"));
    expect(invalidateCumulativeDiffCacheMock).not.toHaveBeenCalled();
    cleanup();
  });
});
