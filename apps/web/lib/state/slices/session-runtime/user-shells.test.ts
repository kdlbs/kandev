import { describe, it, expect, beforeEach } from "vitest";
import { create } from "zustand";
import { immer } from "zustand/middleware/immer";
import { createSessionRuntimeSlice } from "./session-runtime-slice";
import type { SessionRuntimeSlice, UserShellInfo } from "./types";
import { userShellScopeKey, type UserShellScope } from "@/lib/state/user-shell-scope";

function makeStore() {
  return create<SessionRuntimeSlice>()(immer<SessionRuntimeSlice>(createSessionRuntimeSlice));
}

const ENV = "env-1";
const SCOPE_A: UserShellScope = { environmentId: ENV, taskId: "task-a" };
const SCOPE_B: UserShellScope = { environmentId: ENV, taskId: "task-b" };

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

function scoped(store: ReturnType<typeof makeStore>, scope: UserShellScope) {
  return store.getState().userShells.byScope[userShellScopeKey(scope)];
}

function shared(store: ReturnType<typeof makeStore>) {
  return store.getState().userShells.byEnvironmentId[ENV];
}

describe("userShells reducers", () => {
  let store: ReturnType<typeof makeStore>;

  beforeEach(() => {
    store = makeStore();
  });

  it("keeps ordinary rows in one task/environment scope", () => {
    store.getState().setUserShells(SCOPE_A, [shell("a"), shell("b", { seq: 2 })]);
    const list = scoped(store, SCOPE_A);
    expect(list).toHaveLength(2);
    expect(list?.[0].terminalId).toBe("a");
    expect(list?.[1].seq).toBe(2);
    expect(store.getState().userShells.listed[userShellScopeKey(SCOPE_A)]).toBe(true);
  });

  it("isolates ordinary rows between tasks that share an environment", () => {
    store.getState().setUserShells(SCOPE_A, [shell("shell-a")]);
    store.getState().setUserShells(SCOPE_B, [shell("shell-b")]);

    expect(scoped(store, SCOPE_A)?.map((item) => item.terminalId)).toEqual(["shell-a"]);
    expect(scoped(store, SCOPE_B)?.map((item) => item.terminalId)).toEqual(["shell-b"]);
    expect(shared(store)).toEqual([]);
  });

  it("shares fixed and script rows across task scopes in the same environment", () => {
    const fixed = shell("bottom-panel", { kind: "fixed" });
    const script = shell("script-1", { kind: "script" });
    store.getState().setUserShells(SCOPE_A, [shell("shell-a"), fixed, script]);

    expect(scoped(store, SCOPE_A)?.map((item) => item.terminalId)).toEqual(["shell-a"]);
    expect(shared(store)?.map((item) => item.terminalId)).toEqual(["bottom-panel", "script-1"]);

    store.getState().setUserShells(SCOPE_B, [shell("shell-b"), fixed, script]);
    expect(scoped(store, SCOPE_B)?.map((item) => item.terminalId)).toEqual(["shell-b"]);
    expect(shared(store)?.map((item) => item.terminalId)).toEqual(["bottom-panel", "script-1"]);
  });

  it("discards environment-only ordinary rows instead of adopting them", () => {
    store
      .getState()
      .setUserShells({ environmentId: ENV, taskId: null }, [
        shell("shell-foreign"),
        shell("script-1", { kind: "script" }),
      ]);

    expect(scoped(store, { environmentId: ENV, taskId: null })).toBeUndefined();
    expect(shared(store)?.map((item) => item.terminalId)).toEqual(["script-1"]);
  });

  it("rejects adding an ordinary row without a task", () => {
    store.getState().addUserShell({ environmentId: ENV, taskId: null }, shell("shell-a"));
    expect(scoped(store, { environmentId: ENV, taskId: null })).toBeUndefined();
    expect(shared(store)).toBeUndefined();
  });

  it("addUserShell appends idempotently inside its scope", () => {
    store.getState().addUserShell(SCOPE_A, shell("a"));
    store.getState().addUserShell(SCOPE_A, shell("a"));
    store.getState().addUserShell(SCOPE_B, shell("b"));
    expect(scoped(store, SCOPE_A)).toHaveLength(1);
    expect(scoped(store, SCOPE_B)).toHaveLength(1);
  });

  it("removeUserShell tombstones only the addressed scope", () => {
    store.getState().setUserShells(SCOPE_A, [shell("a"), shell("b")]);
    store.getState().setUserShells(SCOPE_B, [shell("c")]);
    store.getState().removeUserShell(SCOPE_A, "a");

    expect(scoped(store, SCOPE_A)?.map((item) => item.terminalId)).toEqual(["b"]);
    expect(scoped(store, SCOPE_B)?.map((item) => item.terminalId)).toEqual(["c"]);
    expect(store.getState().userShells.dismissedByScope[userShellScopeKey(SCOPE_A)]).toEqual({
      a: true,
    });
  });

  it("keeps an optimistically removed shell out of a deferred stale list response", async () => {
    store.getState().setUserShells(SCOPE_A, [shell("a"), shell("b")]);
    let resolveList!: (shells: UserShellInfo[]) => void;
    const deferredList = new Promise<UserShellInfo[]>((resolve) => {
      resolveList = resolve;
    });
    const applyList = deferredList.then((shells) =>
      store.getState().setUserShells(SCOPE_A, shells),
    );

    store.getState().removeUserShell(SCOPE_A, "a");
    resolveList([shell("a"), shell("b")]);
    await applyList;

    expect(scoped(store, SCOPE_A)?.map((item) => item.terminalId)).toEqual(["b"]);
  });

  it("keeps an optimistically removed shell out of stale add events in that scope", () => {
    store.getState().setUserShells(SCOPE_A, [shell("a")]);
    store.getState().removeUserShell(SCOPE_A, "a");

    store.getState().addUserShell(SCOPE_A, shell("a"));

    expect(scoped(store, SCOPE_A)).toEqual([]);
  });

  it("updateUserShell patches the matching row without touching siblings", () => {
    store.getState().setUserShells(SCOPE_A, [shell("a", { customName: null })]);
    store.getState().setUserShells(SCOPE_B, [shell("b", { customName: null })]);
    store.getState().updateUserShell(SCOPE_A, "a", { customName: "build watcher" });

    expect(scoped(store, SCOPE_A)?.[0].customName).toBe("build watcher");
    expect(scoped(store, SCOPE_B)?.[0].customName).toBeNull();
  });

  it("updateUserShell also patches shared script rows", () => {
    store.getState().setUserShells(SCOPE_A, [shell("script-1", { kind: "script" })]);
    store.getState().updateUserShell(SCOPE_A, "script-1", { state: "parked" });
    expect(shared(store)?.[0].state).toBe("parked");
  });

  it("updateUserShell is a no-op for unknown scope", () => {
    store.getState().updateUserShell(SCOPE_A, "a", { state: "parked" });
    expect(scoped(store, SCOPE_A)).toBeUndefined();
  });
});

describe("userShells read authority", () => {
  let store: ReturnType<typeof makeStore>;

  beforeEach(() => {
    store = makeStore();
  });

  it("withdraws successful-list authority as soon as a read starts", () => {
    store.getState().setUserShells(SCOPE_A, []);
    const key = userShellScopeKey(SCOPE_A);
    expect(store.getState().userShells.listed[key]).toBe(true);

    store.getState().setUserShellsLoading(SCOPE_A, true);

    expect(store.getState().userShells.listed[key]).toBe(false);
    expect(store.getState().userShells.loading[key]).toBe(true);
  });

  it("settles a failed read without listing authority while retaining known rows", () => {
    store.getState().setUserShells(SCOPE_A, [shell("a")]);
    store.getState().setUserShellsLoading(SCOPE_A, true);

    store.getState().setUserShellsFailed(SCOPE_A);

    const key = userShellScopeKey(SCOPE_A);
    expect(store.getState().userShells.loaded[key]).toBe(true);
    expect(store.getState().userShells.loading[key]).toBe(false);
    expect(store.getState().userShells.listed[key]).toBe(false);
    expect(scoped(store, SCOPE_A)?.map((item) => item.terminalId)).toEqual(["a"]);
  });

  it("keeps newer shared rows when an older task list resolves late", () => {
    store.getState().setUserShells(SCOPE_B, [shell("script-1", { kind: "script" })], 2);
    store.getState().setUserShells(SCOPE_A, [shell("shell-a")], 1);

    expect(shared(store)?.map((item) => item.terminalId)).toEqual(["script-1"]);
    expect(scoped(store, SCOPE_A)?.map((item) => item.terminalId)).toEqual(["shell-a"]);
    expect(store.getState().userShells.sharedGeneration[ENV]).toBe(2);
  });

  it("keeps sibling scope authority when one scope's read fails", () => {
    store.getState().setUserShells(SCOPE_A, [shell("a")]);
    store.getState().setUserShells(SCOPE_B, [shell("b")]);

    store.getState().setUserShellsFailed(SCOPE_A);

    expect(store.getState().userShells.listed[userShellScopeKey(SCOPE_B)]).toBe(true);
    expect(scoped(store, SCOPE_B)?.map((item) => item.terminalId)).toEqual(["b"]);
  });
});
