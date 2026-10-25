import { describe, expect, it } from "vitest";
import { create } from "zustand";
import { immer } from "zustand/middleware/immer";
import { createWorkspaceSlice } from "./workspace-slice";
import type { WorkspaceSlice } from "./types";
import type { Branch, Repository } from "@/lib/types/http";
import { createAppStore } from "@/lib/state/store";

function makeStore() {
  return create<WorkspaceSlice>()(
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    immer((...a) => ({ ...(createWorkspaceSlice as any)(...a) })),
  );
}

const REPO = "repo-1";
const CREATED_REPOSITORY = "repo-created";
const BRANCHES: Branch[] = [{ name: "main", type: "local" }];
const FETCHED_AT = "2026-04-30T10:00:00Z";

function repository(id: string): Repository {
  return { id, name: id, workspace_id: "ws-1" } as Repository;
}

describe("upsertRepository", () => {
  it("preserves an unloaded cache so a later fetch includes existing repositories", () => {
    const s = makeStore();

    s.getState().upsertRepository("ws-1", repository(CREATED_REPOSITORY));

    expect(s.getState().repositories.loadedByWorkspaceId["ws-1"]).toBeUndefined();
    expect(s.getState().repositories.itemsByWorkspaceId["ws-1"]).toEqual([
      repository(CREATED_REPOSITORY),
    ]);
  });

  it("merges against the current store state without dropping concurrent repositories", () => {
    const s = makeStore();
    s.getState().setRepositories("ws-1", [repository("repo-before")]);
    s.getState().setRepositories("ws-1", [
      repository("repo-before"),
      repository("repo-concurrent"),
    ]);

    s.getState().upsertRepository("ws-1", repository(CREATED_REPOSITORY));

    expect(s.getState().repositories.itemsByWorkspaceId["ws-1"].map((repo) => repo.id)).toEqual([
      "repo-before",
      "repo-concurrent",
      CREATED_REPOSITORY,
    ]);
  });
});

describe("setActiveWorkspace", () => {
  it("resets kanban context before switching to another workspace", () => {
    const store = createAppStore({
      workspaces: { items: [], activeId: "ws-a" },
      workspaceContextGeneration: 4,
      workspaceContextRead: {
        workspaceId: "ws-a",
        generation: 4,
        pending: { workflows: true, repositories: true, steps: true },
        errors: { workflows: "transient", repositories: null, steps: null },
        retryAfterMs: { workflows: 1_000, repositories: null, steps: null },
        requestIds: { workflows: "request-a", repositories: "request-b", steps: "request-c" },
        snapshotPending: true,
        snapshotError: "transient",
        snapshotRetryAfterMs: 1_000,
        snapshotRequestId: "snapshot-a",
        retryVersion: 2,
        retryCycle: 1,
      },
      kanban: {
        workflowId: "workflow-a",
        steps: [],
        tasks: [],
      },
    } as never);

    store.getState().setActiveWorkspace("ws-b");

    expect(store.getState().workspaces.activeId).toBe("ws-b");
    expect(store.getState().workspaceContextGeneration).toBe(5);
    expect(store.getState().workspaceContextRead).toMatchObject({
      workspaceId: null,
      pending: { workflows: false, repositories: false, steps: false },
      snapshotPending: false,
      snapshotError: null,
    });
    expect(store.getState().kanban).toMatchObject({ workflowId: null, steps: [], tasks: [] });
  });
});

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

describe("setRepositoryBranchesFetchError", () => {
  it("records the error without touching cached branches", () => {
    const s = makeStore();
    s.getState().setRepositoryBranches(REPO, BRANCHES, { fetchedAt: FETCHED_AT });
    s.getState().setRepositoryBranchesFetchError(REPO, "network down");
    const state = s.getState().repositoryBranches;
    expect(state.fetchErrorByRepositoryId[REPO]).toBe("network down");
    // Cached branches and fetchedAt are preserved so the dropdown stays usable
    // (stale-while-revalidate semantics).
    expect(state.itemsByRepositoryId[REPO]).toEqual(BRANCHES);
    expect(state.fetchedAtByRepositoryId[REPO]).toBe(FETCHED_AT);
  });

  it("clears the error when called with undefined", () => {
    const s = makeStore();
    s.getState().setRepositoryBranchesFetchError(REPO, "boom");
    s.getState().setRepositoryBranchesFetchError(REPO, undefined);
    expect(s.getState().repositoryBranches.fetchErrorByRepositoryId[REPO]).toBeUndefined();
  });
});
