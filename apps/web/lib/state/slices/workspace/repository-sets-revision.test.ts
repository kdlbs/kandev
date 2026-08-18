import { describe, expect, it } from "vitest";
import { create } from "zustand";
import { immer } from "zustand/middleware/immer";

import { createWorkspaceSlice } from "./workspace-slice";
import type { WorkspaceSlice } from "./types";
import type { RepositorySet } from "@/lib/types/http";
import { repositoryId, workspaceId } from "@/lib/types/ids";

const WS = "ws-1";
const FULL_STACK = "Full-stack";

function createStore() {
  return create<WorkspaceSlice>()(
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    immer((...a) => ({ ...(createWorkspaceSlice as any)(...a) })),
  );
}

function set(id: string, name: string): RepositorySet {
  return {
    id,
    workspace_id: workspaceId(WS),
    name,
    description: "",
    repositories: [{ repository_id: repositoryId("repo-web"), position: 0 }],
    created_at: "2026-08-17T09:00:00Z",
    updated_at: "2026-08-17T09:00:00Z",
  };
}

function idsIn(store: ReturnType<typeof createStore>) {
  return (store.getState().repositorySets.itemsByWorkspaceId[WS] ?? []).map((entry) => entry.id);
}

function revision(store: ReturnType<typeof createStore>) {
  return store.getState().repositorySets.revisionByWorkspaceId[WS] ?? 0;
}

// A list response captured before a WebSocket event must not be applied after
// it: the response replaces the whole collection, so it would erase the event.
describe("repositorySets stale-response guard", () => {
  it("starts every workspace at revision zero", () => {
    expect(revision(createStore())).toBe(0);
  });

  it("bumps the revision on an upsert and on a removal", () => {
    const store = createStore();
    store.getState().upsertRepositorySet(WS, set("set-1", FULL_STACK));
    expect(revision(store)).toBe(1);

    store.getState().removeRepositorySet(WS, "set-1");
    expect(revision(store)).toBe(2);
  });

  it("applies a list response captured at the current revision", () => {
    const store = createStore();
    const captured = revision(store);

    store.getState().setRepositorySets(WS, [set("set-1", FULL_STACK)], captured);

    expect(idsIn(store)).toEqual(["set-1"]);
    expect(store.getState().repositorySets.loadedByWorkspaceId[WS]).toBe(true);
  });

  it("drops a list response that a create event overtook", () => {
    const store = createStore();
    const captured = revision(store);

    // The event lands while the request is in flight.
    store.getState().upsertRepositorySet(WS, set("set-new", "Created meanwhile"));
    // The older response resolves last and does not know about it.
    store.getState().setRepositorySets(WS, [], captured);

    expect(idsIn(store)).toEqual(["set-new"]);
  });

  it("drops a list response that a delete event overtook", () => {
    const store = createStore();
    store.getState().setRepositorySets(WS, [set("set-1", FULL_STACK)]);
    const captured = revision(store);

    store.getState().removeRepositorySet(WS, "set-1");
    // A response captured before the delete would resurrect the set.
    store.getState().setRepositorySets(WS, [set("set-1", FULL_STACK)], captured);

    expect(idsIn(store)).toEqual([]);
  });

  it("applies a response captured after the event", () => {
    const store = createStore();
    store.getState().upsertRepositorySet(WS, set("set-new", "Created meanwhile"));
    const captured = revision(store);

    store.getState().setRepositorySets(WS, [set("set-1", "From the server")], captured);

    expect(idsIn(store)).toEqual(["set-1"]);
  });

  it("applies a response with no expected revision, for callers that cannot race", () => {
    // Boot hydration writes the slice directly and has no in-flight request to
    // be overtaken.
    const store = createStore();
    store.getState().upsertRepositorySet(WS, set("set-new", "Created meanwhile"));

    store.getState().setRepositorySets(WS, [set("set-1", "Hydrated")]);

    expect(idsIn(store)).toEqual(["set-1"]);
  });

  it("keeps revisions independent per workspace", () => {
    const store = createStore();
    store.getState().upsertRepositorySet("ws-other", set("set-other", "Other"));

    // ws-1 never moved, so its captured revision is still valid.
    store.getState().setRepositorySets(WS, [set("set-1", FULL_STACK)], 0);

    expect(idsIn(store)).toEqual(["set-1"]);
  });
});
