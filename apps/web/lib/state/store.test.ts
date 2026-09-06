import { describe, expect, it } from "vitest";
import { createAppStore, type AppState } from "./store";

describe("createAppStore", () => {
  it("hydrates and replaces the full agent runtime availability snapshot", () => {
    const unavailable = {
      status: "unavailable" as const,
      reason: "agentctl_exited" as const,
      occurred_at: "2026-08-08T14:22:52Z",
    };
    const available = { status: "available" as const };
    const store = createAppStore({ agentRuntime: unavailable });

    expect(store.getState().agentRuntime).toEqual(unavailable);

    store.getState().setAgentRuntime(available);

    expect(store.getState().agentRuntime).toEqual(available);
  });

  it("retains sidebar boot settings after slice initialization", () => {
    const store = createAppStore({
      userSettings: {
        sidebarViews: [
          {
            id: "server",
            name: "Server",
            filters: [],
            sort: { key: "state", direction: "asc" },
            group: "none",
            collapsedGroups: [],
          },
        ],
        sidebarActiveViewId: "server",
        sidebarTaskPrefs: {
          pinnedTaskIds: ["task-1"],
          orderedTaskIds: ["task-1"],
          subtaskOrderByParentId: {},
        },
        loaded: true,
      },
    } as unknown as Partial<AppState>);

    expect(store.getState().sidebarViews.activeViewId).toBe("server");
    expect(store.getState().sidebarTaskPrefs.pinnedTaskIds).toEqual(["task-1"]);
  });

  it("retains Threads boot settings after UI slice initialization", () => {
    const store = createAppStore({
      userSettings: {
        threadViews: [
          {
            id: "capped",
            name: "Capped",
            taskScope: { mode: "all", taskIds: [] },
            filters: [],
            sort: { key: "title", direction: "asc" },
            maxColumns: 1,
          },
        ],
        threadActiveViewId: "capped",
        threadViewDraft: null,
        loaded: true,
      },
    } as unknown as Partial<AppState>);

    expect(store.getState().threadViews.views).toHaveLength(1);
    expect(store.getState().threadViews.activeViewId).toBe("capped");
    expect(store.getState().threadViews.views[0]?.maxColumns).toBe(1);
  });

  it("starts with no task-scoped Review PR overrides", () => {
    const store = createAppStore();

    expect(store.getState().reviewPRSelection).toEqual({ selectedKeyByTaskId: {} });
  });

  it("retains caller-supplied task-scoped Review PR overrides", () => {
    const store = createAppStore({
      reviewPRSelection: {
        selectedKeyByTaskId: { "task-1": "acme/widget/7" },
      },
    } as Partial<AppState>);

    expect(store.getState().reviewPRSelection.selectedKeyByTaskId).toEqual({
      "task-1": "acme/widget/7",
    });
  });

  it("stores Review PR overrides independently by task", () => {
    const store = createAppStore();
    const state = store.getState();

    state.setReviewPRSelection("task-1", "acme/widget/1");
    state.setReviewPRSelection("task-2", "acme/widget/2");

    expect(store.getState().reviewPRSelection.selectedKeyByTaskId).toEqual({
      "task-1": "acme/widget/1",
      "task-2": "acme/widget/2",
    });
  });

  it("starts with no connection issue severity", () => {
    const store = createAppStore();

    expect((store.getState().connection as { issueSeverity?: string }).issueSeverity).toBe("none");
  });
});

describe("automation run delete serialization", () => {
  it("claims the delete slot once and releases it only for the matching generation", () => {
    const store = createAppStore();

    // Absent key means idle: the first begin bumps the epoch and claims gen 1.
    expect(store.getState().beginAutomationRunDelete("auto-1")).toBe(1);
    expect(store.getState().automationRuns.mutationEpoch["auto-1"]).toBe(1);
    expect(store.getState().automationRuns.deleting["auto-1"]).toBe(1);

    // While active, further claims are rejected.
    expect(store.getState().beginAutomationRunDelete("auto-1")).toBeNull();

    // Stale generations cannot release the slot.
    store.getState().endAutomationRunDelete("auto-1", 0);
    expect(store.getState().automationRuns.deleting["auto-1"]).toBe(1);
    store.getState().endAutomationRunDelete("auto-1", 99);
    expect(store.getState().automationRuns.deleting["auto-1"]).toBe(1);

    // The matching generation releases it.
    store.getState().endAutomationRunDelete("auto-1", 1);
    expect(store.getState().automationRuns.deleting["auto-1"]).toBe(false);

    // A stale end from an unmounted instance cannot clear a newer slot.
    expect(store.getState().beginAutomationRunDelete("auto-1")).toBe(2);
    store.getState().endAutomationRunDelete("auto-1", 1); // stale
    expect(store.getState().automationRuns.deleting["auto-1"]).toBe(2);
    store.getState().endAutomationRunDelete("auto-1", 2);
    expect(store.getState().automationRuns.deleting["auto-1"]).toBe(false);
  });

  it("advances the epoch without touching the delete slot", () => {
    const store = createAppStore();

    expect(store.getState().beginAutomationRunDelete("auto-1")).toBe(1);
    store.getState().advanceAutomationRunEpoch("auto-1");
    expect(store.getState().automationRuns.mutationEpoch["auto-1"]).toBe(2);
    // The slot is untouched and still holds its own generation.
    expect(store.getState().automationRuns.deleting["auto-1"]).toBe(1);
    store.getState().endAutomationRunDelete("auto-1", 1);
    expect(store.getState().automationRuns.deleting["auto-1"]).toBe(false);

    // A different automation has its own independent counter.
    expect(store.getState().beginAutomationRunDelete("auto-2")).toBe(1);
  });
});
