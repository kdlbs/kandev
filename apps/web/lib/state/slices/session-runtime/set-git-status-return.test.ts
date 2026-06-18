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
