import { describe, it, expect, beforeEach } from "vitest";
import { create } from "zustand";
import { immer } from "zustand/middleware/immer";
import { createSessionRuntimeSlice } from "./session-runtime-slice";
import type { SessionCommit, SessionRuntimeSlice } from "./types";

function makeStore() {
  return create<SessionRuntimeSlice>()(
    immer((set, get, store) => createSessionRuntimeSlice(set, get, store)),
  );
}

const SESSION = "sess-1";

function commit(overrides: Partial<SessionCommit>): SessionCommit {
  return {
    id: "id",
    session_id: SESSION,
    commit_sha: "sha",
    parent_sha: "parent",
    commit_message: "msg",
    author_name: "test",
    author_email: "test@test",
    files_changed: 0,
    insertions: 0,
    deletions: 0,
    committed_at: "2026-05-28T00:00:00Z",
    created_at: "2026-05-28T00:00:00Z",
    ...overrides,
  };
}

describe("bumpSessionCommitsRefetch", () => {
  let useStore: ReturnType<typeof makeStore>;

  beforeEach(() => {
    useStore = makeStore();
  });

  it("increments the trigger counter for the session's env key", () => {
    useStore.getState().bumpSessionCommitsRefetch(SESSION);
    expect(useStore.getState().sessionCommits.refetchTrigger[SESSION]).toBe(1);

    useStore.getState().bumpSessionCommitsRefetch(SESSION);
    expect(useStore.getState().sessionCommits.refetchTrigger[SESSION]).toBe(2);
  });

  it("does NOT touch byEnvironmentId — visible commits stay during refetch", () => {
    // Stale-while-revalidate is the whole point: the WS handler bumps so the
    // hook refetches, but the panel keeps showing the previous commits until
    // the new list arrives. If a bump cleared the list, the panel would
    // briefly fall through to its empty state.
    const existing = [commit({ commit_sha: "abc" }), commit({ commit_sha: "def" })];
    useStore.getState().setSessionCommits(SESSION, existing);

    useStore.getState().bumpSessionCommitsRefetch(SESSION);

    expect(useStore.getState().sessionCommits.byEnvironmentId[SESSION]).toEqual(existing);
  });

  it("resolves through environmentIdBySessionId so co-environment sessions share a trigger", () => {
    // sess-1 and sess-2 share env-foo; the trigger lives under env-foo so
    // either session bumping it refetches both.
    useStore.getState().registerSessionEnvironment(SESSION, "env-foo");
    useStore.getState().registerSessionEnvironment("sess-2", "env-foo");

    useStore.getState().bumpSessionCommitsRefetch(SESSION);

    const triggers = useStore.getState().sessionCommits.refetchTrigger;
    expect(triggers["env-foo"]).toBe(1);
    expect(triggers[SESSION]).toBeUndefined();
  });
});

describe("setSessionCommits — empty-response guard", () => {
  let useStore: ReturnType<typeof makeStore>;

  beforeEach(() => {
    useStore = makeStore();
  });

  it("rejects [] over a populated list by default (race protection)", () => {
    // The default guard protects against a stale fetch response landing
    // after incremental commit_created notifications already populated the
    // store. Without it, an in-flight initial fetch could clobber the live
    // list with [].
    useStore.getState().setSessionCommits(SESSION, [commit({ commit_sha: "abc" })]);

    useStore.getState().setSessionCommits(SESSION, []);

    const after = useStore.getState().sessionCommits.byEnvironmentId[SESSION];
    expect(after).toHaveLength(1);
    expect(after[0].commit_sha).toBe("abc");
  });

  it("accepts [] when allowEmpty:true (authoritative response after reset)", () => {
    // After commits_reset/branch_switched, a refetch can legitimately return
    // []. The caller opts into the empty-accepting path so the panel stops
    // showing the pre-reset list.
    useStore.getState().setSessionCommits(SESSION, [commit({ commit_sha: "abc" })]);

    useStore.getState().setSessionCommits(SESSION, [], { allowEmpty: true });

    expect(useStore.getState().sessionCommits.byEnvironmentId[SESSION]).toEqual([]);
  });

  it("writes the populated list regardless of allowEmpty", () => {
    // Sanity: allowEmpty only changes behaviour for empty arrays. A populated
    // list always overwrites.
    useStore.getState().setSessionCommits(SESSION, [commit({ commit_sha: "old" })]);

    useStore.getState().setSessionCommits(SESSION, [commit({ commit_sha: "new" })]);

    const after = useStore.getState().sessionCommits.byEnvironmentId[SESSION];
    expect(after).toHaveLength(1);
    expect(after[0].commit_sha).toBe("new");
  });
});
