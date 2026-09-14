import { describe, expect, it } from "vitest";
import { create } from "zustand";
import { immer } from "zustand/middleware/immer";
import type { FailedInboxRow } from "@/lib/types/failed-inbox";
import { createFailedInboxSlice } from "./failed-inbox-slice";
import type { FailedInboxSlice } from "./types";

function newStore() {
  // `createFailedInboxSlice` types `set` against the full `AppState` (the
  // zustand slices pattern, so composing it in store.ts needs no cast); this
  // isolated test store only has `FailedInboxSlice`, so the two `set` shapes
  // need a cast here, in test-only code the store.ts architecture rule does
  // not cover.
  return create<FailedInboxSlice>()(
    immer((set) =>
      createFailedInboxSlice(set as unknown as Parameters<typeof createFailedInboxSlice>[0]),
    ),
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
