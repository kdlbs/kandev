import { describe, it, expect, vi } from "vitest";
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
const ORDERED_STATUS_TIME_1 = "2026-09-30T10:00:01Z";
const ORDERED_STATUS_TIME_2 = "2026-09-30T10:00:02Z";
const ORDERED_STATUS_TIME_3 = "2026-09-30T10:00:03Z";
const ENVIRONMENT_A = "environment-a";
const ENVIRONMENT_B = "environment-b";
const MISSING_HANDLER_MESSAGE = "session.git.event handler is missing";
const ACCEPTED_DIFF = "accepted diff";
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

function freshStore() {
  invalidateCumulativeDiffCacheMock.mockClear();
  const store = makeStore();
  seedSessionCommits(store);
  return store;
}

function statusUpdateEvent(
  timestamp: string,
  diff = "-old\n+new",
  sessionId = SESSION,
  taskEnvironmentId = sessionId,
): GitStatusUpdateEvent {
  return {
    type: "status_update",
    session_id: sessionId,
    task_environment_id: taskEnvironmentId,
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
      remote_ahead: 0,
      remote_behind: 0,
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
  taskEnvironmentId = SESSION,
): GitStatusUpdateEvent {
  return {
    type: "status_update",
    session_id: SESSION,
    task_environment_id: taskEnvironmentId,
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
      remote_ahead: 0,
      remote_behind: 0,
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

describe("git-status WS handler — commit events", () => {
  it("commits_reset bumps refetchTrigger and keeps existing commits visible", () => {
    const store = freshStore();
    const handler = gitStatusHandler(store);

    handler(
      gitEvent({
        type: "commits_reset",
        session_id: SESSION,
        timestamp: "2026-05-28T00:00:01Z",
        reset: {
          previous_head: "old-head",
          current_head: "new-head",
          deleted_count: 1,
          repository_name: "repo-a",
        },
      }),
    );

    const state = store.getState();
    // Trigger bumped — useSessionCommits will refetch.
    expect(state.sessionCommits.refetchTrigger[SESSION]).toBe(1);
    expect(state.gitCheckoutGeneration.byEnvironmentId[SESSION]?.["repo-a"]).toBe(1);
    // Existing commits remain — this is the whole point. Clearing would make
    // the Changes panel briefly render its empty state until the refetch
    // resolved.
    expect(state.sessionCommits.byEnvironmentId[SESSION]).toHaveLength(1);
    expect(state.sessionCommits.byEnvironmentId[SESSION][0].commit_sha).toBe("old");
  });

  it("branch_switched bumps refetchTrigger and keeps existing commits visible", () => {
    const store = freshStore();
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
          repository_name: "repo-b",
        },
      }),
    );

    const state = store.getState();
    expect(state.sessionCommits.refetchTrigger[SESSION]).toBe(1);
    expect(state.gitCheckoutGeneration.byEnvironmentId[SESSION]?.["repo-b"]).toBe(1);
    expect(state.sessionCommits.byEnvironmentId[SESSION]).toHaveLength(1);
  });
});

describe("git-status WS handler — status ordering", () => {
  it("accepts root enrichment after a newer child snapshot without accepting stale root data", () => {
    const store = freshStore();
    const handler = gitStatusHandler(store);
    const root = statusUpdateEvent(ORDERED_STATUS_TIME_1, "");
    root.status.detail_state = "pending";
    root.status.tracker_id = "root-tracker";
    root.status.snapshot_revision = 3;
    handler(gitEvent(root));

    const child = statusUpdateEvent(ORDERED_STATUS_TIME_3, "child patch");
    child.status.repository_name = "vendor/child";
    child.status.tracker_id = "child-tracker";
    child.status.snapshot_revision = 4;
    handler(gitEvent(child));

    const ready = statusUpdateEvent(ORDERED_STATUS_TIME_2, ACCEPTED_DIFF);
    ready.status.detail_state = "ready";
    ready.status.tracker_id = "root-tracker";
    ready.status.snapshot_revision = 4;
    handler(gitEvent(ready));

    expect(store.getState().gitStatus.byEnvironmentRepo[SESSION][""].files["a.ts"].diff).toBe(
      ACCEPTED_DIFF,
    );
    expect(
      store.getState().gitStatus.byEnvironmentRepo[SESSION]["vendor/child"].files["a.ts"].diff,
    ).toBe("child patch");
    handler(gitEvent(root));
    expect(store.getState().gitStatus.byEnvironmentRepo[SESSION][""].detail_state).toBe("ready");
    expect(store.getState().gitStatus.byEnvironmentRepo[SESSION][""].files["a.ts"].diff).toBe(
      ACCEPTED_DIFF,
    );
  });

  it("stores the status-level submodule marker", () => {
    const store = freshStore();
    const handler = gitStatusHandler(store);
    const event = statusUpdateEvent(STATUS_TIME_1);
    event.status.is_submodule = true;

    handler(gitEvent(event));

    expect(store.getState().gitStatus.byEnvironmentId[SESSION].is_submodule).toBe(true);
  });

  it("keeps a newer unavailable detail quality when an older ready response arrives", () => {
    const store = freshStore();
    const handler = gitStatusHandler(store);
    const unavailable = statusUpdateEvent(ORDERED_STATUS_TIME_2);
    unavailable.status.status_state = "unavailable";
    unavailable.status.files_complete = false;
    unavailable.status.error_code = "details_unavailable";
    unavailable.status.tracker_id = "tracker-a";
    unavailable.status.tracker_epoch = 1;
    unavailable.status.snapshot_revision = 7;
    handler(gitEvent(unavailable));

    const olderReady = statusUpdateEvent(ORDERED_STATUS_TIME_1);
    olderReady.status.tracker_id = "tracker-a";
    olderReady.status.tracker_epoch = 1;
    olderReady.status.snapshot_revision = 6;
    handler(gitEvent(olderReady));

    expect(store.getState().gitStatus.byEnvironmentId[SESSION]).toBeUndefined();
    expect(store.getState().gitStatus.refreshByEnvironmentRepo?.[SESSION]?.[""]).toMatchObject({
      state: "unavailable",
      error_code: "details_unavailable",
      tracker_id: "tracker-a",
      snapshot_revision: 7,
    });
  });

  it("accepts a newer capture from a replacement tracker with a lower local revision", () => {
    const store = freshStore();
    const handler = gitStatusHandler(store);
    const first = statusUpdateEvent(ORDERED_STATUS_TIME_2);
    first.status.tracker_id = "agentctl-a/tracker-1";
    first.status.tracker_epoch = 44;
    first.status.snapshot_revision = 90;
    handler(gitEvent(first));

    const replacement = statusUpdateEvent(ORDERED_STATUS_TIME_3);
    replacement.status.tracker_id = "agentctl-b/tracker-1";
    replacement.status.tracker_epoch = 1;
    replacement.status.snapshot_revision = 1;
    replacement.status.modified = ["replacement.ts"];
    replacement.status.files = {
      "replacement.ts": {
        path: "replacement.ts",
        status: "modified",
        staged: false,
        additions: 1,
      },
    };
    handler(gitEvent(replacement));

    expect(store.getState().gitStatus.byEnvironmentId[SESSION]?.files).toHaveProperty(
      "replacement.ts",
    );
    expect(store.getState().gitStatus.byEnvironmentId[SESSION]?.tracker_id).toBe(
      "agentctl-b/tracker-1",
    );
  });
});

describe("git-status WS handler — environment isolation", () => {
  it("keeps ordering and refresh quality isolated between environments", () => {
    const store = freshStore();
    const handler = gitStatusHandler(store);
    const unavailable = statusUpdateEvent(ORDERED_STATUS_TIME_2);
    unavailable.task_environment_id = ENVIRONMENT_A;
    unavailable.status.status_state = "unavailable";
    unavailable.status.files_complete = false;
    unavailable.status.error_code = "details_unavailable";
    unavailable.status.tracker_id = "tracker-a";
    unavailable.status.snapshot_revision = 7;
    handler(gitEvent(unavailable));

    const otherEnvironment = statusUpdateEvent(ORDERED_STATUS_TIME_3);
    otherEnvironment.task_environment_id = ENVIRONMENT_B;
    otherEnvironment.status.tracker_id = "tracker-b";
    otherEnvironment.status.snapshot_revision = 1;
    handler(gitEvent(otherEnvironment));

    const stale = statusUpdateEvent(ORDERED_STATUS_TIME_1);
    stale.task_environment_id = ENVIRONMENT_A;
    stale.status.tracker_id = "tracker-a";
    stale.status.snapshot_revision = 6;
    handler(gitEvent(stale));

    expect(store.getState().gitStatus.byEnvironmentId[ENVIRONMENT_B]?.tracker_id).toBe("tracker-b");
    expect(
      store.getState().gitStatus.refreshByEnvironmentRepo?.[ENVIRONMENT_A]?.[""],
    ).toMatchObject({
      state: "unavailable",
      error_code: "details_unavailable",
      snapshot_revision: 7,
    });
    expect(store.getState().gitStatus.byEnvironmentId[ENVIRONMENT_A]).toBeUndefined();
  });

  it.each(["unavailable", "loading"] as const)(
    "does not apply an older %s frame over a newer ready snapshot",
    (statusState) => {
      const store = freshStore();
      const handler = gitStatusHandler(store);
      const ready = statusUpdateEvent(ORDERED_STATUS_TIME_2);
      ready.status.tracker_id = "tracker-a";
      ready.status.tracker_epoch = 1;
      ready.status.snapshot_revision = 8;
      handler(gitEvent(ready));

      const older = statusUpdateEvent(ORDERED_STATUS_TIME_1);
      older.status.status_state = statusState;
      older.status.files_complete = false;
      older.status.tracker_id = "tracker-a";
      older.status.tracker_epoch = 1;
      older.status.snapshot_revision = 7;
      handler(gitEvent(older));

      expect(store.getState().gitStatus.byEnvironmentId[SESSION]?.files).toHaveProperty("a.ts");
      expect(store.getState().gitStatus.refreshByEnvironmentId?.[SESSION]).toBeUndefined();
    },
  );
});

describe("git-status WS handler — commit and upstream evidence", () => {
  it("retains the commit and upstream evidence from a status event", () => {
    const store = freshStore();
    const handler = gitStatusHandler(store);
    const event = statusUpdateEvent(STATUS_TIME_1);
    event.status = {
      ...event.status,
      head_commit: "head-sha",
      base_commit: "base-sha",
      remote_head_commit: "remote-head-sha",
      remote_ahead: 2,
      remote_behind: 3,
    } as typeof event.status;

    handler(gitEvent(event));

    expect(store.getState().gitStatus.byEnvironmentId[SESSION]).toMatchObject({
      head_commit: "head-sha",
      base_commit: "base-sha",
      remote_head_commit: "remote-head-sha",
      remote_ahead: 2,
      remote_behind: 3,
    });
  });
});

describe("git-status WS handler — status updates", () => {
  it("does not invalidate cumulative diff for duplicate status snapshots", () => {
    const store = freshStore();
    const handler = gitStatusHandler(store);

    handler(gitEvent(statusUpdateEvent(STATUS_TIME_1)));
    handler(gitEvent(statusUpdateEvent(STATUS_TIME_2)));

    expect(invalidateCumulativeDiffCacheMock).toHaveBeenCalledTimes(1);
    expect(store.getState().gitStatus.byEnvironmentId[SESSION].timestamp).toBe(STATUS_TIME_2);
  });

  it("invalidates cumulative diff when status diff content changes", () => {
    const store = freshStore();
    const handler = gitStatusHandler(store);

    handler(gitEvent(statusUpdateEvent(STATUS_TIME_1, "-old\n+new")));
    handler(gitEvent(statusUpdateEvent(STATUS_TIME_2, "-old\n+newer")));

    expect(invalidateCumulativeDiffCacheMock).toHaveBeenCalledTimes(2);
  });

  it("does not invalidate or overwrite env status for duplicate sibling-repo snapshots", () => {
    const store = freshStore();
    const handler = gitStatusHandler(store);

    handler(gitEvent(repoStatusUpdateEvent(STATUS_TIME_1, "frontend", "frontend.tsx")));
    handler(gitEvent(repoStatusUpdateEvent(STATUS_TIME_2, "backend", "backend.go")));
    invalidateCumulativeDiffCacheMock.mockClear();

    handler(gitEvent(repoStatusUpdateEvent("2026-05-28T00:00:03Z", "backend", "backend.go")));

    expect(store.getState().gitStatus.byEnvironmentId[SESSION]).toMatchObject({
      modified: ["backend.go"],
      timestamp: "2026-05-28T00:00:03Z",
    });
    expect(invalidateCumulativeDiffCacheMock).not.toHaveBeenCalled();
  });
});

describe("git-status WS handler — shared environment ordering", () => {
  it("does not overwrite newer environment status with an older sibling event", () => {
    const store = freshStore();
    const handler = gitStatusHandler(store);
    store.getState().registerSessionEnvironment(SESSION, "env-1");
    store.getState().registerSessionEnvironment("sess-2", "env-1");

    handler(gitEvent(statusUpdateEvent(STATUS_TIME_2, "-old\n+new", SESSION, "env-1")));
    const olderCleanEvent = statusUpdateEvent(STATUS_TIME_1, "", "sess-2", "env-1");
    olderCleanEvent.status = {
      ...olderCleanEvent.status,
      modified: [],
      files: {},
    };
    handler(gitEvent(olderCleanEvent));

    expect(store.getState().gitStatus.byEnvironmentId["env-1"].modified).toEqual(["a.ts"]);
    expect(invalidateCumulativeDiffCacheMock).toHaveBeenCalledTimes(1);
  });

  it("does not let an undated sibling event replace dated environment state", () => {
    const store = freshStore();
    const handler = gitStatusHandler(store);
    store.getState().registerSessionEnvironment(SESSION, "env-1");
    store.getState().registerSessionEnvironment("sess-2", "env-1");

    handler(gitEvent(statusUpdateEvent(STATUS_TIME_1, "-old\n+new", SESSION, "env-1")));
    invalidateCumulativeDiffCacheMock.mockClear();
    const undatedCleanEvent = statusUpdateEvent("not-a-timestamp", "", "sess-2", "env-1");
    undatedCleanEvent.status = {
      ...undatedCleanEvent.status,
      modified: [],
      files: {},
    };
    handler(gitEvent(undatedCleanEvent));

    expect(store.getState().gitStatus.byEnvironmentId["env-1"].modified).toEqual(["a.ts"]);
    expect(invalidateCumulativeDiffCacheMock).not.toHaveBeenCalled();
  });

  it("accepts equal-time content changes", () => {
    const store = freshStore();
    const handler = gitStatusHandler(store);

    handler(gitEvent(statusUpdateEvent(STATUS_TIME_1, "-old\n+new", SESSION, "env-1")));
    handler(gitEvent(statusUpdateEvent(STATUS_TIME_1, "-old\n+newer", SESSION, "env-1")));

    expect(store.getState().gitStatus.byEnvironmentId["env-1"].files["a.ts"].diff).toBe(
      "-old\n+newer",
    );
    expect(invalidateCumulativeDiffCacheMock).toHaveBeenCalledTimes(2);
  });
});

describe("git-status WS handler — delivered environment identity", () => {
  it("routes status directly to the payload environment instead of the session mapping", () => {
    const store = freshStore();
    const handler = gitStatusHandler(store);
    store.getState().registerSessionEnvironment(SESSION, "mapped-env");
    const event = statusUpdateEvent(STATUS_TIME_1) as GitStatusUpdateEvent & {
      task_environment_id: string;
    };
    event.task_environment_id = "payload-env";

    handler(gitEvent(event));

    expect(store.getState().gitStatus.byEnvironmentRepo["payload-env"][""]).toBeDefined();
    expect(store.getState().gitStatus.byEnvironmentRepo["mapped-env"]).toBeUndefined();
  });

  it("ignores a status update without an environment identity", () => {
    const store = freshStore();
    const handler = gitStatusHandler(store);
    store.getState().registerSessionEnvironment(SESSION, "mapped-env");
    const event = { ...statusUpdateEvent(STATUS_TIME_1) } as Omit<
      GitStatusUpdateEvent,
      "task_environment_id"
    > & {
      task_environment_id?: string;
    };
    delete event.task_environment_id;

    handler(gitEvent(event as unknown as GitStatusUpdateEvent));

    expect(store.getState().gitStatus.byEnvironmentId).toEqual({});
    expect(store.getState().gitStatus.byEnvironmentRepo).toEqual({});
  });

  it("does not accept a sparse status update with no complete file map", () => {
    const store = freshStore();
    const handler = gitStatusHandler(store);
    const event = statusUpdateEvent(STATUS_TIME_1) as GitStatusUpdateEvent & {
      task_environment_id: string;
    };
    event.task_environment_id = "payload-env";
    delete (event.status as typeof event.status & { files?: unknown }).files;

    handler(gitEvent(event));

    expect(store.getState().gitStatus.byEnvironmentRepo["payload-env"]).toBeUndefined();
    expect(
      store.getState().gitStatus.refreshByEnvironmentRepo?.["payload-env"]?.[""],
    ).toMatchObject({ state: "pending" });
  });

  it("preserves snapshot quality through the status event adapter", () => {
    const store = freshStore();
    const handler = gitStatusHandler(store);
    const event = statusUpdateEvent(STATUS_TIME_1) as GitStatusUpdateEvent & {
      task_environment_id: string;
    };
    event.task_environment_id = "payload-env";
    event.status.status_state = "ready";
    event.status.files_complete = true;
    event.status.detail_state = "pending";
    event.status.error_code = "";
    event.status.files!["a.ts"].diff_state = "pending";

    handler(gitEvent(event));

    const status = store.getState().gitStatus.byEnvironmentRepo["payload-env"][""];
    expect(status).toMatchObject({
      status_state: "ready",
      files_complete: true,
      detail_state: "pending",
    });
    expect(status.files["a.ts"].diff_state).toBe("pending");
  });

  it("preserves accepted files while marking incomplete membership as pending", () => {
    const store = freshStore();
    const handler = gitStatusHandler(store);
    handler(gitEvent(statusUpdateEvent(STATUS_TIME_1, ACCEPTED_DIFF, SESSION, "env-1")));

    const incomplete = statusUpdateEvent(STATUS_TIME_2, "", SESSION, "env-1");
    incomplete.status.status_state = "loading";
    incomplete.status.files_complete = false;
    incomplete.status.files = {};
    handler(gitEvent(incomplete));

    expect(store.getState().gitStatus.byEnvironmentRepo["env-1"][""].files["a.ts"].diff).toBe(
      ACCEPTED_DIFF,
    );
    expect(store.getState().gitStatus.refreshByEnvironmentRepo?.["env-1"]?.[""]).toMatchObject({
      state: "pending",
    });
  });

  it("records unavailable status without replacing the last accepted snapshot", () => {
    const store = freshStore();
    const handler = gitStatusHandler(store);
    handler(gitEvent(statusUpdateEvent(STATUS_TIME_1, ACCEPTED_DIFF, SESSION, "env-1")));

    const unavailable = statusUpdateEvent(STATUS_TIME_2, "", SESSION, "env-1");
    unavailable.status.status_state = "unavailable";
    unavailable.status.files_complete = false;
    unavailable.status.error_code = "status_timeout";
    unavailable.status.files = {};
    handler(gitEvent(unavailable));

    expect(store.getState().gitStatus.byEnvironmentRepo["env-1"][""].files["a.ts"].diff).toBe(
      ACCEPTED_DIFF,
    );
    expect(store.getState().gitStatus.refreshByEnvironmentRepo?.["env-1"]?.[""]).toMatchObject({
      state: "unavailable",
      error_code: "status_timeout",
    });
  });
});
