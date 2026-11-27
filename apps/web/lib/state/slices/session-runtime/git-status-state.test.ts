import { describe, expect, it } from "vitest";
import { applyGitStatus } from "./git-status-state";
import type { GitStatusEntry, SessionRuntimeSliceState } from "./types";

function entry(overrides: Partial<GitStatusEntry> = {}): GitStatusEntry {
  return {
    status_state: "ready",
    files_complete: true,
    detail_state: "ready",
    branch: "main",
    remote_branch: null,
    modified: ["src/a.ts"],
    added: [],
    deleted: [],
    untracked: [],
    renamed: [],
    ahead: 0,
    behind: 0,
    files: {
      "src/a.ts": {
        path: "src/a.ts",
        status: "modified",
        staged: false,
        additions: 2,
        deletions: 1,
        diff: "@@ -1 +1 @@\n-old\n+new",
        diff_state: "ready",
      },
    },
    timestamp: null,
    tracker_id: "tracker-a",
    tracker_epoch: 2,
    snapshot_revision: 1,
    ...overrides,
  };
}

function state(): SessionRuntimeSliceState {
  return {
    gitStatus: { byEnvironmentId: {}, byEnvironmentRepo: {} },
  } as unknown as SessionRuntimeSliceState;
}

describe("applyGitStatus", () => {
  it("does not promote prior per-file enrichment into a newer pending snapshot", () => {
    const store = state();
    const initial = entry({ head_commit: "head-a", base_commit: "base-a" });
    initial.files["src/a.ts"].staged_change = {
      status: "modified",
      additions: 1,
      deletions: 0,
      diff: "staged patch",
      diff_state: "ready",
    };
    initial.files["src/a.ts"].unstaged_change = {
      status: "modified",
      additions: 1,
      deletions: 1,
      diff: "unstaged patch",
      diff_state: "ready",
    };
    applyGitStatus(store, "environment", initial);

    const mixed = entry({
      detail_state: "pending",
      snapshot_revision: 2,
      head_commit: "head-b",
      files: {
        "src/a.ts": {
          path: "src/a.ts",
          status: "modified",
          staged: false,
          diff_state: "pending",
          staged_change: {
            status: "modified",
            diff_state: "pending",
          },
          unstaged_change: {
            status: "modified",
            diff_state: "pending",
          },
        },
      },
    });
    applyGitStatus(store, "environment", mixed);

    const file = store.gitStatus.byEnvironmentRepo.environment[""].files["src/a.ts"];
    expect(file.diff_state).toBe("pending");
    expect(file.diff).toBeUndefined();
    expect(file.additions).toBeUndefined();
    expect(file.deletions).toBeUndefined();
    expect(file.staged_change).toEqual({ status: "modified", diff_state: "pending" });
    expect(file.unstaged_change).toEqual({ status: "modified", diff_state: "pending" });
    expect(store.gitStatus.byEnvironmentRepo.environment[""].detail_state).toBe("pending");
  });

  it("rejects a snapshot from an older tracker revision", () => {
    const store = state();
    applyGitStatus(store, "environment", entry());
    const older = entry({
      snapshot_revision: 0,
      files: {},
      modified: [],
    });

    expect(applyGitStatus(store, "environment", older)).toBe(false);
    expect(store.gitStatus.byEnvironmentRepo.environment[""].files["src/a.ts"]).toBeDefined();
  });
});

describe("applyGitStatus source ordering", () => {
  it("uses capture time across tracker lifetimes with colliding local epochs", () => {
    const store = state();
    applyGitStatus(
      store,
      "environment",
      entry({
        tracker_id: "agentctl-a/tracker-a",
        tracker_epoch: 1,
        snapshot_revision: 90,
        timestamp: "2026-09-30T10:00:02.000Z",
      }),
    );
    const siblingOlderCapture = entry({
      tracker_id: "agentctl-b/tracker-a",
      tracker_epoch: 1,
      snapshot_revision: 1,
      timestamp: "2026-09-30T10:00:01.000Z",
      files: {},
      modified: [],
    });
    expect(applyGitStatus(store, "environment", siblingOlderCapture)).toBe(false);

    const replacement = entry({
      tracker_id: "agentctl-c/tracker-a",
      tracker_epoch: 1,
      snapshot_revision: 1,
      timestamp: "2026-09-30T10:00:03.000Z",
      branch: "replacement",
    });
    expect(applyGitStatus(store, "environment", replacement)).toBe(true);
    expect(store.gitStatus.byEnvironmentRepo.environment[""].branch).toBe("replacement");
  });

  it("does not preserve details when file identity changes", () => {
    const store = state();
    applyGitStatus(store, "environment", entry());
    applyGitStatus(
      store,
      "environment",
      entry({
        snapshot_revision: 2,
        detail_state: "pending",
        files: {
          "src/a.ts": {
            path: "src/a.ts",
            status: "deleted",
            staged: false,
            diff_state: "pending",
          },
        },
      }),
    );

    expect(store.gitStatus.byEnvironmentRepo.environment[""].files["src/a.ts"]).toMatchObject({
      status: "deleted",
      diff_state: "pending",
    });
  });
});
