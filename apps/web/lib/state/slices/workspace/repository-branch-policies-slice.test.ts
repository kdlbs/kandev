import { describe, expect, it } from "vitest";
import { create } from "zustand";
import { immer } from "zustand/middleware/immer";

import { createWorkspaceSlice } from "./workspace-slice";
import type { WorkspaceSlice } from "./types";
import type { RepositoryBranchPolicy } from "@/lib/types/http";
import { repositoryId } from "@/lib/types/ids";

function createStore() {
  return create<WorkspaceSlice>()(
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    immer((...a) => ({ ...(createWorkspaceSlice as any)(...a) })),
  );
}

function policy(id: string, name: string, repository = "repo-1"): RepositoryBranchPolicy {
  return {
    id,
    repository_id: repositoryId(repository),
    name,
    description: "",
    base_branch: "develop",
    branch_template: "feature/{title}-{suffix}",
    pull_request_target: "develop",
    created_at: "2026-08-24T10:00:00Z",
    updated_at: "2026-08-24T10:00:00Z",
  };
}

describe("repositoryBranchPolicies slice", () => {
  it("sorts policies by name and marks a repository loaded", () => {
    const store = createStore();
    store
      .getState()
      .setRepositoryBranchPolicies("repo-1", [policy("2", "hotfix"), policy("1", "Feature")]);

    expect(
      store
        .getState()
        .repositoryBranchPolicies.itemsByRepositoryId["repo-1"]?.map((item) => item.name),
    ).toEqual(["Feature", "hotfix"]);
    expect(store.getState().repositoryBranchPolicies.loadedByRepositoryId["repo-1"]).toBe(true);
    expect(store.getState().repositoryBranchPolicies.loadingByRepositoryId["repo-1"]).toBe(false);
  });

  it("rejects a list response captured before a policy event", () => {
    const store = createStore();
    const capturedRevision =
      store.getState().repositoryBranchPolicies.revisionByRepositoryId["repo-1"] ?? 0;

    store.getState().upsertRepositoryBranchPolicy(policy("new", "New policy"));
    store.getState().setRepositoryBranchPolicies("repo-1", [], capturedRevision);

    expect(
      store
        .getState()
        .repositoryBranchPolicies.itemsByRepositoryId["repo-1"]?.map((item) => item.id),
    ).toEqual(["new"]);
  });

  it("removes a policy without invalidating the loaded marker", () => {
    const store = createStore();
    store.getState().setRepositoryBranchPolicies("repo-1", [policy("1", "Feature")]);

    store.getState().removeRepositoryBranchPolicy("repo-1", "1");

    expect(store.getState().repositoryBranchPolicies.itemsByRepositoryId["repo-1"]).toEqual([]);
    expect(store.getState().repositoryBranchPolicies.loadedByRepositoryId["repo-1"]).toBe(true);
  });

  it("bumps the revision when an event removes a policy before the first list response", () => {
    const store = createStore();
    const capturedRevision =
      store.getState().repositoryBranchPolicies.revisionByRepositoryId["repo-1"] ?? 0;

    store.getState().removeRepositoryBranchPolicy("repo-1", "deleted-policy");
    store
      .getState()
      .setRepositoryBranchPolicies("repo-1", [policy("deleted-policy", "Stale")], capturedRevision);

    expect(store.getState().repositoryBranchPolicies.itemsByRepositoryId["repo-1"]).toBeUndefined();
    expect(store.getState().repositoryBranchPolicies.revisionByRepositoryId["repo-1"]).toBe(1);
  });
});
