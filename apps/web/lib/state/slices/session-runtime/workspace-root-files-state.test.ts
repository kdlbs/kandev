import { describe, expect, it } from "vitest";
import { create } from "zustand";
import { immer } from "zustand/middleware/immer";
import { createSessionRuntimeSlice, purgeSessionRuntimeState } from "./session-runtime-slice";
import type { SessionRuntimeSlice } from "./types";

const ENVIRONMENT = "environment-root-files";

const listing = {
  supported: true,
  files: [{ path: "dashboard.json", size: 12, modified_at: "2026-11-19T10:00:00Z" }],
  truncated: false,
  destinations: [{ repository_name: "web", path: "apps/web" }],
};

function makeStore() {
  return create<SessionRuntimeSlice>()(immer(createSessionRuntimeSlice));
}

describe("workspace root files runtime state", () => {
  it("stores a listing by environment and purges it with the last session", () => {
    const store = makeStore();
    store.getState().registerSessionEnvironment("session-root-files", ENVIRONMENT);
    const setListing = (store.getState() as unknown as Record<string, unknown>)[
      "setWorkspaceRootFiles"
    ];

    expect(setListing).toBeTypeOf("function");
    (setListing as (environmentId: string, value: typeof listing) => void)(ENVIRONMENT, listing);
    expect(
      (
        store.getState() as unknown as {
          workspaceRootFiles: { byEnvironmentId: Record<string, unknown> };
        }
      ).workspaceRootFiles.byEnvironmentId[ENVIRONMENT],
    ).toEqual(listing);

    store.setState((state) => {
      purgeSessionRuntimeState(state, "session-root-files");
    });
    expect(
      (
        store.getState() as unknown as {
          workspaceRootFiles: { byEnvironmentId: Record<string, unknown> };
        }
      ).workspaceRootFiles.byEnvironmentId[ENVIRONMENT],
    ).toBeUndefined();
  });
});
