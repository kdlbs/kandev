import { describe, it, expect, beforeEach } from "vitest";
import { create } from "zustand";
import { immer } from "zustand/middleware/immer";
import { createSessionRuntimeSlice } from "./session-runtime-slice";
import type { GitStatusEntry, SessionRuntimeSlice } from "./types";

function makeStore() {
  return create<SessionRuntimeSlice>()(immer<SessionRuntimeSlice>(createSessionRuntimeSlice));
}

const SESSION = "sess-1";

function status(overrides: Partial<GitStatusEntry> = {}): GitStatusEntry {
  return {
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
      "a.ts": { path: "a.ts", status: "modified", staged: false, diff: "-old\n+new" },
    },
    timestamp: "2026-05-28T00:00:00Z",
    ...overrides,
  };
}

describe("setGitStatus change reporting (single deep compare)", () => {
  let store: ReturnType<typeof makeStore>;

  beforeEach(() => {
    store = makeStore();
  });

  it("returns true on the first status and false for an identical follow-up", () => {
    expect(store.getState().setGitStatus(SESSION, status())).toBe(true);
    // Same content, newer timestamp only — must not count as a change.
    expect(
      store.getState().setGitStatus(SESSION, status({ timestamp: "2026-05-28T00:01:00Z" })),
    ).toBe(false);
  });

  it("does not rewrite byEnvironmentId for a duplicate snapshot", () => {
    store.getState().setGitStatus(SESSION, status());
    const ref = store.getState().gitStatus.byEnvironmentId[SESSION];
    store.getState().setGitStatus(SESSION, status({ timestamp: "2026-05-28T00:01:00Z" }));
    expect(store.getState().gitStatus.byEnvironmentId[SESSION]).toBe(ref);
  });

  it("returns true when a file's diff content changes", () => {
    store.getState().setGitStatus(SESSION, status());
    const changed = store.getState().setGitStatus(
      SESSION,
      status({
        files: {
          "a.ts": { path: "a.ts", status: "modified", staged: false, diff: "-old\n+newer" },
        },
      }),
    );
    expect(changed).toBe(true);
  });
});

describe("hasGitStatusChanged SHA fast-path", () => {
  let store: ReturnType<typeof makeStore>;

  beforeEach(() => {
    store = makeStore();
  });

  it("returns false when SHAs + file lists match but diff content shuffled", () => {
    // This is the heavy-rebase case: WorkspaceTracker re-emits the same snapshot
    // with reshuffled diff strings; the fast path must skip the deep walk.
    store.getState().setGitStatus(SESSION, status({ head_commit: "h1", base_commit: "b1" }));
    const changed = store.getState().setGitStatus(
      SESSION,
      status({
        head_commit: "h1",
        base_commit: "b1",
        files: {
          "a.ts": {
            path: "a.ts",
            status: "modified",
            staged: false,
            diff: "-old\n+totally-different",
          },
        },
      }),
    );
    expect(changed).toBe(false);
  });

  it("falls back to full compare (returns true) when head_commit differs", () => {
    store.getState().setGitStatus(SESSION, status({ head_commit: "h1", base_commit: "b1" }));
    const changed = store.getState().setGitStatus(
      SESSION,
      status({
        head_commit: "h2",
        base_commit: "b1",
        files: {
          "a.ts": { path: "a.ts", status: "modified", staged: false, diff: "-old\n+newer" },
        },
      }),
    );
    expect(changed).toBe(true);
  });

  it("preserves legacy behavior when neither side has SHAs", () => {
    // No head_commit → fast path skipped → diff content change still detected.
    store.getState().setGitStatus(SESSION, status());
    const changed = store.getState().setGitStatus(
      SESSION,
      status({
        files: {
          "a.ts": { path: "a.ts", status: "modified", staged: false, diff: "-old\n+newer" },
        },
      }),
    );
    expect(changed).toBe(true);
  });
});
