import { describe, expect, it } from "vitest";
import { create } from "zustand";
import { immer } from "zustand/middleware/immer";
import type { ClarificationInboxBundle } from "@/lib/types/clarification-inbox";
import { createNeedsYouInboxSlice } from "./needs-you-inbox-slice";
import type { NeedsYouInboxSlice } from "./types";

function newStore() {
  // `createNeedsYouInboxSlice` types `set` against the full `AppState` (the
  // zustand slices pattern, so composing it in store.ts needs no cast); this
  // isolated test store only has `NeedsYouInboxSlice`, so the two `set`
  // shapes need a cast here, in test-only code the store.ts architecture
  // rule does not cover.
  return create<NeedsYouInboxSlice>()(
    immer((set) =>
      createNeedsYouInboxSlice(set as unknown as Parameters<typeof createNeedsYouInboxSlice>[0]),
    ),
  );
}

function bundle(overrides: Partial<ClarificationInboxBundle> = {}): ClarificationInboxBundle {
  return {
    pending_id: "p1",
    task_id: "t1",
    session_id: "s1",
    session_state: "WAITING_FOR_INPUT",
    task_title: "Task",
    created_at: "2026-09-03T04:54:02Z",
    context: "",
    messages: [],
    ...overrides,
  };
}

describe("needs-you-inbox slice", () => {
  it("has no rows or count for a workspace before any read (AC .13: absent is not zero)", () => {
    const store = newStore();
    expect(store.getState().needsYouInbox.byWorkspaceId.w1).toBeUndefined();
  });

  it("applies a page read tagged with the current generation", () => {
    const store = newStore();
    const generation = store.getState().beginNeedsYouInboxRead("w1");

    store.getState().setNeedsYouInboxPage("w1", generation, {
      bundles: [bundle()],
      count: 1,
      hiddenCount: 2,
      nextSnoozeExpiry: "2026-09-03T05:00:00Z",
      hasMore: true,
    });

    const state = store.getState().needsYouInbox.byWorkspaceId.w1;
    expect(state.status).toBe("ready");
    expect(state.count).toBe(1);
    expect(state.bundles).toHaveLength(1);
    expect(state.hiddenCount).toBe(2);
    expect(state.hasMore).toBe(true);
    expect(state.nextSnoozeExpiry).toBe("2026-09-03T05:00:00Z");
  });

  // AC .38: a fast workspace switch or overlapping refresh trigger must not
  // let a slower, older response overwrite what a newer one already wrote.
  it("drops a page response whose generation was superseded by a newer read", () => {
    const store = newStore();
    const staleGeneration = store.getState().beginNeedsYouInboxRead("w1");
    const freshGeneration = store.getState().beginNeedsYouInboxRead("w1");
    expect(freshGeneration).not.toBe(staleGeneration);

    store.getState().setNeedsYouInboxPage("w1", freshGeneration, {
      bundles: [bundle({ pending_id: "fresh" })],
      count: 1,
      hiddenCount: 0,
      nextSnoozeExpiry: null,
      hasMore: false,
    });
    // The stale response lands after the fresh one and must be dropped.
    store.getState().setNeedsYouInboxPage("w1", staleGeneration, {
      bundles: [bundle({ pending_id: "stale" })],
      count: 1,
      hiddenCount: 0,
      nextSnoozeExpiry: null,
      hasMore: false,
    });

    const state = store.getState().needsYouInbox.byWorkspaceId.w1;
    expect(state.bundles[0].pending_id).toBe("fresh");
  });

  it("clears rows and count in the same update on a failed read that follows a successful one", () => {
    const store = newStore();
    const okGeneration = store.getState().beginNeedsYouInboxRead("w1");
    store.getState().setNeedsYouInboxPage("w1", okGeneration, {
      bundles: [bundle()],
      count: 1,
      hiddenCount: 0,
      nextSnoozeExpiry: null,
      hasMore: false,
    });

    const failGeneration = store.getState().beginNeedsYouInboxRead("w1");
    store.getState().setNeedsYouInboxError("w1", failGeneration);

    const state = store.getState().needsYouInbox.byWorkspaceId.w1;
    expect(state.status).toBe("error");
    expect(state.bundles).toHaveLength(0);
    expect(state.count).toBe(0);
  });

  it("drops a stale error response the same way it drops a stale page response", () => {
    const store = newStore();
    const staleGeneration = store.getState().beginNeedsYouInboxRead("w1");
    const freshGeneration = store.getState().beginNeedsYouInboxRead("w1");
    store.getState().setNeedsYouInboxPage("w1", freshGeneration, {
      bundles: [bundle()],
      count: 1,
      hiddenCount: 0,
      nextSnoozeExpiry: null,
      hasMore: false,
    });

    store.getState().setNeedsYouInboxError("w1", staleGeneration);

    const state = store.getState().needsYouInbox.byWorkspaceId.w1;
    expect(state.status).toBe("ready");
    expect(state.count).toBe(1);
  });
});

describe("needs-you-inbox slice — boot seeding", () => {
  it("seeds boot state at generation 0 only when no real read has started yet", () => {
    const store = newStore();
    store.getState().seedNeedsYouInboxBoot("w1", {
      count: 3,
      hasMore: false,
      nextSnoozeExpiry: null,
    });

    expect(store.getState().needsYouInbox.byWorkspaceId.w1.count).toBe(3);
  });

  it("lets the first real read supersede a boot seed", () => {
    const store = newStore();
    store.getState().seedNeedsYouInboxBoot("w1", {
      count: 3,
      hasMore: false,
      nextSnoozeExpiry: null,
    });

    const generation = store.getState().beginNeedsYouInboxRead("w1");
    store.getState().setNeedsYouInboxPage("w1", generation, {
      bundles: [],
      count: 0,
      hiddenCount: 0,
      nextSnoozeExpiry: null,
      hasMore: false,
    });

    expect(store.getState().needsYouInbox.byWorkspaceId.w1.count).toBe(0);
    expect(store.getState().needsYouInbox.byWorkspaceId.w1.status).toBe("ready");
  });

  it("never lets a boot seed overwrite a read already in flight", () => {
    const store = newStore();
    store.getState().beginNeedsYouInboxRead("w1");
    store.getState().seedNeedsYouInboxBoot("w1", {
      count: 99,
      hasMore: false,
      nextSnoozeExpiry: null,
    });

    // The seed must not have written anything once a read generation exists.
    expect(store.getState().needsYouInbox.byWorkspaceId.w1.count).toBe(0);
    expect(store.getState().needsYouInbox.byWorkspaceId.w1.status).toBe("loading");
  });
});
