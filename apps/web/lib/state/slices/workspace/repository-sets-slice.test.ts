import { describe, expect, it } from "vitest";
import { create } from "zustand";
import { immer } from "zustand/middleware/immer";

import { createWorkspaceSlice } from "./workspace-slice";
import type { WorkspaceSlice } from "./types";
import type { RepositorySet } from "@/lib/types/http";
import { repositoryId, workspaceId } from "@/lib/types/ids";

function createStore() {
  return create<WorkspaceSlice>()(
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    immer((...a) => ({ ...(createWorkspaceSlice as any)(...a) })),
  );
}

const FULL_STACK = "Full-stack";

function set(id: string, name: string, repositoryIds: string[]): RepositorySet {
  return {
    id,
    workspace_id: workspaceId("ws-1"),
    name,
    description: "",
    repositories: repositoryIds.map((id, position) => ({
      repository_id: repositoryId(id),
      position,
    })),
    created_at: "2026-08-17T09:00:00Z",
    updated_at: "2026-08-17T09:00:00Z",
  };
}

describe("repositorySets slice", () => {
  it("starts empty and reports a workspace as neither loading nor loaded", () => {
    const store = createStore();
    const state = store.getState();
    expect(state.repositorySets.itemsByWorkspaceId).toEqual({});
    expect(state.repositorySets.loadingByWorkspaceId["ws-1"]).toBeUndefined();
    expect(state.repositorySets.loadedByWorkspaceId["ws-1"]).toBeUndefined();
  });

  it("setRepositorySets stores the list and marks the workspace loaded", () => {
    const store = createStore();
    store.getState().setRepositorySetsLoading("ws-1", true);
    store.getState().setRepositorySets("ws-1", [set("set-1", FULL_STACK, ["repo-web"])]);

    const state = store.getState();
    expect(state.repositorySets.itemsByWorkspaceId["ws-1"]).toHaveLength(1);
    expect(state.repositorySets.loadingByWorkspaceId["ws-1"]).toBe(false);
    expect(state.repositorySets.loadedByWorkspaceId["ws-1"]).toBe(true);
  });

  it("upsertRepositorySet adds a new set and replaces an existing one by id", () => {
    const store = createStore();
    store.getState().setRepositorySets("ws-1", [set("set-1", FULL_STACK, ["repo-web"])]);

    store.getState().upsertRepositorySet("ws-1", set("set-2", "Backend", ["repo-api"]));
    expect(store.getState().repositorySets.itemsByWorkspaceId["ws-1"]).toHaveLength(2);

    store.getState().upsertRepositorySet("ws-1", set("set-1", "Renamed", ["repo-web", "repo-api"]));
    const items = store.getState().repositorySets.itemsByWorkspaceId["ws-1"];
    expect(items).toHaveLength(2);
    const updated = items.find((candidate) => candidate.id === "set-1");
    expect(updated?.name).toBe("Renamed");
    expect(updated?.repositories).toHaveLength(2);
  });

  it("upsertRepositorySet keeps the list name-ordered so a new set is not appended out of place", () => {
    const store = createStore();
    store.getState().setRepositorySets("ws-1", [set("set-1", "Backend", ["repo-api"])]);

    store.getState().upsertRepositorySet("ws-1", set("set-2", "Api gateway", ["repo-gateway"]));

    const names = store
      .getState()
      .repositorySets.itemsByWorkspaceId["ws-1"].map((entry) => entry.name);
    expect(names).toEqual(["Api gateway", "Backend"]);
  });

  it("upsertRepositorySet on an unloaded workspace does not claim it is loaded", () => {
    const store = createStore();
    store.getState().upsertRepositorySet("ws-1", set("set-1", FULL_STACK, ["repo-web"]));

    const state = store.getState();
    expect(state.repositorySets.itemsByWorkspaceId["ws-1"]).toHaveLength(1);
    // A WS event for a workspace never listed must not suppress the initial fetch.
    expect(state.repositorySets.loadedByWorkspaceId["ws-1"]).toBeUndefined();
  });

  it("removeRepositorySet drops one set and leaves the rest", () => {
    const store = createStore();
    store
      .getState()
      .setRepositorySets("ws-1", [
        set("set-1", FULL_STACK, ["repo-web"]),
        set("set-2", "Backend", ["repo-api"]),
      ]);

    store.getState().removeRepositorySet("ws-1", "set-1");

    const items = store.getState().repositorySets.itemsByWorkspaceId["ws-1"];
    expect(items).toHaveLength(1);
    expect(items[0].id).toBe("set-2");
  });

  it("removeRepositorySet for an unknown workspace is a no-op", () => {
    const store = createStore();
    expect(() => store.getState().removeRepositorySet("ws-missing", "set-1")).not.toThrow();
    expect(store.getState().repositorySets.itemsByWorkspaceId["ws-missing"]).toBeUndefined();
  });

  it("invalidateRepositorySets clears the loaded marker so the next read refetches", () => {
    const store = createStore();
    store.getState().setRepositorySets("ws-1", [set("set-1", FULL_STACK, ["repo-web"])]);

    store.getState().invalidateRepositorySets("ws-1");

    expect(store.getState().repositorySets.loadedByWorkspaceId["ws-1"]).toBe(false);
  });

  it("keeps workspaces independent", () => {
    const store = createStore();
    store.getState().setRepositorySets("ws-1", [set("set-1", FULL_STACK, ["repo-web"])]);
    store.getState().setRepositorySets("ws-2", []);

    const state = store.getState();
    expect(state.repositorySets.itemsByWorkspaceId["ws-1"]).toHaveLength(1);
    expect(state.repositorySets.itemsByWorkspaceId["ws-2"]).toHaveLength(0);
    expect(state.repositorySets.loadedByWorkspaceId["ws-2"]).toBe(true);
  });
});
