import { describe, expect, it } from "vitest";
import { create } from "zustand";
import { immer } from "zustand/middleware/immer";
import type { InboxHistoryBundle } from "@/lib/types/inbox-history";
import { createInboxHistorySlice } from "./inbox-history-slice";
import type { InboxHistorySlice } from "./types";

function newStore() {
  // createInboxHistorySlice types `set` against the full AppState (the
  // zustand slices pattern, so composing it in store.ts needs no cast); this
  // isolated test store only has InboxHistorySlice, so the two `set` shapes
  // need a cast here, in test-only code the store.ts architecture rule does
  // not cover.
  return create<InboxHistorySlice>()(
    immer((set) =>
      createInboxHistorySlice(set as unknown as Parameters<typeof createInboxHistorySlice>[0]),
    ),
  );
}

function bundle(overrides: Partial<InboxHistoryBundle> = {}): InboxHistoryBundle {
  return {
    pending_id: "p1",
    task_id: "t1",
    session_id: "s1",
    session_state: "COMPLETED",
    task_title: "Task",
    created_at: "2026-09-03T04:54:02Z",
    context: "",
    messages: [],
    kind: "clarification",
    reason: "session_ended",
    asking_turn_id: "turn-1",
    ...overrides,
  };
}

describe("inbox-history slice", () => {
  it("has no rows or total for a workspace before any read", () => {
    const store = newStore();
    expect(store.getState().inboxHistory.byWorkspaceId.w1).toBeUndefined();
  });

  it("applies a page read tagged with the current generation", () => {
    const store = newStore();
    const generation = store.getState().beginInboxHistoryRead("w1");

    store.getState().setInboxHistoryPage("w1", generation, {
      bundles: [bundle()],
      total: 1,
      hasMore: true,
    });

    const state = store.getState().inboxHistory.byWorkspaceId.w1;
    expect(state.status).toBe("ready");
    expect(state.total).toBe(1);
    expect(state.bundles).toHaveLength(1);
    expect(state.hasMore).toBe(true);
  });

  // AC .23 (mirrors AC-UI-NEEDS-YOU-INBOX-001.38): a fast workspace switch or
  // overlapping refresh must not let a slower, older response overwrite what
  // a newer one already wrote.
  it("drops a page response whose generation was superseded by a newer read", () => {
    const store = newStore();
    const staleGeneration = store.getState().beginInboxHistoryRead("w1");
    const freshGeneration = store.getState().beginInboxHistoryRead("w1");
    expect(freshGeneration).not.toBe(staleGeneration);

    store.getState().setInboxHistoryPage("w1", freshGeneration, {
      bundles: [bundle({ pending_id: "fresh" })],
      total: 1,
      hasMore: false,
    });
    store.getState().setInboxHistoryPage("w1", staleGeneration, {
      bundles: [bundle({ pending_id: "stale" })],
      total: 1,
      hasMore: false,
    });

    const state = store.getState().inboxHistory.byWorkspaceId.w1;
    expect(state.bundles[0].pending_id).toBe("fresh");
  });

  it("clears rows and total in the same update on a failed read that follows a successful one", () => {
    const store = newStore();
    const okGeneration = store.getState().beginInboxHistoryRead("w1");
    store.getState().setInboxHistoryPage("w1", okGeneration, {
      bundles: [bundle()],
      total: 1,
      hasMore: false,
    });

    const failGeneration = store.getState().beginInboxHistoryRead("w1");
    store.getState().setInboxHistoryError("w1", failGeneration);

    const state = store.getState().inboxHistory.byWorkspaceId.w1;
    expect(state.status).toBe("error");
    expect(state.bundles).toHaveLength(0);
    expect(state.total).toBe(0);
  });

  it("drops a stale error response the same way it drops a stale page response", () => {
    const store = newStore();
    const staleGeneration = store.getState().beginInboxHistoryRead("w1");
    const freshGeneration = store.getState().beginInboxHistoryRead("w1");
    store.getState().setInboxHistoryPage("w1", freshGeneration, {
      bundles: [bundle()],
      total: 1,
      hasMore: false,
    });

    store.getState().setInboxHistoryError("w1", staleGeneration);

    const state = store.getState().inboxHistory.byWorkspaceId.w1;
    expect(state.status).toBe("ready");
    expect(state.total).toBe(1);
  });
});
