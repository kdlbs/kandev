import { describe, expect, it } from "vitest";
import { create } from "zustand";
import { immer } from "zustand/middleware/immer";
import type { FailedInboxRow } from "@/lib/types/failed-inbox";
import { createFailedInboxSlice } from "./failed-inbox-slice";
import type { FailedInboxSlice } from "./types";
import { createWorkspaceSlice } from "../workspace/workspace-slice";
import type { WorkspaceSlice } from "../workspace/types";

type TestStore = FailedInboxSlice & WorkspaceSlice;

function newStore() {
  // `createFailedInboxSlice` and `createWorkspaceSlice` both type `set`/`get`
  // against the full `AppState` (the zustand slices pattern, so composing
  // them in store.ts needs no cast); this isolated test store only composes
  // these two slices, so the shapes need a cast here, in test-only code the
  // store.ts architecture rule does not cover. `createWorkspaceSlice` is
  // real, not a stub: `beginFailedInboxRead` now reads
  // `workspaces.activeIdRevision`, the app-wide signal a genuine workspace
  // switch bumps regardless of whether the Inbox is mounted to see it, so a
  // test proving that has to drive a real `setActiveWorkspace`.
  return create<TestStore>()(
    immer((set, get, api) => ({
      ...createWorkspaceSlice(
        set as unknown as Parameters<typeof createWorkspaceSlice>[0],
        get as unknown as Parameters<typeof createWorkspaceSlice>[1],
        api as unknown as Parameters<typeof createWorkspaceSlice>[2],
      ),
      ...createFailedInboxSlice(
        set as unknown as Parameters<typeof createFailedInboxSlice>[0],
        get as unknown as Parameters<typeof createFailedInboxSlice>[1],
      ),
    })),
  );
}

function row(overrides: Partial<FailedInboxRow> = {}): FailedInboxRow {
  return {
    task_id: "t1",
    title: "Task",
    workspace_id: "w1",
    origin: "manual",
    failure_instant: "2026-09-14T01:00:00Z",
    reason: "boom",
    ...overrides,
  };
}

describe("failed-inbox slice", () => {
  it("has no rows or count for a workspace before any read (AC .16: absent is not zero)", () => {
    const store = newStore();
    expect(store.getState().failedInbox.byWorkspaceId.w1).toBeUndefined();
  });

  it("applies a page read tagged with the current generation", () => {
    const store = newStore();
    const generation = store.getState().beginFailedInboxRead("w1");

    store.getState().setFailedInboxPage("w1", generation, {
      rows: [row()],
      count: 1,
      truncated: true,
    });

    const state = store.getState().failedInbox.byWorkspaceId.w1;
    expect(state.status).toBe("ready");
    expect(state.count).toBe(1);
    expect(state.rows).toHaveLength(1);
    expect(state.truncated).toBe(true);
  });

  // AC-UI-INBOX-FAILED-001.26: a fast workspace switch or overlapping refresh
  // trigger must not let a slower, older response overwrite a newer one.
  it("drops a page response whose generation was superseded by a newer read", () => {
    const store = newStore();
    const staleGeneration = store.getState().beginFailedInboxRead("w1");
    const freshGeneration = store.getState().beginFailedInboxRead("w1");
    expect(freshGeneration).not.toBe(staleGeneration);

    store.getState().setFailedInboxPage("w1", freshGeneration, {
      rows: [row({ task_id: "fresh" })],
      count: 1,
      truncated: false,
    });
    // The stale response lands after the fresh one and must be dropped.
    store.getState().setFailedInboxPage("w1", staleGeneration, {
      rows: [row({ task_id: "stale" })],
      count: 1,
      truncated: false,
    });

    const state = store.getState().failedInbox.byWorkspaceId.w1;
    expect(state.rows[0].task_id).toBe("fresh");
  });

  it("clears rows and count in the same update on a failed read that follows a successful one", () => {
    const store = newStore();
    const okGeneration = store.getState().beginFailedInboxRead("w1");
    store.getState().setFailedInboxPage("w1", okGeneration, {
      rows: [row()],
      count: 1,
      truncated: false,
    });

    const failGeneration = store.getState().beginFailedInboxRead("w1");
    store.getState().setFailedInboxError("w1", failGeneration);

    const state = store.getState().failedInbox.byWorkspaceId.w1;
    expect(state.status).toBe("error");
    expect(state.rows).toHaveLength(0);
    expect(state.count).toBe(0);
  });

  it("drops a stale error response the same way it drops a stale page response", () => {
    const store = newStore();
    const staleGeneration = store.getState().beginFailedInboxRead("w1");
    const freshGeneration = store.getState().beginFailedInboxRead("w1");
    store.getState().setFailedInboxPage("w1", freshGeneration, {
      rows: [row()],
      count: 1,
      truncated: false,
    });

    store.getState().setFailedInboxError("w1", staleGeneration);

    const state = store.getState().failedInbox.byWorkspaceId.w1;
    expect(state.status).toBe("ready");
    expect(state.count).toBe(1);
  });

  it("keys generations independently per workspace", () => {
    const store = newStore();
    const genW1 = store.getState().beginFailedInboxRead("w1");
    const genW2 = store.getState().beginFailedInboxRead("w2");

    store.getState().setFailedInboxPage("w2", genW2, {
      rows: [row({ workspace_id: "w2" })],
      count: 1,
      truncated: false,
    });
    store.getState().setFailedInboxPage("w1", genW1, {
      rows: [],
      count: 0,
      truncated: false,
    });

    expect(store.getState().failedInbox.byWorkspaceId.w1.status).toBe("ready");
    expect(store.getState().failedInbox.byWorkspaceId.w2.rows).toHaveLength(1);
  });
});

describe("failed-inbox slice: response identity", () => {
  it("drops a response after the active workspace changes", () => {
    const store = newStore();
    store.getState().setActiveWorkspace("w1");
    const generation = store.getState().beginFailedInboxRead("w1");

    store.getState().setActiveWorkspace("w2");
    store.getState().setFailedInboxPage("w1", generation, {
      rows: [row()],
      count: 1,
      truncated: false,
    });

    expect(store.getState().failedInbox.byWorkspaceId.w1.status).toBe("loading");
    expect(store.getState().failedInbox.byWorkspaceId.w1.rows).toHaveLength(0);
  });

  it("drops a response after switching away and back to the same workspace", () => {
    const store = newStore();
    store.getState().setActiveWorkspace("w1");
    const generation = store.getState().beginFailedInboxRead("w1");

    store.getState().setActiveWorkspace("w2");
    store.getState().setActiveWorkspace("w1");
    store.getState().setFailedInboxError("w1", generation);

    expect(store.getState().failedInbox.byWorkspaceId.w1.status).toBe("loading");
    expect(store.getState().failedInbox.byWorkspaceId.w1.rows).toHaveLength(0);
  });
});

describe("failed-inbox slice: workspace-change status transitions (AC .16)", () => {
  // The count "shall remain rendered" across a background refresh -- only a
  // never-read or previously-errored workspace should show as not-yet-known
  // while a request is in flight.
  it("keeps a workspace's status ready while a background refresh of already-loaded data is in flight", () => {
    const store = newStore();
    const firstGeneration = store.getState().beginFailedInboxRead("w1");
    store.getState().setFailedInboxPage("w1", firstGeneration, {
      rows: [row()],
      count: 1,
      truncated: false,
    });

    store.getState().beginFailedInboxRead("w1");

    const state = store.getState().failedInbox.byWorkspaceId.w1;
    expect(state.status).toBe("ready");
    expect(state.rows).toHaveLength(1);
    expect(state.count).toBe(1);
  });

  // "Changing the active workspace shall clear it until a response for the
  // new one is applied" -- returning to a workspace visited earlier in the
  // same session must not show its stale cache. Drives `setActiveWorkspace`
  // (the real signal a switch produces) rather than inferring a switch from
  // the `beginFailedInboxRead` call sequence, since the guard now keys off
  // `workspaces.activeIdRevision`, not a copy local to this slice.
  it("clears a workspace's stale ready state when the operator switches back to it", () => {
    const store = newStore();
    store.getState().setActiveWorkspace("w1");
    const w1Generation = store.getState().beginFailedInboxRead("w1");
    store.getState().setFailedInboxPage("w1", w1Generation, {
      rows: [row()],
      count: 1,
      truncated: false,
    });
    expect(store.getState().failedInbox.byWorkspaceId.w1.status).toBe("ready");

    // Switch away to w2 (Inbox stays mounted and reads it too), then back to
    // w1, before w1's new read resolves.
    store.getState().setActiveWorkspace("w2");
    store.getState().beginFailedInboxRead("w2");
    store.getState().setActiveWorkspace("w1");
    store.getState().beginFailedInboxRead("w1");

    const state = store.getState().failedInbox.byWorkspaceId.w1;
    expect(state.status).not.toBe("ready");
    expect(state.rows).toHaveLength(0);
    expect(state.count).toBe(0);
  });

  // Regression coverage for the gap the switch-back fix above did not close:
  // `beginFailedInboxRead` is only ever called while the Inbox is mounted, so
  // a workspace switch that happens while it is closed must still be caught
  // the next time this workspace is read -- it must not depend on the Inbox
  // having been open (and reading) for every intermediate workspace.
  it("clears a workspace's stale ready state when the switch away and back happens with no read in between", () => {
    const store = newStore();
    store.getState().setActiveWorkspace("w1");
    const w1Generation = store.getState().beginFailedInboxRead("w1");
    store.getState().setFailedInboxPage("w1", w1Generation, {
      rows: [row()],
      count: 1,
      truncated: false,
    });
    expect(store.getState().failedInbox.byWorkspaceId.w1.status).toBe("ready");

    // The active workspace changes twice (e.g. via the sidebar switcher on
    // another page) with the Inbox closed throughout -- nothing here calls
    // `beginFailedInboxRead` for w2 at all.
    store.getState().setActiveWorkspace("w2");
    store.getState().setActiveWorkspace("w1");

    // The Inbox reopens on w1 and its mount effect reads it.
    const state0 = store.getState().failedInbox.byWorkspaceId.w1;
    expect(state0.status).toBe("ready"); // still the stale cache, pre-read
    store.getState().beginFailedInboxRead("w1");

    const state = store.getState().failedInbox.byWorkspaceId.w1;
    expect(state.status).not.toBe("ready");
    expect(state.rows).toHaveLength(0);
    expect(state.count).toBe(0);
  });
});
