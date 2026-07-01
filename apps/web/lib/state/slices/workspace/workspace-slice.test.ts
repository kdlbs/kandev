import { describe, expect, it } from "vitest";
import { create } from "zustand";
import { immer } from "zustand/middleware/immer";
import { createWorkspaceSlice } from "./workspace-slice";
import type { WorkspaceSlice } from "./types";

function makeStore() {
  return create<WorkspaceSlice>()(
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    immer((...a) => ({ ...(createWorkspaceSlice as any)(...a) })),
  );
}

describe("repository scripts", () => {
  it("does not expose repository scripts server-state through the workspace slice", () => {
    const state = makeStore().getState() as unknown as Record<string, unknown>;

    expect("repositoryScripts" in state).toBe(false);
    expect("setRepositoryScripts" in state).toBe(false);
    expect("clearRepositoryScripts" in state).toBe(false);
  });
});

describe("repository branches", () => {
  it("does not expose repository branch server-state through the workspace slice", () => {
    const state = makeStore().getState() as unknown as Record<string, unknown>;

    expect("repositoryBranches" in state).toBe(false);
    expect("setRepositoryBranches" in state).toBe(false);
  });
});

describe("workspace repositories", () => {
  it("does not expose repository server-state through the workspace slice", () => {
    const state = makeStore().getState() as unknown as Record<string, unknown>;

    expect("repositories" in state).toBe(false);
    expect("setRepositories" in state).toBe(false);
  });
});
