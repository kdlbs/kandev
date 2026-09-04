import { describe, it, expect } from "vitest";
import { create } from "zustand";
import { immer } from "zustand/middleware/immer";
import { createSessionSlice } from "./session-slice";
import { createSessionRuntimeSlice } from "../session-runtime/session-runtime-slice";
import type { QueuedMessage, SessionSlice } from "./types";
import type { SessionRuntimeSlice } from "../session-runtime/types";
import {
  agentProfileId as toAgentProfileId,
  repositoryId as toRepositoryId,
  sessionId as toSessionId,
  taskId as toTaskId,
  type TaskSession,
} from "@/lib/types/http";

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

type SessionOverrides = Partial<Omit<TaskSession, "id" | "agent_profile_id" | "repository_id">> & {
  id?: string;
  agent_profile_id?: string;
  repository_id?: string;
};

function makeSession(overrides: SessionOverrides = {}): TaskSession {
  const { id, agent_profile_id, repository_id, ...rest } = overrides;
  return {
    id: id ? toSessionId(id) : SESSION_ID,
    task_id: TASK_ID,
    state: "RUNNING",
    started_at: TS,
    updated_at: TS,
    ...(agent_profile_id !== undefined
      ? { agent_profile_id: toAgentProfileId(agent_profile_id) }
      : {}),
    ...(repository_id !== undefined ? { repository_id: toRepositoryId(repository_id) } : {}),
    ...rest,
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

  it("preserves backend cancellation state across unrelated partial events", () => {
    const store = makeStore();

    store
      .getState()
      .upsertTaskSessionFromEvent(TASK_ID, makeSession({ cancellation_pending: true }));
    store.getState().upsertTaskSessionFromEvent(TASK_ID, makeSession({ state: "RUNNING" }));

    expect(store.getState().taskSessions.items[SESSION_ID].cancellation_pending).toBe(true);
  });

  it("seeds environmentIdBySessionId when task_environment_id is present", () => {
    const store = makeStore();

    store
      .getState()
      .upsertTaskSessionFromEvent(TASK_ID, makeSession({ task_environment_id: "env-1" }));

    expect(store.getState().environmentIdBySessionId[SESSION_ID]).toBe("env-1");
  });

  it("preserves the live workspace root when a later partial refresh omits it", () => {
    const store = makeStore();

    store
      .getState()
      .upsertTaskSessionFromEvent(
        TASK_ID,
        makeSession({ workspace_path: "/task-root", worktree_path: "/task-root/kandev" }),
      );
    store.getState().setTaskSessionsForTask(TASK_ID, [makeSession()], {});

    const session = store.getState().taskSessions.items[SESSION_ID];
    expect(session.workspace_path).toBe("/task-root");
    expect(session.worktree_path).toBe("/task-root/kandev");
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

  it("invalidates a loaded list when an event introduces a partial session", () => {
    const store = makeStore();
    const existing = makeSession({ id: "session-existing", repository_id: "repo-1" });
    store.getState().setTaskSessionsForTask(TASK_ID, [existing], {});

    store.getState().upsertTaskSessionFromEvent(TASK_ID, makeSession());

    expect(store.getState().taskSessionsByTask.loadedByTaskId[TASK_ID]).toBe(false);
    expect(store.getState().taskSessionsByTask.itemsByTaskId[TASK_ID].map((s) => s.id)).toEqual([
      "session-existing",
      SESSION_ID,
    ]);
  });

  it("keeps a loaded list authoritative when an event updates a known session", () => {
    const store = makeStore();
    store
      .getState()
      .setTaskSessionsForTask(TASK_ID, [makeSession({ repository_id: "repo-1" })], {});

    store.getState().upsertTaskSessionFromEvent(TASK_ID, makeSession({ state: "COMPLETED" }));

    expect(store.getState().taskSessionsByTask.loadedByTaskId[TASK_ID]).toBe(true);
    expect(store.getState().taskSessions.items[SESSION_ID].repository_id).toBe("repo-1");
  });
});

describe("cancellation revision ordering", () => {
  it("rejects a stale REST cancellation snapshot after a newer live event", () => {
    const store = makeStore();

    store
      .getState()
      .upsertTaskSessionFromEvent(
        TASK_ID,
        makeSession({ cancellation_pending: true, cancellation_revision: 1 }),
      );
    store
      .getState()
      .upsertTaskSessionFromEvent(
        TASK_ID,
        makeSession({ cancellation_pending: false, cancellation_revision: 2 }),
      );

    // A delayed REST response captured during the older pending generation must
    // not restore true after the newer live false event has settled.
    store
      .getState()
      .setTaskSessionsForTask(
        TASK_ID,
        [makeSession({ cancellation_pending: true, cancellation_revision: 1 })],
        {},
      );

    const session = store.getState().taskSessions.items[SESSION_ID];
    expect(session.cancellation_pending).toBe(false);
    expect(session.cancellation_revision).toBe(2);
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
    store
      .getState()
      .setTaskSessionsForTask(TASK_ID, [makeSession({ repository_id: "repo-1" })], {});

    const session = store.getState().taskSessions.items[SESSION_ID];
    expect(session.task_environment_id).toBe("env-1");
    expect(session.agent_profile_id).toBe("profile-1");
    expect(session.repository_id).toBe("repo-1");
    expect(store.getState().environmentIdBySessionId[SESSION_ID]).toBe("env-1");
  });

  it("flips loadedByTaskId to true (unlike upsertTaskSessionFromEvent)", () => {
    const store = makeStore();

    store.getState().setTaskSessionsForTask(TASK_ID, [makeSession()], {});

    expect(store.getState().taskSessionsByTask.loadedByTaskId[TASK_ID]).toBe(true);
  });
});

describe("setTaskSessionsForTask reconciles active turns", () => {
  it("clears an orphaned active turn when an authoritative refresh reports WAITING_FOR_INPUT", () => {
    const store = makeStore();
    store.setState((draft) => {
      draft.turns.activeBySession[SESSION_ID] = "turn-from-missed-history";
    });

    store
      .getState()
      .setTaskSessionsForTask(
        TASK_ID,
        [makeSession({ state: "WAITING_FOR_INPUT", updated_at: "2026-04-20T00:01:00Z" })],
        {},
      );

    expect(store.getState().turns.activeBySession[SESSION_ID]).toBeNull();
  });

  it("keeps a newer active turn when an older WAITING_FOR_INPUT state arrives", () => {
    const store = makeStore();
    const turnStartedAt = "2026-04-20T00:02:00Z";
    store.getState().addTurn({
      id: "turn-new",
      session_id: SESSION_ID,
      task_id: TASK_ID,
      started_at: turnStartedAt,
      created_at: turnStartedAt,
      updated_at: turnStartedAt,
    });
    store.getState().setActiveTurn(SESSION_ID, "turn-new");

    store
      .getState()
      .upsertTaskSessionFromEvent(
        TASK_ID,
        makeSession({ state: "WAITING_FOR_INPUT", updated_at: "2026-04-20T00:01:00Z" }),
      );

    expect(store.getState().turns.activeBySession[SESSION_ID]).toBe("turn-new");
  });

  it("keeps an active turn while the refreshed session remains RUNNING", () => {
    const store = makeStore();
    const turnStartedAt = "2026-04-20T00:02:00Z";
    store.getState().addTurn({
      id: "turn-running",
      session_id: SESSION_ID,
      task_id: TASK_ID,
      started_at: turnStartedAt,
      created_at: turnStartedAt,
      updated_at: turnStartedAt,
    });
    store.getState().setActiveTurn(SESSION_ID, "turn-running");

    store
      .getState()
      .setTaskSessionsForTask(
        TASK_ID,
        [makeSession({ state: "RUNNING", updated_at: "2026-04-20T00:03:00Z" })],
        {},
      );

    expect(store.getState().turns.activeBySession[SESSION_ID]).toBe("turn-running");
  });

  it("clears only the active-turn markers authoritatively retired by source adoption", () => {
    const store = makeStore();
    const adoptedSession = SESSION_ID;
    const stillActiveSession = toSessionId("session-still-active");
    const startedAt = "2026-04-20T00:02:00Z";
    store.getState().addTurn({
      id: "turn-adopted",
      session_id: adoptedSession,
      task_id: TASK_ID,
      started_at: startedAt,
      created_at: startedAt,
      updated_at: startedAt,
    });
    store.getState().addTurn({
      id: "turn-still-active",
      session_id: stillActiveSession,
      task_id: TASK_ID,
      started_at: startedAt,
      created_at: startedAt,
      updated_at: startedAt,
    });
    store.getState().setActiveTurn(adoptedSession, "turn-adopted");
    store.getState().setActiveTurn(stillActiveSession, "turn-still-active");

    store.getState().reconcileWorkspaceSourcesAdopted([adoptedSession]);

    expect(store.getState().turns.activeBySession[adoptedSession]).toBeNull();
    expect(store.getState().turns.activeBySession[stillActiveSession]).toBe("turn-still-active");
  });
});

describe("session foreground activity reconciliation", () => {
  // @covers AC-PLATFORM-BACKGROUND-WORK-LIVENESS-001.9
  it("clears stale activity when an authoritative settled snapshot omits the field", () => {
    const store = makeStore();

    store
      .getState()
      .setTaskSession(
        makeSession({ state: "WAITING_FOR_INPUT", foreground_activity: "background" }),
      );
    expect(store.getState().taskSessions.items[SESSION_ID].foreground_activity).toBe("background");

    store
      .getState()
      .setTaskSessionsForTask(
        TASK_ID,
        [makeSession({ state: "WAITING_FOR_INPUT", repository_id: "repo-1" })],
        { [SESSION_ID]: 0 },
      );

    const session = store.getState().taskSessions.items[SESSION_ID];
    expect(session.foreground_activity).toBeNull();
    expect(session.repository_id).toBe("repo-1");
  });

  it("preserves activity when a partial event omits the field", () => {
    const store = makeStore();

    store
      .getState()
      .upsertTaskSessionFromEvent(TASK_ID, makeSession({ foreground_activity: "background" }));
    store.getState().upsertTaskSessionFromEvent(TASK_ID, makeSession({ repository_id: "repo-1" }));

    expect(store.getState().taskSessions.items[SESSION_ID].foreground_activity).toBe("background");
  });

  // @covers AC-PLATFORM-BACKGROUND-WORK-LIVENESS-001.7
  // @covers AC-PLATFORM-BACKGROUND-WORK-LIVENESS-001.9
  it("preserves a repeated activity event newer than an in-flight snapshot", () => {
    const store = makeStore();

    store.getState().upsertTaskSessionFromEvent(
      TASK_ID,
      makeSession({
        foreground_activity: "background",
        active_subagent_count: 1,
        supports_steering: false,
      }),
    );
    const activityEpochsAtRequestStart = { [SESSION_ID]: 1 };
    store.getState().upsertTaskSessionFromEvent(
      TASK_ID,
      makeSession({
        foreground_activity: "background",
        active_subagent_count: 2,
        supports_steering: true,
      }),
    );

    store.getState().setTaskSessionsForTask(
      TASK_ID,
      [
        makeSession({
          foreground_activity: "generating",
          active_subagent_count: 0,
          supports_steering: false,
          repository_id: "repo-1",
        }),
      ],
      activityEpochsAtRequestStart,
    );

    expect(store.getState().taskSessions.items[SESSION_ID]).toMatchObject({
      foreground_activity: "background",
      active_subagent_count: 2,
      supports_steering: true,
      repository_id: "repo-1",
    });
  });

  it("removes the activity epoch with the session", () => {
    const store = makeStore();

    store
      .getState()
      .upsertTaskSessionFromEvent(TASK_ID, makeSession({ foreground_activity: "background" }));

    expect(store.getState().taskSessions.activityEpochBySession?.[SESSION_ID]).toBe(1);
    store.getState().removeTaskSession(TASK_ID, SESSION_ID);
    expect(store.getState().taskSessions.activityEpochBySession?.[SESSION_ID]).toBeUndefined();
  });

  it("applies an explicit substate flip from an enriched update", () => {
    const store = makeStore();

    store.getState().setTaskSession(makeSession({ foreground_activity: "background" }));
    // The enriched endpoint now reports the turn is generating again.
    store.getState().setTaskSession(makeSession({ foreground_activity: "generating" }));

    expect(store.getState().taskSessions.items[SESSION_ID].foreground_activity).toBe("generating");
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

    store.getState().setQueueEntries(SESSION_ID, entries, {
      count: 2,
      max: 10,
      mergeEnabled: true,
      autoRun: true,
    });

    expect(store.getState().queue.bySessionId[SESSION_ID]).toEqual(entries);
    expect(store.getState().queue.metaBySessionId[SESSION_ID]).toEqual({
      count: 2,
      max: 10,
      mergeEnabled: true,
      autoRun: true,
    });
  });

  it("retains a disabled merge state", () => {
    const store = makeStore();
    store.getState().setQueueEntries(SESSION_ID, [makeEntry()], {
      count: 1,
      max: 10,
      mergeEnabled: false,
      autoRun: false,
    });
    expect(store.getState().queue.metaBySessionId[SESSION_ID]?.mergeEnabled).toBe(false);
    expect(store.getState().queue.metaBySessionId[SESSION_ID]?.autoRun).toBe(false);
  });

  it("removeQueueEntry drops a single entry by id and refreshes meta.count", () => {
    const store = makeStore();
    const entries = [makeEntry({ id: "e1" }), makeEntry({ id: "e2" }), makeEntry({ id: "e3" })];
    store.getState().setQueueEntries(SESSION_ID, entries, {
      count: 3,
      max: 10,
      mergeEnabled: true,
      autoRun: true,
    });

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
    store.getState().setQueueEntries(SESSION_ID, [makeEntry()], {
      count: 1,
      max: 10,
      mergeEnabled: true,
      autoRun: true,
    });

    store.getState().clearQueueStatus(SESSION_ID);

    expect(store.getState().queue.bySessionId[SESSION_ID]).toBeUndefined();
    expect(store.getState().queue.metaBySessionId[SESSION_ID]).toBeUndefined();
  });
});
