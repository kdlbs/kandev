import { describe, expect, it, vi } from "vitest";
import { create } from "zustand";
import { immer } from "zustand/middleware/immer";
import { createSessionRuntimeSlice } from "./slices/session-runtime/session-runtime-slice";
import type { SessionRuntimeSlice, UserShellInfo } from "./slices/session-runtime/types";
import type { AppState } from "./store";
import type { StoreApi } from "zustand";
import { createOrdinaryShellOnce } from "./user-shell-creation";

function makeStore() {
  return create<SessionRuntimeSlice>()(immer<SessionRuntimeSlice>(createSessionRuntimeSlice));
}

function shell(id: string): UserShellInfo {
  return {
    terminalId: id,
    kind: "ordinary",
    seq: 1,
    state: "open",
    ptyStatus: "running",
    customName: null,
    displayName: "Terminal 1",
  };
}

const SCOPE = { environmentId: "env-1", taskId: "task-1" };

describe("createOrdinaryShellOnce", () => {
  it("joins concurrent creates for one store and scope", async () => {
    const store = makeStore() as unknown as StoreApi<AppState>;
    let resolve!: (value: UserShellInfo) => void;
    const create = vi.fn(
      () =>
        new Promise<UserShellInfo>((done) => {
          resolve = done;
        }),
    );

    const first = createOrdinaryShellOnce(store, SCOPE, create);
    const second = createOrdinaryShellOnce(store, SCOPE, create);
    expect(first).toBe(second);
    expect(create).toHaveBeenCalledTimes(1);

    resolve(shell("shell-1"));
    await expect(first).resolves.toMatchObject({ terminalId: "shell-1" });
    expect(store.getState().userShells.byScope[JSON.stringify(["env-1", "task-1"])]).toEqual([
      expect.objectContaining({ terminalId: "shell-1" }),
    ]);
  });

  it("creates again after the previous operation settles", async () => {
    const store = makeStore() as unknown as StoreApi<AppState>;
    const create = vi
      .fn()
      .mockResolvedValueOnce(shell("shell-1"))
      .mockResolvedValueOnce(shell("shell-2"));

    await createOrdinaryShellOnce(store, SCOPE, create);
    await createOrdinaryShellOnce(store, SCOPE, create);

    expect(create).toHaveBeenCalledTimes(2);
  });

  it("does not join creates from different scopes", async () => {
    const store = makeStore() as unknown as StoreApi<AppState>;
    const create = vi.fn(async () => shell("shell-1"));

    await Promise.all([
      createOrdinaryShellOnce(store, SCOPE, create),
      createOrdinaryShellOnce(store, { environmentId: "env-1", taskId: "task-2" }, create),
    ]);

    expect(create).toHaveBeenCalledTimes(2);
  });
});
