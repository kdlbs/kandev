import { describe, it, expect } from "vitest";
import { create } from "zustand";
import { immer } from "zustand/middleware/immer";
import { createSessionSlice } from "./session-slice";
import { createSessionRuntimeSlice } from "../session-runtime/session-runtime-slice";
import type { QueuedMessage, SessionSlice } from "./types";
import type { SessionRuntimeSlice } from "../session-runtime/types";
import type { TaskSession } from "@/lib/types/http";

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

const TASK_ID = "task-1";
const SESSION_ID = "session-1";
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

describe("upsertTaskSessionFromEvent", () => {
  it("does not flip loadedByTaskId so API hydration can still run", () => {
    const store = makeStore();

    store.getState().upsertTaskSessionFromEvent(TASK_ID, makeSession());

    expect(store.getState().taskSessionsByTask.loadedByTaskId[TASK_ID]).toBeFalsy();
  });

  it("merges fields on a second call rather than replacing the row", () => {
    const store = makeStore();

    store
      .getState()
      .upsertTaskSessionFromEvent(
        TASK_ID,
        makeSession({ agent_profile_id: "profile-1", repository_id: "repo-1" }),
      );
    // Second event omits fields that were set by the first
    store.getState().upsertTaskSessionFromEvent(TASK_ID, makeSession({ state: "COMPLETED" }));

    const session = store.getState().taskSessions.items[SESSION_ID];
    expect(session.state).toBe("COMPLETED");
    expect(session.agent_profile_id).toBe("profile-1");
    expect(session.repository_id).toBe("repo-1");
  });

  it("seeds environmentIdBySessionId when task_environment_id is present", () => {
    const store = makeStore();

    store
      .getState()
      .upsertTaskSessionFromEvent(TASK_ID, makeSession({ task_environment_id: "env-1" }));

    expect(store.getState().environmentIdBySessionId[SESSION_ID]).toBe("env-1");
  });

  it("does not seed environmentIdBySessionId when task_environment_id is absent", () => {
    const store = makeStore();

    store.getState().upsertTaskSessionFromEvent(TASK_ID, makeSession());

    expect(store.getState().environmentIdBySessionId[SESSION_ID]).toBeUndefined();
  });

  it("appends to itemsByTaskId when the list already exists", () => {
    const store = makeStore();
    const other = makeSession({ id: "session-other" });

    store.getState().upsertTaskSessionFromEvent(TASK_ID, other);
    store.getState().upsertTaskSessionFromEvent(TASK_ID, makeSession());

    const list = store.getState().taskSessionsByTask.itemsByTaskId[TASK_ID];
    expect(list.map((s) => s.id)).toEqual(["session-other", SESSION_ID]);
  });
});

describe("setTaskSessionsForTask preserves WS-seeded fields", () => {
  it("merges incoming sessions with existing rows so task_environment_id is not clobbered", () => {
    const store = makeStore();

    // WS event arrives first and seeds task_environment_id + agent_profile_id
    store
      .getState()
      .upsertTaskSessionFromEvent(
        TASK_ID,
        makeSession({ task_environment_id: "env-1", agent_profile_id: "profile-1" }),
      );

    // API hydration arrives next without task_environment_id (race window)
    store.getState().setTaskSessionsForTask(TASK_ID, [makeSession({ repository_id: "repo-1" })]);

    const session = store.getState().taskSessions.items[SESSION_ID];
    expect(session.task_environment_id).toBe("env-1");
    expect(session.agent_profile_id).toBe("profile-1");
    expect(session.repository_id).toBe("repo-1");
    expect(store.getState().environmentIdBySessionId[SESSION_ID]).toBe("env-1");
  });

  it("flips loadedByTaskId to true (unlike upsertTaskSessionFromEvent)", () => {
    const store = makeStore();

    store.getState().setTaskSessionsForTask(TASK_ID, [makeSession()]);

    expect(store.getState().taskSessionsByTask.loadedByTaskId[TASK_ID]).toBe(true);
  });
});

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
