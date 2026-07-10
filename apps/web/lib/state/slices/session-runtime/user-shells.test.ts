import { describe, it, expect, beforeEach } from "vitest";
import { create } from "zustand";
import { immer } from "zustand/middleware/immer";
import { createSessionRuntimeSlice } from "./session-runtime-slice";
import type { SessionRuntimeSlice, UserShellInfo } from "./types";

function makeStore() {
  return create<SessionRuntimeSlice>()(immer<SessionRuntimeSlice>(createSessionRuntimeSlice));
}

const ENV = "env-1";
function shell(id: string, overrides: Partial<UserShellInfo> = {}): UserShellInfo {
  return {
    terminalId: id,
    kind: "ordinary",
    seq: 1,
    state: "open",
    ptyStatus: "running",
    customName: null,
    displayName: "Terminal 1",
    ...overrides,
  };
}

describe("userShells reducers", () => {
  let store: ReturnType<typeof makeStore>;

  beforeEach(() => {
    store = makeStore();
  });

  it("setUserShells populates the env list", () => {
    store.getState().setUserShells(ENV, [shell("a"), shell("b", { seq: 2 })]);
    const list = store.getState().userShells.byEnvironmentId[ENV];
    expect(list).toHaveLength(2);
    expect(list?.[0].terminalId).toBe("a");
    expect(list?.[1].seq).toBe(2);
  });

  it("addUserShell appends idempotently", () => {
    store.getState().addUserShell(ENV, shell("a"));
    store.getState().addUserShell(ENV, shell("a"));
    expect(store.getState().userShells.byEnvironmentId[ENV]).toHaveLength(1);
  });

  it("removeUserShell removes by terminalId", () => {
    store.getState().setUserShells(ENV, [shell("a"), shell("b")]);
    store.getState().removeUserShell(ENV, "a");
    const list = store.getState().userShells.byEnvironmentId[ENV];
    expect(list).toHaveLength(1);
    expect(list?.[0].terminalId).toBe("b");
  });

  it("updateUserShell patches the matching row", () => {
    store.getState().setUserShells(ENV, [shell("a", { customName: null })]);
    store.getState().updateUserShell(ENV, "a", { customName: "build watcher" });
    const list = store.getState().userShells.byEnvironmentId[ENV];
    expect(list?.[0].customName).toBe("build watcher");
  });

  it("updateUserShell leaves other rows untouched", () => {
    store.getState().setUserShells(ENV, [shell("a"), shell("b", { state: "open" })]);
    store.getState().updateUserShell(ENV, "b", { state: "parked" });
    const list = store.getState().userShells.byEnvironmentId[ENV];
    expect(list?.[0].state).toBe("open");
    expect(list?.[1].state).toBe("parked");
  });

  it("updateUserShell is a no-op for unknown environmentId", () => {
    store.getState().updateUserShell("missing-env", "a", { state: "parked" });
    expect(store.getState().userShells.byEnvironmentId["missing-env"]).toBeUndefined();
  });
});
