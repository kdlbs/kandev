import { describe, it, expect, beforeEach } from "vitest";
import { create } from "zustand";
import { immer } from "zustand/middleware/immer";
import { createSessionRuntimeSlice } from "./session-runtime-slice";
import type { GitStatusEntry, SessionRuntimeSlice } from "./types";

function makeStore() {
  return create<SessionRuntimeSlice>()(immer<SessionRuntimeSlice>(createSessionRuntimeSlice));
}

const SESSION = "sess-1";
const MODIFIED_STATUS = "modified" as const;
const NEWER_TIMESTAMP = "2026-05-28T00:01:00Z";

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
      "a.ts": { path: "a.ts", status: MODIFIED_STATUS, staged: false, diff: "-old\n+new" },
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
    expect(store.getState().setGitStatus(SESSION, status({ timestamp: NEWER_TIMESTAMP }))).toBe(
      false,
    );
  });

  it("does not rewrite byEnvironmentId for a duplicate snapshot", () => {
    store.getState().setGitStatus(SESSION, status());
    const ref = store.getState().gitStatus.byEnvironmentId[SESSION];
    store.getState().setGitStatus(SESSION, status({ timestamp: NEWER_TIMESTAMP }));
    expect(store.getState().gitStatus.byEnvironmentId[SESSION]).toBe(ref);
  });

  it("returns true when a file's diff content changes", () => {
    store.getState().setGitStatus(SESSION, status());
    const changed = store.getState().setGitStatus(
      SESSION,
      status({
        files: {
          "a.ts": { path: "a.ts", status: MODIFIED_STATUS, staged: false, diff: "-old\n+newer" },
        },
      }),
    );
    expect(changed).toBe(true);
  });

  it("returns true when only a mixed-change facet changes", () => {
    const first = status({
      files: {
        "a.ts": {
          path: "a.ts",
          status: MODIFIED_STATUS,
          staged: false,
          diff: "combined",
          staged_change: { status: MODIFIED_STATUS, diff: "staged one" },
          unstaged_change: { status: MODIFIED_STATUS, diff: "unstaged" },
        } as never,
      },
    });
    store.getState().setGitStatus(SESSION, first);

    const changed = store.getState().setGitStatus(SESSION, {
      ...first,
      timestamp: NEWER_TIMESTAMP,
      files: {
        "a.ts": {
          ...first.files["a.ts"],
          staged_change: { status: MODIFIED_STATUS, diff: "staged two" },
        } as never,
      },
    });

    expect(changed).toBe(true);
  });

  it("returns true when only the submodule marker changes", () => {
    store.getState().setGitStatus(SESSION, status({ is_submodule: false }));

    expect(store.getState().setGitStatus(SESSION, status({ is_submodule: true }))).toBe(true);
  });

  it("normalizes legacy composite file keys before storing the status", () => {
    store.getState().setGitStatus(
      SESSION,
      status({
        files: {
          "frontend\u0000src/app.ts": { status: MODIFIED_STATUS, staged: false } as never,
        },
      }),
    );

    expect(store.getState().gitStatus.byEnvironmentId[SESSION]?.files).toEqual({
      "frontend\u0000src/app.ts": {
        path: "src/app.ts",
        repository_name: "frontend",
        status: "modified",
        staged: false,
      },
    });
  });

  it("returns false when the submodule marker is unchanged", () => {
    store.getState().setGitStatus(SESSION, status({ is_submodule: true }));

    expect(
      store
        .getState()
        .setGitStatus(SESSION, status({ is_submodule: true, timestamp: NEWER_TIMESTAMP })),
    ).toBe(false);
  });
});
