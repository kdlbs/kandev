import { describe, it, expect } from "vitest";
import { create } from "zustand";
import { immer } from "zustand/middleware/immer";
import { createSessionSlice } from "./session-slice";
import { createSessionRuntimeSlice } from "../session-runtime/session-runtime-slice";
import type { QueuedMessage, SessionSlice } from "./types";
import type { SessionRuntimeSlice } from "../session-runtime/types";
import { sessionId as toSessionId, taskId as toTaskId } from "@/lib/types/http";

type CombinedSlice = SessionSlice & SessionRuntimeSlice;

function makeStore() {
  return create<CombinedSlice>()(
    immer((set) => ({
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      ...(createSessionSlice as any)(set),
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      ...(createSessionRuntimeSlice as any)(set),
    })),
  );
}

const TASK_ID = toTaskId("task-1");
const SESSION_ID = toSessionId("session-1");
const TS = "2026-04-20T00:00:00Z";

function makeEntry(overrides: Partial<QueuedMessage> = {}): QueuedMessage {
  return {
    id: "entry-1",
    session_id: SESSION_ID,
    task_id: TASK_ID,
    content: "hello",
    plan_mode: false,
    queued_at: TS,
    queued_by: "user",
    ...overrides,
  };
}

describe("queue actions", () => {
  it("setQueueEntries stores the ordered list and capacity meta", () => {
    const store = makeStore();
    const entries = [
      makeEntry({ id: "e1", content: "first" }),
      makeEntry({ id: "e2", content: "second" }),
    ];

    store.getState().setQueueEntries(SESSION_ID, entries, { count: 2, max: 10 });

    expect(store.getState().queue.bySessionId[SESSION_ID]).toEqual(entries);
    expect(store.getState().queue.metaBySessionId[SESSION_ID]).toEqual({ count: 2, max: 10 });
  });

  it("removeQueueEntry drops a single entry by id and refreshes meta.count", () => {
    const store = makeStore();
    const entries = [makeEntry({ id: "e1" }), makeEntry({ id: "e2" }), makeEntry({ id: "e3" })];
    store.getState().setQueueEntries(SESSION_ID, entries, { count: 3, max: 10 });

    store.getState().removeQueueEntry(SESSION_ID, "e2");

    expect(store.getState().queue.bySessionId[SESSION_ID].map((e) => e.id)).toEqual(["e1", "e3"]);
    expect(store.getState().queue.metaBySessionId[SESSION_ID].count).toBe(2);
    expect(store.getState().queue.metaBySessionId[SESSION_ID].max).toBe(10);
  });

  it("removeQueueEntry is a no-op when the session has no entries", () => {
    const store = makeStore();
    store.getState().removeQueueEntry(SESSION_ID, "missing");
    expect(store.getState().queue.bySessionId[SESSION_ID]).toBeUndefined();
  });

  it("clearQueueStatus removes both entries and meta", () => {
    const store = makeStore();
    store.getState().setQueueEntries(SESSION_ID, [makeEntry()], { count: 1, max: 10 });

    store.getState().clearQueueStatus(SESSION_ID);

    expect(store.getState().queue.bySessionId[SESSION_ID]).toBeUndefined();
    expect(store.getState().queue.metaBySessionId[SESSION_ID]).toBeUndefined();
  });
});
