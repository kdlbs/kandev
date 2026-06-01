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

// The workspace slice is now client-only: it tracks the active workspace
// selection. All server data moved to TanStack Query (see use-workspaces,
// use-repositories, use-repository-branches).
describe("workspace slice (client-only active selection)", () => {
  it("defaults to no active workspace", () => {
    const s = makeStore();
    expect(s.getState().workspaces.activeId).toBeNull();
  });

  it("setActiveWorkspace updates the active id", () => {
    const s = makeStore();
    s.getState().setActiveWorkspace("ws-1");
    expect(s.getState().workspaces.activeId).toBe("ws-1");
    s.getState().setActiveWorkspace(null);
    expect(s.getState().workspaces.activeId).toBeNull();
  });

  it("setActiveWorkspace is a no-op when the id is unchanged", () => {
    const s = makeStore();
    s.getState().setActiveWorkspace("ws-1");
    const before = s.getState().workspaces;
    s.getState().setActiveWorkspace("ws-1");
    // Same reference: the slice short-circuits identical updates.
    expect(s.getState().workspaces).toBe(before);
  });
});
