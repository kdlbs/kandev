import { describe, expect, it } from "vitest";
import { create } from "zustand";
import { immer } from "zustand/middleware/immer";
import { createWorkspaceSlice } from "./workspace-slice";
import type { WorkspaceSlice } from "./types";
import type { Branch } from "@/lib/types/http";

function makeStore() {
  return create<WorkspaceSlice>()(
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    immer((...a) => ({ ...(createWorkspaceSlice as any)(...a) })),
  );
}

const REPO = "repo-1";
const BRANCHES: Branch[] = [{ name: "main", type: "local" }];
const FETCHED_AT = "2026-04-30T10:00:00Z";

describe("setRepositoryBranches", () => {
  it("stores branches and marks loaded without meta", () => {
    const s = makeStore();
    s.getState().setRepositoryBranches(REPO, BRANCHES);
    const state = s.getState().repositoryBranches;
    expect(state.itemsByRepositoryId[REPO]).toEqual(BRANCHES);
    expect(state.loadedByRepositoryId[REPO]).toBe(true);
    expect(state.loadingByRepositoryId[REPO]).toBe(false);
    expect(state.fetchedAtByRepositoryId[REPO]).toBeUndefined();
    expect(state.fetchErrorByRepositoryId[REPO]).toBeUndefined();
  });

  it("records fetchedAt + clears prior fetchError on success", () => {
    const s = makeStore();
    s.getState().setRepositoryBranches(REPO, BRANCHES, { fetchError: "boom" });
    expect(s.getState().repositoryBranches.fetchErrorByRepositoryId[REPO]).toBe("boom");
    s.getState().setRepositoryBranches(REPO, BRANCHES, { fetchedAt: FETCHED_AT });
    const state = s.getState().repositoryBranches;
    expect(state.fetchedAtByRepositoryId[REPO]).toBe(FETCHED_AT);
    expect(state.fetchErrorByRepositoryId[REPO]).toBeUndefined();
  });

  it("preserves the prior fetchedAt when meta omits it", () => {
    const s = makeStore();
    s.getState().setRepositoryBranches(REPO, BRANCHES, { fetchedAt: FETCHED_AT });
    s.getState().setRepositoryBranches(REPO, BRANCHES);
    expect(s.getState().repositoryBranches.fetchedAtByRepositoryId[REPO]).toBe(FETCHED_AT);
  });
});

describe("setRepositoryBranchesLoading", () => {
  it("toggles only the loading flag", () => {
    const s = makeStore();
    s.getState().setRepositoryBranchesLoading(REPO, true);
    expect(s.getState().repositoryBranches.loadingByRepositoryId[REPO]).toBe(true);
    s.getState().setRepositoryBranchesLoading(REPO, false);
    expect(s.getState().repositoryBranches.loadingByRepositoryId[REPO]).toBe(false);
  });
});
