import { describe, expect, it } from "vitest";
import { create } from "zustand";
import { immer } from "zustand/middleware/immer";
import { createLinearSlice } from "./linear-slice";
import type { LinearSlice } from "./types";
import type { LinearIssueWatch } from "@/lib/types/linear";

function makeStore() {
  return create<LinearSlice>()(
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    immer((...a) => ({ ...(createLinearSlice as any)(...a) })),
  );
}

function watch(id: string, overrides: Partial<LinearIssueWatch> = {}): LinearIssueWatch {
  return {
    id,
    workspaceId: "ws-1",
    workflowId: "wf-1",
    workflowStepId: "step-1",
    filter: { teamKey: "ENG" },
    agentProfileId: "",
    executorProfileId: "",
    prompt: "",
    enabled: true,
    pollIntervalSeconds: 300,
    createdAt: "2026-01-01T00:00:00Z",
    updatedAt: "2026-01-01T00:00:00Z",
    ...overrides,
  };
}

describe("linear issue-watches slice", () => {
  it("starts empty and not loaded", () => {
    const store = makeStore();
    const s = store.getState();
    expect(s.linearIssueWatches.items).toEqual([]);
    expect(s.linearIssueWatches.loaded).toBe(false);
    expect(s.linearIssueWatches.loading).toBe(false);
  });

  it("setLinearIssueWatches replaces items and flips loaded=true", () => {
    const store = makeStore();
    store.getState().setLinearIssueWatches([watch("a"), watch("b")]);
    const s = store.getState();
    expect(s.linearIssueWatches.items.map((w) => w.id)).toEqual(["a", "b"]);
    expect(s.linearIssueWatches.loaded).toBe(true);
  });

  it("setLinearIssueWatchesLoading toggles loading independently", () => {
    const store = makeStore();
    store.getState().setLinearIssueWatchesLoading(true);
    expect(store.getState().linearIssueWatches.loading).toBe(true);
    store.getState().setLinearIssueWatchesLoading(false);
    expect(store.getState().linearIssueWatches.loading).toBe(false);
  });

  it("addLinearIssueWatch appends a new entry", () => {
    const store = makeStore();
    store.getState().setLinearIssueWatches([watch("a")]);
    store.getState().addLinearIssueWatch(watch("b"));
    expect(store.getState().linearIssueWatches.items.map((w) => w.id)).toEqual(["a", "b"]);
  });

  it("updateLinearIssueWatch replaces in place by id; missing id is a no-op", () => {
    const store = makeStore();
    store
      .getState()
      .setLinearIssueWatches([watch("a", { filter: { teamKey: "OLD" } }), watch("b")]);
    store.getState().updateLinearIssueWatch(watch("a", { filter: { teamKey: "NEW" } }));
    expect(store.getState().linearIssueWatches.items[0].filter.teamKey).toBe("NEW");

    store.getState().updateLinearIssueWatch(watch("ghost", { filter: { teamKey: "X" } }));
    expect(store.getState().linearIssueWatches.items.map((w) => w.id)).toEqual(["a", "b"]);
  });

  it("removeLinearIssueWatch filters by id", () => {
    const store = makeStore();
    store.getState().setLinearIssueWatches([watch("a"), watch("b"), watch("c")]);
    store.getState().removeLinearIssueWatch("b");
    expect(store.getState().linearIssueWatches.items.map((w) => w.id)).toEqual(["a", "c"]);
  });

  it("resetLinearIssueWatches clears items AND loaded so a refetch is triggered", () => {
    // The whole point of this action vs. setLinearIssueWatches([]) — an empty
    // setWatches keeps loaded=true and would block the fetch effect from
    // re-running on workspace switch.
    const store = makeStore();
    store.getState().setLinearIssueWatches([watch("a")]);
    expect(store.getState().linearIssueWatches.loaded).toBe(true);

    store.getState().resetLinearIssueWatches();
    expect(store.getState().linearIssueWatches.items).toEqual([]);
    expect(store.getState().linearIssueWatches.loaded).toBe(false);
  });
});
