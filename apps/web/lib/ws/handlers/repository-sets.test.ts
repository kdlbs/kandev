import { describe, expect, it } from "vitest";
import { create } from "zustand";
import { immer } from "zustand/middleware/immer";

import { createWorkspaceSlice } from "@/lib/state/slices/workspace/workspace-slice";
import type { WorkspaceSlice } from "@/lib/state/slices/workspace/types";
import { registerRepositorySetsHandlers } from "./repository-sets";

function makeStore() {
  return create<WorkspaceSlice>()(
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    immer((...a) => ({ ...(createWorkspaceSlice as any)(...a) })),
  );
}

function payload(overrides: Record<string, unknown> = {}) {
  return {
    id: "set-1",
    workspace_id: "ws-1",
    name: "Full-stack",
    description: "web + gateway",
    repositories: [
      { repository_id: "repo-web", position: 0 },
      { repository_id: "repo-gateway", position: 1, base_branch: "develop" },
    ],
    created_at: "2026-08-17T09:00:00Z",
    updated_at: "2026-08-17T09:00:00Z",
    ...overrides,
  };
}

// eslint-disable-next-line @typescript-eslint/no-explicit-any
function handlersFor(store: any) {
  return registerRepositorySetsHandlers(store) as Record<
    string,
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    (message: { payload: any }) => void
  >;
}

function message(body: Record<string, unknown>) {
  return { payload: body };
}

describe("repository_set WebSocket handlers", () => {
  it("registers exactly the three repository_set events", () => {
    const store = makeStore();
    expect(Object.keys(handlersFor(store)).sort()).toEqual([
      "repository_set.created",
      "repository_set.deleted",
      "repository_set.updated",
    ]);
  });

  it("created adds the set to its workspace", () => {
    const store = makeStore();
    handlersFor(store)["repository_set.created"](message(payload()));

    const sets = store.getState().repositorySets.itemsByWorkspaceId["ws-1"];
    expect(sets).toHaveLength(1);
    expect(sets[0].name).toBe("Full-stack");
    expect(sets[0].repositories.map((entry) => entry.repository_id)).toEqual([
      "repo-web",
      "repo-gateway",
    ]);
    expect(sets[0].repositories[1].base_branch).toBe("develop");
  });

  it("updated replaces the existing set rather than adding a second copy", () => {
    const store = makeStore();
    const handlers = handlersFor(store);
    handlers["repository_set.created"](message(payload()));

    handlers["repository_set.updated"](
      message(
        payload({ name: "Renamed", repositories: [{ repository_id: "repo-web", position: 0 }] }),
      ),
    );

    const sets = store.getState().repositorySets.itemsByWorkspaceId["ws-1"];
    expect(sets).toHaveLength(1);
    expect(sets[0].name).toBe("Renamed");
    expect(sets[0].repositories).toHaveLength(1);
  });

  it("deleted removes only the named set", () => {
    const store = makeStore();
    const handlers = handlersFor(store);
    handlers["repository_set.created"](message(payload()));
    handlers["repository_set.created"](message(payload({ id: "set-2", name: "Backend" })));

    handlers["repository_set.deleted"](message({ id: "set-1", workspace_id: "ws-1" }));

    const sets = store.getState().repositorySets.itemsByWorkspaceId["ws-1"];
    expect(sets).toHaveLength(1);
    expect(sets[0].id).toBe("set-2");
  });

  it("ignores a payload with no workspace id rather than writing an undefined key", () => {
    const store = makeStore();
    handlersFor(store)["repository_set.created"](message(payload({ workspace_id: undefined })));

    expect(store.getState().repositorySets.itemsByWorkspaceId).toEqual({});
  });

  it("normalizes a missing repositories list to an empty array", () => {
    const store = makeStore();
    handlersFor(store)["repository_set.created"](message(payload({ repositories: undefined })));

    const sets = store.getState().repositorySets.itemsByWorkspaceId["ws-1"];
    expect(sets[0].repositories).toEqual([]);
  });

  it("keeps workspaces isolated", () => {
    const store = makeStore();
    const handlers = handlersFor(store);
    handlers["repository_set.created"](message(payload()));
    handlers["repository_set.created"](
      message(payload({ id: "set-other", workspace_id: "ws-2", name: "Other" })),
    );

    const state = store.getState().repositorySets.itemsByWorkspaceId;
    expect(state["ws-1"]).toHaveLength(1);
    expect(state["ws-2"]).toHaveLength(1);
  });
});
