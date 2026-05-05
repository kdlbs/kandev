import { describe, it, expect } from "vitest";
import { create } from "zustand";
import { immer } from "zustand/middleware/immer";
import { createSessionSlice } from "./session-slice";
import { createSessionRuntimeSlice } from "../session-runtime/session-runtime-slice";
import type { SessionSlice } from "./types";
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
