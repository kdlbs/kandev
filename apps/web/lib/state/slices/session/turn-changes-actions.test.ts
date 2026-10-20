import { describe, expect, it } from "vitest";
import { create } from "zustand";
import { immer } from "zustand/middleware/immer";
import { createSessionSlice } from "./session-slice";
import type { SessionSlice } from "./types";
import type { TurnChangeSetSummary } from "@/lib/types/turn-changes";

function makeStore() {
  return create<SessionSlice>()(
    immer((set) => ({
      ...(createSessionSlice as unknown as (storeSet: typeof set) => SessionSlice)(set),
    })),
  );
}

function summary(id: string, revision: number, ordinal: number): TurnChangeSetSummary {
  return {
    id,
    task_id: "task-1",
    session_id: "session-1",
    turn_id: `turn-${id}`,
    revision,
    availability: "ready",
    complete: true,
    summary_complete: true,
    content_complete: true,
    turn_ordinal: ordinal,
    fallback_anchor: `turn-changes:${id}`,
    file_count: 1,
    binary_file_count: 0,
    unknown_count_file_count: 0,
    repository_count: 0,
    repositories: [],
  };
}

describe("turn change summary reconciliation", () => {
  it("orders turns and rejects an older paged snapshot after a live revision", () => {
    const store = makeStore();
    store.getState().mergeTurnChangeSummary("session-1", summary("later", 4, 2));
    store
      .getState()
      .mergeTurnChangePage("session-1", [summary("later", 2, 2), summary("first", 1, 1)], {
        offset: 0,
        nextOffset: 2,
      });

    expect(store.getState().turnChanges.bySession["session-1"].map((item) => item.id)).toEqual([
      "first",
      "later",
    ]);
    expect(store.getState().turnChanges.bySession["session-1"][1].revision).toBe(4);
    expect(store.getState().turnChanges.nextOffsetBySession["session-1"]).toBe(2);
  });
});
