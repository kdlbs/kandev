import { describe, expect, it } from "vitest";
import { create } from "zustand";
import { immer } from "zustand/middleware/immer";
import { createJiraSlice } from "./jira-slice";
import type { JiraSlice } from "./types";
import type { JiraIssueWatch } from "@/lib/types/jira";

function makeStore() {
  return create<JiraSlice>()(immer((set) => createJiraSlice(set)));
}

function watch(id: string, overrides: Partial<JiraIssueWatch> = {}): JiraIssueWatch {
  return {
    id,
    workspaceId: "ws-1",
    workflowId: "wf-1",
    workflowStepId: "step-1",
    repositoryId: "",
    baseBranch: "",
    jql: "project = PROJ",
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

describe("jira issue-watches slice", () => {
  it("starts empty and not loaded", () => {
    const store = makeStore();
    const s = store.getState();
    expect(s.jiraIssueWatches.items).toEqual([]);
    expect(s.jiraIssueWatches.loaded).toBe(false);
    expect(s.jiraIssueWatches.loading).toBe(false);
  });

  it("keeps mutations isolated between stores", () => {
    const firstStore = makeStore();
    const secondStore = makeStore();

    firstStore.getState().addJiraIssueWatch(watch("first-store"));

    expect(firstStore.getState().jiraIssueWatches.items.map((item) => item.id)).toEqual([
      "first-store",
    ]);
    expect(secondStore.getState().jiraIssueWatches).toEqual({
      items: [],
      loaded: false,
      loading: false,
    });
  });

  it("setJiraIssueWatches replaces items and flips loaded=true", () => {
    const store = makeStore();
    store.getState().setJiraIssueWatches([watch("a"), watch("b")]);
    const s = store.getState();
    expect(s.jiraIssueWatches.items.map((w) => w.id)).toEqual(["a", "b"]);
    expect(s.jiraIssueWatches.loaded).toBe(true);
  });

  it("setJiraIssueWatchesLoading toggles loading independently", () => {
    const store = makeStore();
    store.getState().setJiraIssueWatchesLoading(true);
    expect(store.getState().jiraIssueWatches.loading).toBe(true);
    store.getState().setJiraIssueWatchesLoading(false);
    expect(store.getState().jiraIssueWatches.loading).toBe(false);
  });

  it("addJiraIssueWatch appends a new entry", () => {
    const store = makeStore();
    store.getState().setJiraIssueWatches([watch("a")]);
    store.getState().addJiraIssueWatch(watch("b"));
    expect(store.getState().jiraIssueWatches.items.map((w) => w.id)).toEqual(["a", "b"]);
  });

  it("updateJiraIssueWatch replaces in place by id; missing id is a no-op", () => {
    const store = makeStore();
    store.getState().setJiraIssueWatches([watch("a", { jql: "OLD" }), watch("b")]);
    store.getState().updateJiraIssueWatch(watch("a", { jql: "NEW" }));
    expect(store.getState().jiraIssueWatches.items[0].jql).toBe("NEW");

    store.getState().updateJiraIssueWatch(watch("ghost", { jql: "X" }));
    expect(store.getState().jiraIssueWatches.items.map((w) => w.id)).toEqual(["a", "b"]);
  });

  it("removeJiraIssueWatch filters by id", () => {
    const store = makeStore();
    store.getState().setJiraIssueWatches([watch("a"), watch("b"), watch("c")]);
    store.getState().removeJiraIssueWatch("b");
    expect(store.getState().jiraIssueWatches.items.map((w) => w.id)).toEqual(["a", "c"]);
  });

  it("resetJiraIssueWatches clears items AND loaded so a refetch is triggered", () => {
    // The whole point of this action vs. setJiraIssueWatches([]) — an empty
    // setWatches keeps loaded=true and would block the fetch effect from
    // re-running on workspace switch.
    const store = makeStore();
    store.getState().setJiraIssueWatches([watch("a")]);
    store.getState().setJiraIssueWatchesLoading(true);
    expect(store.getState().jiraIssueWatches.loaded).toBe(true);

    store.getState().resetJiraIssueWatches();
    expect(store.getState().jiraIssueWatches.items).toEqual([]);
    expect(store.getState().jiraIssueWatches.loaded).toBe(false);
    expect(store.getState().jiraIssueWatches.loading).toBe(true);
  });
});
