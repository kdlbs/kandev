import { describe, expect, it } from "vitest";
import { create } from "zustand";
import { immer } from "zustand/middleware/immer";
import { createSessionSlice } from "@/lib/state/slices/session/session-slice";
import type { SessionSlice } from "@/lib/state/slices/session/types";
import type { TurnChangeSetSummary } from "@/lib/types/turn-changes";
import { registerTurnChangesHandlers } from "./turn-changes";

function makeStore() {
  return create<SessionSlice>()(
    immer((set) => ({
      ...(createSessionSlice as unknown as (storeSet: typeof set) => SessionSlice)(set),
    })),
  );
}

describe("session.turn.changes.updated", () => {
  it("merges only the compact session summary and ignores stale revisions", () => {
    const store = makeStore();
    const handler = registerTurnChangesHandlers(store as never)["session.turn.changes.updated"]!;
    const summary = {
      id: "set-1",
      task_id: "task-1",
      session_id: "session-1",
      turn_id: "turn-1",
      revision: 2,
      availability: "ready",
      complete: true,
      summary_complete: true,
      content_complete: true,
      turn_ordinal: 1,
      fallback_anchor: "turn-changes:turn-1",
      file_count: 1,
      binary_file_count: 0,
      unknown_count_file_count: 0,
      repository_count: 0,
      repositories: [],
    } as TurnChangeSetSummary;
    const message = (revision: number) =>
      ({ payload: { session_id: "session-1", change_set: { ...summary, revision } } }) as never;

    handler(message(2));
    handler(message(1));

    expect(store.getState().turnChanges.bySession["session-1"][0].revision).toBe(2);
  });
});
