import { describe, expect, it } from "vitest";
import { applyGitStatus } from "./git-status-state";
import { projectGitStatusForDisplay } from "./git-status-display-state";
import type { GitStatusEntry, SessionRuntimeSliceState } from "./types";

type GitStatusDisplayEntry = Parameters<typeof projectGitStatusForDisplay>[1];
const STAGED_ONLY_PATCH = "staged-only patch";
const UNSTAGED_ONLY_PATCH = "unstaged-only patch";

function entry(overrides: Partial<GitStatusEntry> = {}): GitStatusEntry {
  return {
    status_state: "ready",
    files_complete: true,
    detail_state: "ready",
    branch: "main",
    head_commit: "head-a",
    base_commit: "base-a",
    comparison_target: "origin/main",
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
        additions: 3,
        deletions: 2,
        diff: "@@ -1 +1,2 @@\n-old\n+new",
        diff_state: "ready",
      },
    },
    timestamp: null,
    tracker_id: "tracker-a",
    tracker_epoch: 1,
    snapshot_revision: 1,
    ...overrides,
  };
}

function state(): SessionRuntimeSliceState {
  return {
    gitStatus: { byEnvironmentId: {}, byEnvironmentRepo: {} },
    gitStatusDisplay: { byEnvironmentRepo: {} },
    gitCheckoutGeneration: { byEnvironmentId: { environment: { "": 2 } } },
  } as unknown as SessionRuntimeSliceState;
}

function displayEntry(store: SessionRuntimeSliceState): GitStatusDisplayEntry | undefined {
  return (
    store as SessionRuntimeSliceState & {
      gitStatusDisplay: {
        byEnvironmentRepo: Record<string, Record<string, GitStatusDisplayEntry>>;
      };
    }
  ).gitStatusDisplay.byEnvironmentRepo.environment?.[""];
}

describe("Git status display snapshots", () => {
  it("keeps a ready display beside an accepted pending snapshot without enriching raw state", () => {
    const store = state();
    applyGitStatus(store, "environment", entry());

    const pending = entry({
      detail_state: "pending",
      base_commit: "",
      snapshot_revision: 2,
      files: {
        "src/a.ts": {
          path: "src/a.ts",
          status: "modified",
          staged: false,
          diff_state: "pending",
        },
      },
    });
    applyGitStatus(store, "environment", pending);

    const raw = store.gitStatus.byEnvironmentRepo.environment[""];
    expect(raw.files["src/a.ts"]).not.toHaveProperty("diff");
    expect(raw.files["src/a.ts"]).not.toHaveProperty("additions");
    expect(displayEntry(store)?.files["src/a.ts"].flat?.diff).toContain("+new");

    const projected = projectGitStatusForDisplay(raw, displayEntry(store), 2);
    expect(projected.files["src/a.ts"]).toMatchObject({
      diff: "@@ -1 +1,2 @@\n-old\n+new",
      diff_state: "pending",
      additions: 3,
      deletions: 2,
      display_stale: true,
    });
  });
});

describe("Git status display comparison scope", () => {
  it("treats an empty base scope in a ready snapshot as an explicit comparison change", () => {
    const store = state();
    applyGitStatus(store, "environment", entry());

    applyGitStatus(
      store,
      "environment",
      entry({
        base_commit: "",
        snapshot_revision: 2,
        files: {
          "src/a.ts": {
            path: "src/a.ts",
            status: "modified",
            staged: false,
            additions: 4,
            deletions: 1,
            diff: "new comparison patch",
            diff_state: "ready",
          },
        },
      }),
    );

    const current = store.gitStatus.byEnvironmentRepo.environment[""];
    const projected = projectGitStatusForDisplay(current, displayEntry(store), 2);
    expect(projected.files["src/a.ts"]).toMatchObject({
      diff: "new comparison patch",
      additions: 4,
    });
    expect(projected.files["src/a.ts"]).not.toHaveProperty("display_stale");
  });
});

describe("Git status display refresh states", () => {
  it("keeps ready display content while a refresh request is unavailable", () => {
    const store = state();
    applyGitStatus(store, "environment", entry());
    const current = store.gitStatus.byEnvironmentRepo.environment[""];

    const projected = projectGitStatusForDisplay(current, displayEntry(store), 2, "unavailable");

    expect(projected.files["src/a.ts"]).toMatchObject({
      diff: "@@ -1 +1,2 @@\n-old\n+new",
      additions: 3,
      deletions: 2,
      diff_state: "unavailable",
      display_stale: true,
    });
  });

  it("updates ready empty patches and prunes files removed by complete membership", () => {
    const store = state();
    applyGitStatus(store, "environment", entry());

    applyGitStatus(
      store,
      "environment",
      entry({
        snapshot_revision: 2,
        files: {
          "src/a.ts": {
            path: "src/a.ts",
            status: "modified",
            staged: false,
            additions: 0,
            deletions: 0,
            diff: "",
            diff_state: "ready",
          },
        },
      }),
    );
    const ready = store.gitStatus.byEnvironmentRepo.environment[""];
    const projectedReady = projectGitStatusForDisplay(ready, displayEntry(store), 2).files[
      "src/a.ts"
    ];
    expect(projectedReady).toMatchObject({ diff: "", additions: 0 });
    expect(projectedReady).not.toHaveProperty("display_stale");

    applyGitStatus(store, "environment", entry({ snapshot_revision: 3, files: {}, modified: [] }));
    expect(displayEntry(store)?.files).toEqual({});
  });
});

describe("Git status display facets", () => {
  it("retains staged and unstaged representations independently", () => {
    const store = state();
    const ready = entry({
      files: {
        "src/a.ts": {
          path: "src/a.ts",
          status: "modified",
          staged: false,
          diff_state: "ready",
          staged_change: {
            status: "modified",
            additions: 1,
            deletions: 0,
            diff: "staged patch",
            diff_state: "ready",
          },
          unstaged_change: {
            status: "modified",
            additions: 2,
            deletions: 1,
            diff: "unstaged patch",
            diff_state: "ready",
          },
        },
      },
    });
    applyGitStatus(store, "environment", ready);
    applyGitStatus(
      store,
      "environment",
      entry({
        snapshot_revision: 2,
        detail_state: "pending",
        files: {
          "src/a.ts": {
            path: "src/a.ts",
            status: "modified",
            staged: false,
            diff_state: "pending",
            staged_change: { status: "modified", diff_state: "pending" },
            unstaged_change: { status: "modified", diff_state: "unavailable" },
          },
        },
      }),
    );

    const display = displayEntry(store)?.files["src/a.ts"];
    expect(display?.staged?.diff).toBe("staged patch");
    expect(display?.unstaged?.diff).toBe("unstaged patch");
  });
});

describe("Git status display compact layers", () => {
  it("retains a compact layer when the same path becomes mixed and pending", () => {
    const store = state();
    applyGitStatus(
      store,
      "environment",
      entry({
        files: {
          "src/a.ts": {
            path: "src/a.ts",
            status: "modified",
            staged: false,
            additions: 4,
            deletions: 1,
            diff: "ready compact unstaged patch",
            diff_state: "ready",
          },
        },
      }),
    );

    applyGitStatus(
      store,
      "environment",
      entry({
        snapshot_revision: 2,
        detail_state: "pending",
        files: {
          "src/a.ts": {
            path: "src/a.ts",
            status: "modified",
            staged: false,
            diff_state: "pending",
            staged_change: { status: "modified", diff_state: "pending" },
            unstaged_change: { status: "modified", diff_state: "pending" },
          },
        },
      }),
    );

    const raw = store.gitStatus.byEnvironmentRepo.environment[""];
    expect(raw.files["src/a.ts"]).not.toHaveProperty("diff");
    expect(raw.files["src/a.ts"].unstaged_change).not.toHaveProperty("diff");
    const projected = projectGitStatusForDisplay(raw, displayEntry(store), 2).files["src/a.ts"];
    expect(projected.unstaged_change).toMatchObject({
      diff: "ready compact unstaged patch",
      additions: 4,
      deletions: 1,
      display_stale: true,
    });
    expect(projected.staged_change).not.toHaveProperty("diff");
    expect(projected).not.toHaveProperty("diff");
  });
});

it.each([
  {
    layer: "staged" as const,
    staged: true,
    patch: STAGED_ONLY_PATCH,
    other: UNSTAGED_ONLY_PATCH,
  },
  {
    layer: "unstaged" as const,
    staged: false,
    patch: UNSTAGED_ONLY_PATCH,
    other: STAGED_ONLY_PATCH,
  },
])(
  "projects a mixed pending file as its surviving compact $layer layer",
  ({ layer, staged, patch, other }) => {
    const store = state();
    applyGitStatus(
      store,
      "environment",
      entry({
        files: {
          "src/a.ts": {
            path: "src/a.ts",
            status: "modified",
            staged,
            additions: 8,
            deletions: 2,
            diff: "combined staged and unstaged patch",
            diff_state: "ready",
            staged_change: {
              status: "modified",
              additions: 3,
              deletions: 1,
              diff: STAGED_ONLY_PATCH,
              diff_state: "ready",
            },
            unstaged_change: {
              status: "modified",
              additions: 5,
              deletions: 1,
              diff: UNSTAGED_ONLY_PATCH,
              diff_state: "ready",
            },
          },
        },
      }),
    );

    applyGitStatus(
      store,
      "environment",
      entry({
        snapshot_revision: 2,
        detail_state: "pending",
        files: {
          "src/a.ts": {
            path: "src/a.ts",
            status: "modified",
            staged,
            diff_state: "pending",
          },
        },
      }),
    );

    const raw = store.gitStatus.byEnvironmentRepo.environment[""];
    expect(raw.files["src/a.ts"]).not.toHaveProperty("diff");
    expect(raw.files["src/a.ts"]).not.toHaveProperty("additions");
    const projected = projectGitStatusForDisplay(raw, displayEntry(store), 2).files["src/a.ts"];
    expect(projected).toMatchObject({ diff: patch, diff_state: "pending", display_stale: true });
    expect(projected.diff).not.toBe(other);
    expect(displayEntry(store)?.files["src/a.ts"]).toMatchObject({ [layer]: { diff: patch } });
    expect(
      displayEntry(store)?.files["src/a.ts"]?.[layer === "staged" ? "unstaged" : "staged"],
    ).toBe(undefined);

    applyGitStatus(
      store,
      "environment",
      entry({
        snapshot_revision: 3,
        detail_state: "pending",
        files: {
          "src/a.ts": {
            path: "src/a.ts",
            status: "modified",
            staged,
            diff_state: "pending",
            staged_change: { status: "modified", diff_state: "pending" },
            unstaged_change: { status: "modified", diff_state: "pending" },
          },
        },
      }),
    );

    const mixedPending = store.gitStatus.byEnvironmentRepo.environment[""];
    const mixedProjection = projectGitStatusForDisplay(mixedPending, displayEntry(store), 2).files[
      "src/a.ts"
    ];
    const removedLayer = layer === "staged" ? "unstaged_change" : "staged_change";
    const retainedLayer = layer === "staged" ? "staged_change" : "unstaged_change";
    expect(mixedProjection[retainedLayer]).toMatchObject({ diff: patch, display_stale: true });
    expect(mixedProjection[removedLayer]).not.toHaveProperty("diff");
    expect(mixedProjection).not.toHaveProperty("diff");
  },
);

describe("Git status display layer replacement", () => {
  it("does not reuse a compact patch when a path changes from staged-only to unstaged-only", () => {
    const store = state();
    applyGitStatus(
      store,
      "environment",
      entry({
        files: {
          "src/a.ts": {
            path: "src/a.ts",
            status: "modified",
            staged: true,
            additions: 2,
            deletions: 0,
            diff: STAGED_ONLY_PATCH,
            diff_state: "ready",
          },
        },
      }),
    );
    applyGitStatus(
      store,
      "environment",
      entry({
        snapshot_revision: 2,
        detail_state: "pending",
        files: {
          "src/a.ts": {
            path: "src/a.ts",
            status: "modified",
            staged: false,
            diff_state: "pending",
          },
        },
      }),
    );

    let raw = store.gitStatus.byEnvironmentRepo.environment[""];
    expect(raw.files["src/a.ts"]).not.toHaveProperty("diff");
    let projected = projectGitStatusForDisplay(raw, displayEntry(store), 2).files["src/a.ts"];
    expect(projected).not.toHaveProperty("diff");
    expect(projected).not.toHaveProperty("display_stale");
    expect(displayEntry(store)?.files["src/a.ts"]?.staged).toBeUndefined();

    applyGitStatus(
      store,
      "environment",
      entry({
        snapshot_revision: 3,
        files: {
          "src/a.ts": {
            path: "src/a.ts",
            status: "modified",
            staged: false,
            additions: 3,
            deletions: 1,
            diff: UNSTAGED_ONLY_PATCH,
            diff_state: "ready",
          },
        },
      }),
    );
    applyGitStatus(
      store,
      "environment",
      entry({
        snapshot_revision: 4,
        detail_state: "pending",
        files: {
          "src/a.ts": {
            path: "src/a.ts",
            status: "modified",
            staged: true,
            diff_state: "pending",
          },
        },
      }),
    );

    raw = store.gitStatus.byEnvironmentRepo.environment[""];
    expect(raw.files["src/a.ts"]).not.toHaveProperty("diff");
    projected = projectGitStatusForDisplay(raw, displayEntry(store), 2).files["src/a.ts"];
    expect(projected).not.toHaveProperty("diff");
    expect(projected).not.toHaveProperty("display_stale");
    expect(displayEntry(store)?.files["src/a.ts"]?.unstaged).toBeUndefined();
  });
});

describe("Git status display scope replacement", () => {
  it("does not retain content across checkout identity changes or rejected results", () => {
    const store = state();
    applyGitStatus(store, "environment", entry());
    const priorDisplay = displayEntry(store);

    const rejected = entry({ snapshot_revision: 0, head_commit: "old-head", files: {} });
    expect(applyGitStatus(store, "environment", rejected)).toBe(false);
    expect(displayEntry(store)).toBe(priorDisplay);

    applyGitStatus(
      store,
      "environment",
      entry({
        snapshot_revision: 2,
        head_commit: "head-b",
        detail_state: "pending",
        files: {
          "src/a.ts": {
            path: "src/a.ts",
            status: "modified",
            staged: false,
            diff_state: "pending",
          },
        },
      }),
    );
    const current = store.gitStatus.byEnvironmentRepo.environment[""];
    expect(displayEntry(store)?.files["src/a.ts"]?.flat?.diff).toBeUndefined();
    expect(
      projectGitStatusForDisplay(current, displayEntry(store), 2).files["src/a.ts"],
    ).not.toHaveProperty("display_stale");
  });
});

describe("Git status display repository scopes", () => {
  it("does not reuse another repository's display snapshot", () => {
    const store = state();
    applyGitStatus(store, "environment", entry({ repository_name: "repo-a" }));
    applyGitStatus(
      store,
      "environment",
      entry({
        repository_name: "repo-b",
        detail_state: "pending",
        snapshot_revision: 2,
        files: {
          "src/a.ts": {
            path: "src/a.ts",
            status: "modified",
            staged: false,
            diff_state: "pending",
          },
        },
      }),
    );

    const repoB = store.gitStatus.byEnvironmentRepo.environment["repo-b"];
    expect(
      projectGitStatusForDisplay(
        repoB,
        (
          store as SessionRuntimeSliceState & {
            gitStatusDisplay: {
              byEnvironmentRepo: Record<string, Record<string, GitStatusDisplayEntry>>;
            };
          }
        ).gitStatusDisplay.byEnvironmentRepo.environment["repo-b"],
        2,
      ).files["src/a.ts"],
    ).not.toHaveProperty("diff");
  });
});
