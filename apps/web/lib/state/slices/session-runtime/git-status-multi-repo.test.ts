import { describe, it, expect, beforeEach } from "vitest";
import { create } from "zustand";
import { immer } from "zustand/middleware/immer";
import { createSessionRuntimeSlice } from "./session-runtime-slice";
import type { GitStatusEntry, SessionRuntimeSlice } from "./types";

/**
 * Builds a status with the given modified files. Both `modified` (the legacy
 * string list) and `files` (the FileInfo map used by hasGitStatusChanged for
 * change detection) are populated so the slice's same-status guard recognises
 * subsequent updates as meaningful changes.
 */
function entry(overrides: { modified: string[]; repository_name?: string }): GitStatusEntry {
  const files: GitStatusEntry["files"] = {};
  for (const path of overrides.modified) {
    files[path] = { path, status: "modified", staged: false };
  }
  return {
    branch: "main",
    remote_branch: null,
    modified: overrides.modified,
    added: [],
    deleted: [],
    untracked: [],
    renamed: [],
    ahead: 0,
    behind: 0,
    files,
    timestamp: new Date().toISOString() + Math.random(),
    repository_name: overrides.repository_name,
  };
}

const REPO_FRONTEND = "frontend";
const REPO_BACKEND = "backend";
const SESSION = "sess";
const FRONTEND_FILE = "frontend.tsx";
const BACKEND_FILE = "backend.go";

function makeStore() {
  return create<SessionRuntimeSlice>()(
    immer((set, get, store) => createSessionRuntimeSlice(set, get, store)),
  );
}

describe("session-runtime gitStatus multi-repo routing", () => {
  let useStore: ReturnType<typeof makeStore>;

  beforeEach(() => {
    useStore = makeStore();
  });

  it("routes a single status without repository_name into the legacy map only", () => {
    useStore.getState().setGitStatus(SESSION, entry({ modified: ["a.ts"] }));
    const state = useStore.getState();
    expect(state.gitStatus.byEnvironmentId[SESSION]?.modified).toEqual(["a.ts"]);
    // Per-repo map gets an entry under the empty key (mirrors single-repo) so
    // consumers that only read byEnvironmentRepo still see it.
    expect(Object.keys(state.gitStatus.byEnvironmentRepo[SESSION] ?? {})).toEqual([""]);
  });

  it("routes per-repo statuses into byEnvironmentRepo keyed by repository_name", () => {
    useStore
      .getState()
      .setGitStatus(SESSION, entry({ modified: [FRONTEND_FILE], repository_name: REPO_FRONTEND }));
    useStore
      .getState()
      .setGitStatus(SESSION, entry({ modified: [BACKEND_FILE], repository_name: REPO_BACKEND }));

    const repoMap = useStore.getState().gitStatus.byEnvironmentRepo[SESSION];
    expect(Object.keys(repoMap).sort()).toEqual([REPO_BACKEND, REPO_FRONTEND]);
    expect(repoMap[REPO_FRONTEND].modified).toEqual([FRONTEND_FILE]);
    expect(repoMap[REPO_BACKEND].modified).toEqual([BACKEND_FILE]);
  });

  it("does NOT overwrite the per-repo map when a sibling repo updates", () => {
    useStore
      .getState()
      .setGitStatus(SESSION, entry({ modified: [FRONTEND_FILE], repository_name: REPO_FRONTEND }));
    useStore
      .getState()
      .setGitStatus(SESSION, entry({ modified: [BACKEND_FILE], repository_name: REPO_BACKEND }));
    // Update frontend again — backend must still be there.
    useStore
      .getState()
      .setGitStatus(
        SESSION,
        entry({ modified: ["frontend2.tsx"], repository_name: REPO_FRONTEND }),
      );
    const repoMap = useStore.getState().gitStatus.byEnvironmentRepo[SESSION];
    expect(repoMap[REPO_FRONTEND].modified).toEqual(["frontend2.tsx"]);
    expect(repoMap[REPO_BACKEND].modified).toEqual([BACKEND_FILE]);
  });

  it("clearGitStatus drops both maps", () => {
    useStore
      .getState()
      .setGitStatus(SESSION, entry({ modified: ["x.ts"], repository_name: REPO_FRONTEND }));
    useStore.getState().clearGitStatus(SESSION);
    const state = useStore.getState();
    expect(state.gitStatus.byEnvironmentId[SESSION]).toBeUndefined();
    expect(state.gitStatus.byEnvironmentRepo[SESSION]).toBeUndefined();
  });
});
