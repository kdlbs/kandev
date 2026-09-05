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
const REVISION_EPOCH = "7";

function pendingRevision(sequence: number) {
  return { epoch: REVISION_EPOCH, sequence };
}

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

describe("setTaskSessionPendingAction — hydration", () => {
  it("updates the by-id and per-task projections without replacing session state", () => {
    const store = makeStore();
    const session = makeSession({
      pending_action: null,
      pending_action_revision: pendingRevision(1),
    });
    store.setState((draft) => {
      draft.taskSessions.items[SESSION_ID] = session;
      draft.taskSessionsByTask.itemsByTaskId[TASK_ID] = [session];
    });

    store.getState().setTaskSessionPendingAction(SESSION_ID, "clarification", pendingRevision(2));

    expect(store.getState().taskSessions.items[SESSION_ID]).toMatchObject({
      state: "RUNNING",
      pending_action: "clarification",
      pending_action_revision: pendingRevision(2),
    });
    expect(store.getState().taskSessionsByTask.itemsByTaskId[TASK_ID]?.[0].pending_action).toBe(
      "clarification",
    );
  });

  it("does not create a bare session when the projection is not loaded", () => {
    const store = makeStore();

    store.getState().setTaskSessionPendingAction(SESSION_ID, "clarification");

    expect(store.getState().taskSessions.items[SESSION_ID]).toBeUndefined();
  });
});

describe("setTaskSessionPendingAction orphan projections", () => {
  it("retains a newer event received before session-list hydration", () => {
    const store = makeStore();
    const newerRevision = pendingRevision(2);

    store
      .getState()
      .setTaskSessionPendingAction(SESSION_ID, "clarification", newerRevision, TASK_ID);
    store.getState().setTaskSessionsForTask(
      TASK_ID,
      [
        makeSession({
          pending_action: null,
          pending_action_revision: pendingRevision(1),
        }),
      ],
      {},
    );

    expect(store.getState().taskSessions.items[SESSION_ID]).toMatchObject({
      pending_action: "clarification",
      pending_action_revision: newerRevision,
    });
  });

  it("keeps the newest orphan projection when stale events and a stale list arrive", () => {
    const store = makeStore();
    const successorRevision = pendingRevision(3);

    store
      .getState()
      .setTaskSessionPendingAction(SESSION_ID, "clarification", pendingRevision(2), TASK_ID);
    store.getState().setTaskSessionPendingAction(SESSION_ID, null, successorRevision, TASK_ID);
    store
      .getState()
      .setTaskSessionPendingAction(SESSION_ID, "permission", pendingRevision(1), TASK_ID);
    store.getState().setTaskSessionsForTask(
      TASK_ID,
      [
        makeSession({
          pending_action: "clarification",
          pending_action_revision: pendingRevision(1),
        }),
      ],
      {},
    );

    expect(store.getState().taskSessions.items[SESSION_ID]).toMatchObject({
      pending_action: null,
      pending_action_revision: successorRevision,
    });
  });

  it("records a recoverable list error without making an empty list authoritative", () => {
    const store = makeStore();

    store.getState().setTaskSessionsError(TASK_ID, "service unavailable");

    expect(store.getState().taskSessionsByTask.errorByTaskId?.[TASK_ID]).toBe(
      "service unavailable",
    );
    expect(store.getState().taskSessionsByTask.loadedByTaskId[TASK_ID]).toBeUndefined();

    store.getState().setTaskSessionsForTask(TASK_ID, [makeSession()], {});

    expect(store.getState().taskSessionsByTask.errorByTaskId?.[TASK_ID]).toBeNull();
    expect(store.getState().taskSessionsByTask.loadedByTaskId[TASK_ID]).toBe(true);
  });
});

describe("setTaskSessionPendingAction revision ordering", () => {
  it("preserves a newer WebSocket projection when a deferred list response resolves", () => {
    const store = makeStore();
    const staleListSession = makeSession({
      pending_action: null,
      pending_action_revision: pendingRevision(1),
    });
    store.getState().setTaskSessionsForTask(TASK_ID, [staleListSession], {});

    store.getState().setTaskSessionPendingAction(SESSION_ID, "clarification", pendingRevision(2));
    store
      .getState()
      .setTaskSessionsForTask(TASK_ID, [{ ...staleListSession, state: "COMPLETED" }], {});

    expect(store.getState().taskSessions.items[SESSION_ID]).toMatchObject({
      state: "COMPLETED",
      pending_action: "clarification",
      pending_action_revision: pendingRevision(2),
    });
  });

  it("rejects a delayed WebSocket projection older than the HTTP snapshot", () => {
    const store = makeStore();
    store.getState().setTaskSessionsForTask(
      TASK_ID,
      [
        makeSession({
          pending_action: "clarification",
          pending_action_revision: pendingRevision(2),
        }),
      ],
      {},
    );

    store.getState().setTaskSessionPendingAction(SESSION_ID, null, pendingRevision(1));

    expect(store.getState().taskSessions.items[SESSION_ID]).toMatchObject({
      pending_action: "clarification",
      pending_action_revision: pendingRevision(2),
    });
  });

  it("accepts a newer backend epoch and rejects delayed frames from the old epoch", () => {
    const store = makeStore();
    // Epochs are parsed as bigint, so decimal width changes at restart 10 do
    // not turn this into lexicographic ordering.
    const oldRevision = { epoch: "9", sequence: 99 };
    const newRevision = { epoch: "10", sequence: 1 };
    store
      .getState()
      .setTaskSessionsForTask(
        TASK_ID,
        [makeSession({ pending_action: "clarification", pending_action_revision: oldRevision })],
        {},
      );

    store.getState().setTaskSessionPendingAction(SESSION_ID, null, newRevision);
    store.getState().setTaskSessionPendingAction(SESSION_ID, "permission", {
      ...oldRevision,
      sequence: 100,
    });

    expect(store.getState().taskSessions.items[SESSION_ID]).toMatchObject({
      pending_action: null,
      pending_action_revision: newRevision,
    });
  });

  it("rejects an unseen older backend epoch after client state is rebuilt", () => {
    const store = makeStore();
    const currentRevision = { epoch: "3", sequence: 1 };
    store
      .getState()
      .setTaskSessionsForTask(
        TASK_ID,
        [makeSession({ pending_action: null, pending_action_revision: currentRevision })],
        {},
      );

    store.getState().setTaskSessionPendingAction(SESSION_ID, "clarification", {
      epoch: "1",
      sequence: 99,
    });

    expect(store.getState().taskSessions.items[SESSION_ID]).toMatchObject({
      pending_action: null,
      pending_action_revision: currentRevision,
    });
  });
});
