import { describe, expect, it } from "vitest";
import { create } from "zustand";
import { immer } from "zustand/middleware/immer";
import { createForgejoSlice } from "./forgejo-slice";
import type { ForgejoSlice } from "./types";

function makeStore() {
  return create<ForgejoSlice>()(immer((...args) => createForgejoSlice(...args)));
}

describe("Forgejo slice", () => {
  it("keeps queue data and errors scoped to their workspace", () => {
    const store = makeStore();
    store.getState().setForgejoQueueState("ws-a", { issues: [], pull_requests: [] });
    store.getState().setForgejoQueueState("ws-b", null, "offline");

    expect(store.getState().forgejoQueue["ws-a"]?.error).toBeNull();
    expect(store.getState().forgejoQueue["ws-b"]?.error).toBe("offline");
  });

  it("clears every cached Forgejo resource when disconnecting a workspace", () => {
    const store = makeStore();
    store.getState().setForgejoConfigState("ws-a", null);
    store.getState().setForgejoIssueWatchesState("ws-a", []);
    store.getState().setForgejoReviewWatchesState("ws-a", []);
    store.getState().setForgejoActionPresetsState("ws-a", []);
    store.getState().setForgejoQueueState("ws-b", { issues: [], pull_requests: [] });

    store.getState().resetForgejoWorkspaceState("ws-a");

    expect(store.getState().forgejoConfig["ws-a"]).toBeUndefined();
    expect(store.getState().forgejoIssueWatches["ws-a"]).toBeUndefined();
    expect(store.getState().forgejoQueue["ws-b"]?.loaded).toBe(true);
  });
});
