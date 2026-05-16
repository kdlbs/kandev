import { describe, it, expect } from "vitest";
import { create } from "zustand";
import { immer } from "zustand/middleware/immer";
import { createSessionRuntimeSlice } from "./session-runtime-slice";
import { createSessionSlice } from "../session/session-slice";
import type { SessionCommit, SessionRuntimeSlice } from "./types";
import type { SessionSlice } from "../session/types";

function makeStore() {
  return create<SessionRuntimeSlice>()(immer(createSessionRuntimeSlice));
}

type CombinedSlice = SessionRuntimeSlice & SessionSlice;

function makeCombinedStore() {
  return create<CombinedSlice>()(
    immer((set) => ({
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      ...(createSessionSlice as any)(set),
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      ...(createSessionRuntimeSlice as any)(set),
    })),
  );
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
    committed_at: "2026-05-09T00:00:00Z",
    files_changed: 1,
    insertions: 1,
    deletions: 0,
    created_at: "2026-05-09T00:00:00Z",
  };
}

describe("registerSessionEnvironment — migrateEnvKeyedData", () => {
  it("setUserShells writes directly under the environmentId key (no session translation)", () => {
    const store = makeStore();

    store.getState().setUserShells("env-1", [{ terminalId: "t1" } as never]);

    const state = store.getState();
    expect(state.userShells.byEnvironmentId["env-1"]).toEqual([{ terminalId: "t1" }]);
    expect(state.userShells.loaded["env-1"]).toBe(true);
  });

  it("setUserShells with empty environmentId is a no-op", () => {
    const store = makeStore();

    store.getState().setUserShells("", [{ terminalId: "t1" } as never]);

    const state = store.getState();
    expect(state.userShells.byEnvironmentId[""]).toBeUndefined();
  });

  it("migrates data from sessionId key to environmentId key", () => {
    const store = makeStore();

    // Simulate data arriving under the fallback sessionId key
    store.getState().setSessionCommits("sess-1", [{ commit_sha: "abc" } as never]);
    store.getState().setGitStatus("sess-1", { branch: "main" } as never);
    store.getState().appendShellOutput("sess-1", "hello");

    // Register the environment mapping
    store.getState().registerSessionEnvironment("sess-1", "env-1");

    const state = store.getState();
    // Data should now live under the environmentId key
    expect(state.sessionCommits.byEnvironmentId["env-1"]).toEqual([{ commit_sha: "abc" }]);
    expect(state.gitStatus.byEnvironmentId["env-1"]).toEqual({ branch: "main" });
    expect(state.shell.outputs["env-1"]).toBe("hello");
    // sessionId key should be cleaned up
    expect(state.sessionCommits.byEnvironmentId["sess-1"]).toBeUndefined();
    expect(state.gitStatus.byEnvironmentId["sess-1"]).toBeUndefined();
    expect(state.shell.outputs["sess-1"]).toBeUndefined();
  });

  it("does not clobber pre-existing environmentId data", () => {
    const store = makeStore();

    // Data already stored under the environmentId key
    store.getState().registerSessionEnvironment("other-sess", "env-1");
    store.getState().setSessionCommits("other-sess", [{ commit_sha: "existing" } as never]);

    // Stale data under sessionId from before mapping was known
    store.getState().setGitStatus("sess-2", { branch: "stale" } as never);

    // A different session wrote commits under its own fallback key
    store.setState((draft) => {
      draft.sessionCommits.byEnvironmentId["sess-2"] = [{ commit_sha: "orphan" } as never];
    });

    // Register: sess-2 maps to env-1 (which already has commit data)
    store.getState().registerSessionEnvironment("sess-2", "env-1");

    const state = store.getState();
    // Commits: pre-existing env-1 data preserved, orphaned sess-2 key deleted
    expect(state.sessionCommits.byEnvironmentId["env-1"]).toEqual([{ commit_sha: "existing" }]);
    expect(state.sessionCommits.byEnvironmentId["sess-2"]).toBeUndefined();
    // Git status: no env-1 data existed, so sess-2 data migrated
    expect(state.gitStatus.byEnvironmentId["env-1"]).toEqual({ branch: "stale" });
    expect(state.gitStatus.byEnvironmentId["sess-2"]).toBeUndefined();
  });

  it("no-ops when sessionId equals environmentId", () => {
    const store = makeStore();

    store.getState().setSessionCommits("local-1", [{ commit_sha: "abc" } as never]);

    store.getState().registerSessionEnvironment("local-1", "local-1");

    const state = store.getState();
    // Data stays under the same key
    expect(state.sessionCommits.byEnvironmentId["local-1"]).toEqual([{ commit_sha: "abc" }]);
  });

  it("migrates only sub-keys that have data", () => {
    const store = makeStore();

    // Only put data in shell outputs and userShells, leave others empty
    store.getState().appendShellOutput("sess-3", "output");
    store.setState((draft) => {
      draft.userShells.byEnvironmentId["sess-3"] = [{ terminalId: "t1" } as never];
    });

    store.getState().registerSessionEnvironment("sess-3", "env-3");

    const state = store.getState();
    expect(state.shell.outputs["env-3"]).toBe("output");
    expect(state.shell.outputs["sess-3"]).toBeUndefined();
    expect(state.userShells.byEnvironmentId["env-3"]).toEqual([{ terminalId: "t1" }]);
    expect(state.userShells.byEnvironmentId["sess-3"]).toBeUndefined();
    // Other stores should have no data for either key
    expect(state.sessionCommits.byEnvironmentId["env-3"]).toBeUndefined();
    expect(state.sessionCommits.byEnvironmentId["sess-3"]).toBeUndefined();
  });
});

describe("setGitStatus", () => {
  it("updates cached status when only branch diff totals change", () => {
    const store = makeStore();

    store.getState().setGitStatus("sess-1", {
      branch: "feature",
      remote_branch: "",
      ahead: 0,
      behind: 0,
      files: {},
      timestamp: "same-timestamp",
      branch_additions: 0,
      branch_deletions: 0,
    } as never);

    store.getState().setGitStatus("sess-1", {
      branch: "feature",
      remote_branch: "",
      ahead: 0,
      behind: 0,
      files: {},
      timestamp: "same-timestamp",
      branch_additions: 3,
      branch_deletions: 1,
    } as never);

    expect(store.getState().gitStatus.byEnvironmentId["sess-1"]).toMatchObject({
      branch_additions: 3,
      branch_deletions: 1,
    });
  });
});

describe("setTaskSession — cross-slice migration", () => {
  it("migrates env-keyed data when task_environment_id is set", () => {
    const store = makeCombinedStore();

    // Data arrives under fallback sessionId key before the mapping is known
    store.getState().setGitStatus("sess-1", { branch: "main" } as never);
    store.getState().setSessionCommits("sess-1", [{ commit_sha: "abc" } as never]);

    // Session update arrives with task_environment_id
    store.getState().setTaskSession({
      id: "sess-1",
      task_id: "task-1",
      state: "RUNNING",
      task_environment_id: "env-1",
      started_at: "",
      updated_at: "",
    });

    const state = store.getState();
    expect(state.environmentIdBySessionId["sess-1"]).toBe("env-1");
    expect(state.gitStatus.byEnvironmentId["env-1"]).toEqual({ branch: "main" });
    expect(state.gitStatus.byEnvironmentId["sess-1"]).toBeUndefined();
    expect(state.sessionCommits.byEnvironmentId["env-1"]).toEqual([{ commit_sha: "abc" }]);
    expect(state.sessionCommits.byEnvironmentId["sess-1"]).toBeUndefined();
  });

  it("does not migrate when task_environment_id is absent", () => {
    const store = makeCombinedStore();

    store.getState().setGitStatus("sess-2", { branch: "dev" } as never);

    store.getState().setTaskSession({
      id: "sess-2",
      task_id: "task-2",
      state: "RUNNING",
      started_at: "",
      updated_at: "",
    });

    const state = store.getState();
    expect(state.environmentIdBySessionId["sess-2"]).toBeUndefined();
    expect(state.gitStatus.byEnvironmentId["sess-2"]).toEqual({ branch: "dev" });
  });
});

describe("setTaskSessionsForTask — bulk cross-slice migration", () => {
  it("migrates env-keyed data for all sessions with task_environment_id", () => {
    const store = makeCombinedStore();

    // Data under fallback keys
    store.getState().setGitStatus("sess-a", { branch: "a" } as never);
    store.getState().appendShellOutput("sess-b", "hello");

    store.getState().setTaskSessionsForTask("task-1", [
      {
        id: "sess-a",
        task_id: "task-1",
        state: "COMPLETED",
        task_environment_id: "env-x",
        started_at: "",
        updated_at: "",
      },
      {
        id: "sess-b",
        task_id: "task-1",
        state: "RUNNING",
        task_environment_id: "env-y",
        started_at: "",
        updated_at: "",
      },
    ]);

    const state = store.getState();
    expect(state.gitStatus.byEnvironmentId["env-x"]).toEqual({ branch: "a" });
    expect(state.gitStatus.byEnvironmentId["sess-a"]).toBeUndefined();
    expect(state.shell.outputs["env-y"]).toBe("hello");
    expect(state.shell.outputs["sess-b"]).toBeUndefined();
  });
});

describe("setSessionCommits — stale response protection", () => {
  it("does not overwrite existing commits with an empty array", () => {
    const store = makeStore();

    store.getState().setSessionCommits("sess-1", [commit("abc")]);
    store.getState().setSessionCommits("sess-1", []);

    const state = store.getState();
    expect(state.sessionCommits.byEnvironmentId["sess-1"]).toEqual([commit("abc")]);
  });

  it("sets an empty array when no existing commits are present", () => {
    const store = makeStore();

    store.getState().setSessionCommits("sess-1", []);

    const state = store.getState();
    expect(state.sessionCommits.byEnvironmentId["sess-1"]).toEqual([]);
  });

  it("overwrites existing commits with a non-empty array", () => {
    const store = makeStore();

    store.getState().setSessionCommits("sess-1", [commit("abc")]);
    store.getState().setSessionCommits("sess-1", [commit("def")]);

    const state = store.getState();
    expect(state.sessionCommits.byEnvironmentId["sess-1"]).toEqual([commit("def")]);
  });

  it("allows an empty response after commits are explicitly cleared", () => {
    const store = makeStore();

    store.getState().setSessionCommits("sess-1", [commit("abc")]);
    store.getState().clearSessionCommits("sess-1");
    store.getState().setSessionCommits("sess-1", []);

    const state = store.getState();
    expect(state.sessionCommits.byEnvironmentId["sess-1"]).toEqual([]);
  });
});
