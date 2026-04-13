import { describe, it, expect } from "vitest";
import { create } from "zustand";
import { immer } from "zustand/middleware/immer";
import { createSessionRuntimeSlice } from "./session-runtime-slice";
import type { SessionRuntimeSlice } from "./types";

function makeStore() {
  return create<SessionRuntimeSlice>()(immer(createSessionRuntimeSlice));
}

describe("registerSessionEnvironment — migrateEnvKeyedData", () => {
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
    store.getState().setUserShells("sess-3", [{ terminalId: "t1" } as never]);

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
