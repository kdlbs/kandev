import { describe, expect, it } from "vitest";
import { create } from "zustand";
import { immer } from "zustand/middleware/immer";

import { createWorkspaceSlice } from "@/lib/state/slices/workspace/workspace-slice";
import type { WorkspaceSlice } from "@/lib/state/slices/workspace/types";
import type { AppState } from "@/lib/state/store";
import type { StoreApi } from "zustand";
import { registerRepositoryBranchPoliciesHandlers } from "./repository-branch-policies";

function makeStore() {
  return create<WorkspaceSlice>()(
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    immer((...a) => ({ ...(createWorkspaceSlice as any)(...a) })),
  );
}

function handlersFor(store: ReturnType<typeof makeStore>) {
  return registerRepositoryBranchPoliciesHandlers(store as unknown as StoreApi<AppState>) as Record<
    string,
    (message: { payload: Record<string, unknown> }) => void
  >;
}

function payload(overrides: Record<string, unknown> = {}) {
  return {
    id: "policy-1",
    repository_id: "repo-1",
    name: "Feature",
    description: "Feature branches",
    base_branch: "develop",
    branch_template: "feature/{title}-{suffix}",
    pull_request_target: "develop",
    created_at: "2026-08-24T10:00:00Z",
    updated_at: "2026-08-24T10:00:00Z",
    ...overrides,
  };
}

describe("repository branch policy WebSocket handlers", () => {
  it("registers create, update, and delete events", () => {
    const handlers = handlersFor(makeStore());
    expect(Object.keys(handlers).sort()).toEqual([
      "repository_branch_policy.created",
      "repository_branch_policy.deleted",
      "repository_branch_policy.updated",
    ]);
  });

  it("upserts and removes a policy in its repository bucket", () => {
    const store = makeStore();
    const handlers = handlersFor(store);

    handlers["repository_branch_policy.created"]({ payload: payload() });
    handlers["repository_branch_policy.updated"]({ payload: payload({ name: "Renamed" }) });
    expect(store.getState().repositoryBranchPolicies.itemsByRepositoryId["repo-1"]?.[0]?.name).toBe(
      "Renamed",
    );

    handlers["repository_branch_policy.deleted"]({
      payload: { id: "policy-1", repository_id: "repo-1" },
    });
    expect(store.getState().repositoryBranchPolicies.itemsByRepositoryId["repo-1"]).toEqual([]);
  });

  it("ignores events without a repository identity", () => {
    const store = makeStore();
    handlersFor(store)["repository_branch_policy.created"]({
      payload: payload({ repository_id: "" }),
    });
    expect(store.getState().repositoryBranchPolicies.itemsByRepositoryId).toEqual({});
  });
});
