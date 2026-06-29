import { describe, expect, it } from "vitest";
import {
  markInactiveChangesIncreases,
  migrateEnvironmentKeys,
  selectChangesCountByEnvironment,
  selectChangesMarkerByEnvironment,
  shouldClearPendingChangesFocus,
} from "./changes-panel-focus";
import type { GitStatusEntry, SessionCommit } from "@/lib/state/slices/session-runtime/types";

const TEST_TIMESTAMP = "2026-06-29T00:00:00Z";

function gitStatus(files: string[], timestamp = TEST_TIMESTAMP): GitStatusEntry {
  return {
    branch: "main",
    remote_branch: null,
    modified: [],
    added: [],
    deleted: [],
    untracked: files,
    renamed: [],
    ahead: 0,
    behind: 0,
    files: Object.fromEntries(
      files.map((path) => [path, { path, status: "untracked", staged: false }]),
    ),
    timestamp,
  };
}

function commit(sha: string): SessionCommit {
  return {
    id: `commit-${sha}`,
    session_id: "sess-1",
    commit_sha: sha,
    parent_sha: `parent-${sha}`,
    author_name: "Test User",
    author_email: "test@example.com",
    commit_message: `Commit ${sha}`,
    committed_at: TEST_TIMESTAMP,
    files_changed: 1,
    insertions: 1,
    deletions: 0,
    created_at: TEST_TIMESTAMP,
  };
}

describe("selectChangesCountByEnvironment", () => {
  it("counts git files and commits per environment", () => {
    const state = {
      gitStatus: {
        byEnvironmentId: {},
        byEnvironmentRepo: {
          envA: {
            repo1: gitStatus(["one.ts", "two.ts"]),
          },
          envB: {
            repo1: gitStatus([]),
            repo2: gitStatus(["three.ts"]),
          },
        },
      },
      sessionCommits: {
        loading: {},
        refetchTrigger: {},
        byEnvironmentId: {
          envA: [commit("commit-1"), commit("commit-2")],
          envC: [commit("commit-3")],
        },
      },
    };

    expect(selectChangesCountByEnvironment(state)).toEqual({
      envA: 4,
      envB: 1,
      envC: 1,
    });
  });
});

describe("selectChangesMarkerByEnvironment", () => {
  it("changes fingerprint for meaningful git updates with the same count", () => {
    const baseState = {
      gitStatus: {
        byEnvironmentId: {},
        byEnvironmentRepo: {
          envA: {
            repo1: gitStatus(["one.ts"], "2026-06-29T00:00:00Z"),
          },
        },
      },
      sessionCommits: {
        loading: {},
        refetchTrigger: {},
        byEnvironmentId: {},
      },
    };
    const nextState = {
      ...baseState,
      gitStatus: {
        byEnvironmentId: {},
        byEnvironmentRepo: {
          envA: {
            repo1: gitStatus(["one.ts"], "2026-06-29T00:01:00Z"),
          },
        },
      },
    };

    const baseMarker = selectChangesMarkerByEnvironment(baseState).envA;
    const nextMarker = selectChangesMarkerByEnvironment(nextState).envA;

    expect(nextMarker.count).toBe(baseMarker.count);
    expect(nextMarker.fingerprint).not.toBe(baseMarker.fingerprint);
  });
});

describe("markInactiveChangesIncreases", () => {
  it("baselines first observations and queues only inactive environment increases", () => {
    const previousMarkers = {};
    const pendingEnvKeys = new Set<string>();

    markInactiveChangesIncreases({
      markersByEnv: {
        envA: { count: 1, fingerprint: "a1" },
        envB: { count: 0, fingerprint: "b0" },
      },
      activeEnvKey: "envA",
      previousActiveEnvKey: "envA",
      previousMarkers,
      pendingEnvKeys,
    });

    expect(pendingEnvKeys.size).toBe(0);

    markInactiveChangesIncreases({
      markersByEnv: {
        envA: { count: 2, fingerprint: "a2" },
        envB: { count: 1, fingerprint: "b1" },
      },
      activeEnvKey: "envA",
      previousActiveEnvKey: "envA",
      previousMarkers,
      pendingEnvKeys,
    });

    expect([...pendingEnvKeys]).toEqual(["envB"]);
    expect(previousMarkers).toEqual({
      envA: { count: 2, fingerprint: "a2" },
      envB: { count: 1, fingerprint: "b1" },
    });
  });

  it("queues a same-batch update for the previously inactive environment", () => {
    const previousMarkers = {
      envB: { count: 0, fingerprint: "b0" },
    };
    const pendingEnvKeys = new Set<string>();

    markInactiveChangesIncreases({
      markersByEnv: {
        envB: { count: 1, fingerprint: "b1" },
      },
      activeEnvKey: "envB",
      previousActiveEnvKey: "envA",
      previousMarkers,
      pendingEnvKeys,
    });

    expect([...pendingEnvKeys]).toEqual(["envB"]);
  });

  it("queues count-neutral meaningful updates for inactive environments", () => {
    const previousMarkers = {
      envB: { count: 1, fingerprint: "b1" },
    };
    const pendingEnvKeys = new Set<string>();

    markInactiveChangesIncreases({
      markersByEnv: {
        envB: { count: 1, fingerprint: "b2" },
      },
      activeEnvKey: "envA",
      previousActiveEnvKey: "envA",
      previousMarkers,
      pendingEnvKeys,
    });

    expect([...pendingEnvKeys]).toEqual(["envB"]);
  });
});

describe("migrateEnvironmentKeys", () => {
  it("migrates pending and previous fallback session keys to environment keys", () => {
    const previousMarkers = {
      "session-B": { count: 1, fingerprint: "b1" },
    };
    const pendingEnvKeys = new Set(["session-B"]);

    migrateEnvironmentKeys({
      environmentIdBySessionId: { "session-B": "envB" },
      previousMarkers,
      pendingEnvKeys,
    });

    expect([...pendingEnvKeys]).toEqual(["envB"]);
    expect(previousMarkers).toEqual({
      envB: { count: 1, fingerprint: "b1" },
    });
  });
});

describe("shouldClearPendingChangesFocus", () => {
  it("keeps retryable activation results pending", () => {
    expect(shouldClearPendingChangesFocus("activated")).toBe(true);
    expect(shouldClearPendingChangesFocus("no-panel")).toBe(true);
    expect(shouldClearPendingChangesFocus("blocked-agent-group")).toBe(false);
    expect(shouldClearPendingChangesFocus("no-api")).toBe(false);
  });
});
