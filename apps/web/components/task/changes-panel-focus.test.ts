import { describe, expect, it } from "vitest";
import {
  applyChangesPanelAutoFocusState,
  markInactiveChangesIncreases,
  migrateEnvironmentKeys,
  selectChangesMarkerByEnvironment,
  shouldClearPendingChangesFocus,
} from "./changes-panel-focus";
import type { GitStatusEntry } from "@/lib/state/slices/session-runtime/types";

const TEST_TIMESTAMP = "2026-06-29T00:00:00Z";
const UPDATED_FINGERPRINT = "repo:path:updated";

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

function signal(count: number, fingerprint: string): string {
  return `${count}\u0000${fingerprint}`;
}

describe("applyChangesPanelAutoFocusState", () => {
  it("migrates keys before queuing, defers during restore, and clears after activation", () => {
    const previousMarkers = {};
    const pendingEnvKeys = new Set<string>();
    let previousActiveEnvKey: string | null = "envA";
    let activateCalls = 0;

    previousActiveEnvKey = applyChangesPanelAutoFocusState({
      signalsByEnv: {
        "session-B": signal(1, "repo:path:initial"),
      },
      activeEnvKey: "envA",
      previousActiveEnvKey,
      environmentIdBySessionId: {},
      previousMarkers,
      pendingEnvKeys,
      isRestoringLayout: false,
      activate: () => {
        activateCalls += 1;
        return "activated";
      },
    });

    expect(pendingEnvKeys.size).toBe(0);

    previousActiveEnvKey = applyChangesPanelAutoFocusState({
      signalsByEnv: {
        envB: signal(12, UPDATED_FINGERPRINT),
      },
      activeEnvKey: "envA",
      previousActiveEnvKey,
      environmentIdBySessionId: { "session-B": "envB" },
      previousMarkers,
      pendingEnvKeys,
      isRestoringLayout: false,
      activate: () => {
        activateCalls += 1;
        return "activated";
      },
    });

    expect([...pendingEnvKeys]).toEqual(["envB"]);
    expect(previousMarkers).toEqual({
      envB: { count: 12, fingerprint: UPDATED_FINGERPRINT },
    });

    previousActiveEnvKey = applyChangesPanelAutoFocusState({
      signalsByEnv: {
        envB: signal(12, UPDATED_FINGERPRINT),
      },
      activeEnvKey: "envB",
      previousActiveEnvKey,
      environmentIdBySessionId: { "session-B": "envB" },
      previousMarkers,
      pendingEnvKeys,
      isRestoringLayout: true,
      activate: () => {
        activateCalls += 1;
        return "activated";
      },
    });

    expect(activateCalls).toBe(0);
    expect([...pendingEnvKeys]).toEqual(["envB"]);

    previousActiveEnvKey = applyChangesPanelAutoFocusState({
      signalsByEnv: {
        envB: signal(12, UPDATED_FINGERPRINT),
      },
      activeEnvKey: "envB",
      previousActiveEnvKey,
      environmentIdBySessionId: { "session-B": "envB" },
      previousMarkers,
      pendingEnvKeys,
      isRestoringLayout: false,
      activate: () => {
        activateCalls += 1;
        return "activated";
      },
    });

    expect(previousActiveEnvKey).toBe("envB");
    expect(activateCalls).toBe(1);
    expect(pendingEnvKeys.size).toBe(0);
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
