import { describe, it, expect } from "vitest";
import { create } from "zustand";
import { immer } from "zustand/middleware/immer";
import { createSessionSlice } from "./session-slice";
import { createSessionRuntimeSlice } from "../session-runtime/session-runtime-slice";
import type { SessionSlice } from "./types";
import type { SessionRuntimeSlice } from "../session-runtime/types";
import { sessionId as toSessionId, taskId as toTaskId, type TaskSession } from "@/lib/types/http";

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

function makeSession(overrides: Partial<TaskSession> = {}): TaskSession {
  return {
    id: SESSION_ID,
    task_id: TASK_ID,
    state: "RUNNING",
    started_at: TS,
    updated_at: TS,
    ...overrides,
  };
}

// updateSessionReadCursor exists specifically so the mark-read HTTP response
// — a full-session snapshot frozen at request time — never overwrites a
// field (e.g. state) that a concurrent WebSocket update wrote to the store
// while the request was still in flight. It must touch ONLY
// last_read_message_id.
describe("updateSessionReadCursor", () => {
  it("updates only last_read_message_id, leaving every other field untouched", () => {
    const store = makeStore();
    store.setState((draft) => {
      draft.taskSessions.items[SESSION_ID] = makeSession({ last_read_message_id: "m1" });
    });

    store.getState().updateSessionReadCursor(SESSION_ID, "m2");

    const session = store.getState().taskSessions.items[SESSION_ID];
    expect(session.last_read_message_id).toBe("m2");
    expect(session.state).toBe("RUNNING");
    expect(session.updated_at).toBe(TS);
  });

  it("does not clobber a newer field written by a concurrent update while the request was in flight", () => {
    const store = makeStore();
    store.setState((draft) => {
      draft.taskSessions.items[SESSION_ID] = makeSession({ last_read_message_id: "m1" });
    });

    // Simulate a WS session.state_changed landing after the mark-read
    // request was dispatched but before its (now-stale) response resolves.
    store.getState().setTaskSession(makeSession({ state: "WAITING_FOR_INPUT" }));
    // The mark-read response resolves afterward, carrying only the cursor.
    store.getState().updateSessionReadCursor(SESSION_ID, "m1");

    const session = store.getState().taskSessions.items[SESSION_ID];
    expect(session.state).toBe("WAITING_FOR_INPUT");
    expect(session.last_read_message_id).toBe("m1");
  });

  it("is a no-op when the session isn't in the store (never creates a bare record)", () => {
    const store = makeStore();

    store.getState().updateSessionReadCursor(SESSION_ID, "m2");

    expect(store.getState().taskSessions.items[SESSION_ID]).toBeUndefined();
  });

  it("mirrors the update into the per-task session list", () => {
    const store = makeStore();
    const session = makeSession({ last_read_message_id: "m1" });
    store.setState((draft) => {
      draft.taskSessions.items[SESSION_ID] = session;
      draft.taskSessionsByTask.itemsByTaskId[TASK_ID] = [session];
    });

    store.getState().updateSessionReadCursor(SESSION_ID, "m2");

    const listed = store.getState().taskSessionsByTask.itemsByTaskId[TASK_ID]?.[0];
    expect(listed?.last_read_message_id).toBe("m2");
  });
});
