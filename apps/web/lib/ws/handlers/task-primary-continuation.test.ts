import { beforeEach, describe, expect, it, vi } from "vitest";
import { fetchTaskSession, listTaskSessions } from "@/lib/api/domains/session-api";
import type * as SessionApi from "@/lib/api/domains/session-api";
import { registerTaskSessionHandlers } from "./agent-session";
import { registerTasksHandlers } from "./tasks";
import { pickReplacementSessionId } from "./session-selection";
import { makeMessage, makeTask } from "./tasks.test-helpers";
import {
  SESSION_STATE_CHANGED,
  deferred,
  flushFocusWork,
  membership,
  notifySibling,
  notifyState,
  promote,
  response,
  withSiblings,
} from "./task-primary-focus.test-helpers";

vi.mock("@/lib/api/domains/session-api", async (original) => ({
  ...(await original<typeof SessionApi>()),
  fetchTaskSession: vi.fn(),
  listTaskSessions: vi.fn(),
}));
beforeEach(() => {
  vi.mocked(fetchTaskSession).mockReset();
  vi.mocked(listTaskSessions).mockReset();
});
type TestStore = ReturnType<typeof withSiblings>;
const TIME = "2026-11-19T12:00:00Z";
const LATER = "2026-11-19T12:00:01Z";
const OTHER_TASK = "other-task";
const ENTRY = "entry:00000000000000000042";
const SURVIVOR_CURSOR = "survivor-read";
const NEW_SESSION_ENTRY = "new-session";
function proofRead() {
  return Promise.withResolvers<Awaited<ReturnType<typeof listTaskSessions>>>();
}
function remove(store: TestStore, id = "old") {
  registerTaskSessionHandlers(store)["session.removed"]!({
    id: "remove",
    type: "notification",
    action: "session.removed",
    payload: { task_id: "t1", session_id: id },
  });
}
function beginMove(store: TestStore) {
  const id = store.getState().beginWorkflowSessionFocus({
    taskId: "t1",
    workflowId: "wf1",
    destinationStepId: "step2",
    presentationToken: 1,
    navigationRevision: store.getState().taskRemoval.navigationRevision,
  });
  if (id === null) throw new Error("Could not begin workflow test move");
  return id;
}
function commitMove(store: TestStore, requestId: number, target = "latest") {
  store
    .getState()
    .bindWorkflowSessionFocus({ requestId, presentationToken: 1, entryIdentity: ENTRY });
  registerTasksHandlers(store)["task.updated"]!(
    makeMessage({
      ...makeTask("t1", target),
      workflow_step_id: "step2",
      metadata: {
        workflow_session_route: {
          operation_id: "move",
          destination_step_id: "step2",
          entry_identity: ENTRY,
          destination_session_id: target,
          phase: "committed",
        },
      },
    }),
  );
}

// @covers AC-TASKS-TASK-UNREAD-DOT-001.11
describe("retirement and explicit workflow ownership", () => {
  it("preserves a newer in-flight workflow move while retiring its predecessor", async () => {
    const store = withSiblings("FAILED");
    const read = proofRead();
    vi.mocked(listTaskSessions).mockReturnValue(read.promise);
    const selected: Array<string | null> = [];
    const release = store.subscribe((state) => selected.push(state.tasks.activeSessionId));
    try {
      remove(store);
      const requestId = beginMove(store);
      read.resolve(membership("next", "latest"));
      await flushFocusWork();
      expect(store.getState().taskSessions.items.old).toBeUndefined();
      expect(selected).not.toContain("next");
      commitMove(store, requestId);
      expect(store.getState().tasks.activeSessionId).toBe("latest");
    } finally {
      release();
    }
  });

  it("cancels only a workflow request whose own recipient is retired", async () => {
    const store = withSiblings("FAILED");
    commitMove(store, beginMove(store), "old");
    expect(store.getState().workflowSessionFocus.request?.sessionId).toBe("old");
    vi.mocked(listTaskSessions).mockResolvedValue(membership());
    remove(store);
    await flushFocusWork();
    expect(store.getState().workflowSessionFocus.request).toBeNull();
    expect(store.getState().taskSessions.items.old).toBeUndefined();
  });
});

// @covers AC-TASKS-TASK-UNREAD-DOT-001.11
describe("owned empty-slot continuation", () => {
  it.each(["none", "pin", "navigation"] as const)(
    "reconciles promotion without overriding %s",
    async (change) => {
      const store = withSiblings("FAILED");
      const emptyPrimary = membership("next", "latest");
      for (const row of emptyPrimary.sessions) row.is_primary = false;
      vi.mocked(listTaskSessions)
        .mockResolvedValueOnce(emptyPrimary)
        .mockResolvedValue(membership("next", "latest"));
      remove(store);
      await flushFocusWork();
      expect(store.getState().taskSessions.items.old).toBeUndefined();
      expect(store.getState().tasks.activeSessionId).toBeNull();
      if (change === "pin") store.getState().setActiveSession("t1", "latest");
      if (change === "navigation") store.getState().setActiveTask(OTHER_TASK);
      promote(store);
      await flushFocusWork();
      const expected = { none: "next", pin: "latest", navigation: null } as const;
      expect(store.getState().tasks.activeSessionId).toBe(expected[change]);
      if (change === "navigation") expect(store.getState().tasks.activeTaskId).toBe(OTHER_TASK);
    },
  );

  it("does not overwrite a committed workflow after an empty slot starts reproof", async () => {
    const store = withSiblings("FAILED");
    const emptyPrimary = membership("next", "latest");
    for (const row of emptyPrimary.sessions) row.is_primary = false;
    const held = proofRead();
    vi.mocked(listTaskSessions).mockResolvedValueOnce(emptyPrimary).mockReturnValue(held.promise);
    remove(store);
    await flushFocusWork();
    promote(store);
    const requestId = beginMove(store);
    commitMove(store, requestId);
    held.resolve(membership("next", "latest"));
    await flushFocusWork();
    expect(store.getState().tasks.activeSessionId).toBe("latest");
  });

  it("continues a workflow-cancelled empty slot without admitting an unseen sibling", async () => {
    const store = withSiblings("FAILED");
    store.getState().setActiveTask(OTHER_TASK);
    promote(store, "next");
    store.getState().setActiveSessionAuto("t1", "old");
    const first = proofRead();
    vi.mocked(listTaskSessions).mockReturnValue(first.promise);
    const requestId = beginMove(store);
    remove(store);
    first.resolve(membership("next", "latest"));
    await flushFocusWork();
    store.getState().cancelWorkflowSessionFocus({ requestId });
    expect(store.getState().tasks.activeSessionId).toBeNull();
    const calls = vi.mocked(listTaskSessions).mock.calls.length;
    promote(store, "next");
    registerTasksHandlers(store)["task.updated"]!(
      makeMessage({
        id: "t1",
        title: "Rename without primary",
      }),
    );
    registerTasksHandlers(store)["task.created"]!({
      ...makeMessage(makeTask("t1", "next")),
      action: "task.created",
    });
    await flushFocusWork();
    expect(vi.mocked(listTaskSessions).mock.calls).toHaveLength(calls);
    expect(store.getState().tasks.activeSessionId).toBeNull();
    store
      .getState()
      .setTaskSessionsForTask("t1", [response("WAITING_FOR_INPUT", "next").session], {});
    store.getState().removeTaskSession("t1", "latest");
    notifySibling(store);
    expect(store.getState().tasks.activeSessionId).toBeNull();
    const next = proofRead();
    vi.mocked(listTaskSessions).mockReturnValue(next.promise);
    promote(store, "latest");
    expect(store.getState().tasks.activeSessionId).toBeNull();
    next.resolve(membership("latest", "next"));
    await flushFocusWork();
    expect(store.getState().tasks.activeSessionId).toBe("latest");
  });
});

// @covers AC-TASKS-TASK-UNREAD-DOT-001.11
describe("competing committed remote removals", () => {
  it.each(["old-first", "new-first"] as const)(
    "never selects the second removed recipient with %s",
    async (order) => {
      const store = withSiblings("FAILED");
      store.getState().setActiveTask(OTHER_TASK);
      promote(store);
      store.getState().setActiveSessionAuto("t1", "old");
      store.getState().setTaskSession({
        ...store.getState().taskSessions.items.latest,
        last_read_message_id: SURVIVOR_CURSOR,
      });
      const oldProof = proofRead();
      const nextProof = proofRead();
      vi.mocked(listTaskSessions)
        .mockReturnValueOnce(oldProof.promise)
        .mockReturnValueOnce(nextProof.promise)
        .mockResolvedValue(membership("latest"));
      const selected: Array<string | null> = [];
      const release = store.subscribe((state) => selected.push(state.tasks.activeSessionId));
      try {
        remove(store);
        remove(store, "next");
        if (order === "old-first") oldProof.resolve(membership("next", "latest"));
        else nextProof.resolve(membership("latest"));
        await flushFocusWork();
        expect(selected).not.toContain("next");
        oldProof.resolve(membership("next", "latest"));
        nextProof.resolve(membership("latest"));
        await flushFocusWork();
        expect(selected).not.toContain("next");
        expect(store.getState().taskSessions.items.old).toBeUndefined();
        expect(store.getState().taskSessions.items.next).toBeUndefined();
        expect(store.getState().tasks.activeSessionId).toBe("latest");
        expect(store.getState().taskSessions.items.latest.last_read_message_id).toBe(
          SURVIVOR_CURSOR,
        );
      } finally {
        release();
      }
    },
  );
});

// @covers AC-TASKS-TASK-UNREAD-DOT-001.6
describe("projected recipient admission", () => {
  it.each(["admit", "failed", "foreign", "navigation", "restart"] as const)(
    "reconsiders real membership with %s",
    async (kind) => {
      const store = withSiblings("STARTING");
      store.getState().removeTaskSession("t1", "next");
      store.getState().removeTaskSession("t1", "latest");
      store.getState().setActiveSession("t1", "old");
      vi.mocked(fetchTaskSession).mockResolvedValue(response("CANCELLED"));
      promote(store);
      notifyState(store, "CANCELLED", TIME);
      await flushFocusWork();
      expect(store.getState().tasks.activeSessionId).toBe("old");
      if (kind === "navigation") store.getState().setActiveTask(OTHER_TASK);
      if (kind === "restart") notifyState(store, "STARTING", LATER);
      const recipient = response(
        kind === "failed" ? "FAILED" : "WAITING_FOR_INPUT",
        "next",
        kind === "foreign" ? "foreign" : "t1",
      ).session;
      store
        .getState()
        .setTaskSessionsForTask("t1", [store.getState().taskSessions.items.old, recipient], {});
      await flushFocusWork();
      let expected: string | null = "old";
      if (kind === "admit") expected = "next";
      if (kind === "navigation") expected = null;
      expect(store.getState().tasks.activeSessionId).toBe(expected);
    },
  );
});

// @covers AC-TASKS-TASK-UNREAD-DOT-001.6
describe("chronological lifecycle replacement", () => {
  it.each(["http-desc", "event-appended"] as const)(
    "uses start time after recipient failure with %s data",
    async (order) => {
      const store = withSiblings("CANCELLED");
      store.getState().removeTaskSession("t1", "latest");
      const old = {
        ...store.getState().taskSessions.items.old,
        started_at: "2026-11-19T10:00:00Z",
      };
      const recent = {
        ...store.getState().taskSessions.items.next,
        started_at: "2026-11-19T11:30:00Z",
      };
      const older = {
        ...response("WAITING_FOR_INPUT", "older").session,
        started_at: "2026-11-19T11:00:00Z",
      };
      store.getState().setActiveTask(OTHER_TASK);
      store
        .getState()
        .setTaskSessionsForTask(
          "t1",
          order === "http-desc" ? [recent, older, old] : [recent, old],
          {},
        );
      if (order === "event-appended") {
        store.getState().upsertTaskSessionFromEvent("t1", older);
      }
      store.getState().setActiveSessionAuto("t1", "old");
      const read = deferred();
      vi.mocked(fetchTaskSession).mockReturnValue(read.promise);
      notifySibling(store);
      registerTaskSessionHandlers(store)[SESSION_STATE_CHANGED]!({
        id: "failed-recipient",
        type: "notification",
        action: SESSION_STATE_CHANGED,
        payload: { task_id: "t1", session_id: "latest", new_state: "FAILED", updated_at: LATER },
      });
      read.resolve(response("CANCELLED"));
      await flushFocusWork();
      expect(store.getState().tasks.activeSessionId).toBe("next");
    },
  );

  it("breaks equal-time ties independently of list order", () => {
    const store = withSiblings("FAILED");
    const rows = store.getState().taskSessionsByTask.itemsByTaskId.t1;
    store.getState().setTaskSessionsForTask("t1", rows, {});
    expect(pickReplacementSessionId(store.getState(), "t1")).toBe("latest");
    store.getState().setTaskSessionsForTask("t1", [...rows].reverse(), {});
    expect(pickReplacementSessionId(store.getState(), "t1")).toBe("latest");
  });
});

// @covers AC-TASKS-TASK-UNREAD-DOT-001.6, AC-TASKS-TASK-UNREAD-DOT-001.11
describe("lifecycle handoff competing with a background deletion", () => {
  it.each(["removal-first", "terminal-first"] as const)(
    "chooses the survivor with %s arrival",
    async (order) => {
      const store = withSiblings("RUNNING");
      store.getState().setActiveTask(OTHER_TASK);
      promote(store, "latest");
      const rows = store.getState().taskSessionsByTask.itemsByTaskId.t1.map((row) => ({
        ...row,
        started_at: row.id === "next" ? LATER : TIME,
        last_read_message_id: row.id === "latest" ? SURVIVOR_CURSOR : row.last_read_message_id,
      }));
      store.getState().setTaskSessionsForTask("t1", rows, {});
      store.getState().setActiveSessionAuto("t1", "old");
      const ended = deferred();
      const deletion = proofRead();
      vi.mocked(fetchTaskSession).mockReturnValue(ended.promise);
      vi.mocked(listTaskSessions).mockReturnValue(deletion.promise);
      const selected: Array<string | null> = [];
      const release = store.subscribe((state) => selected.push(state.tasks.activeSessionId));
      try {
        if (order === "removal-first") remove(store, "next");
        notifyState(store, "COMPLETED", LATER);
        if (order === "terminal-first") remove(store, "next");
        expect(store.getState().tasks.activeSessionId).toBe("old");
        ended.resolve(response("COMPLETED"));
        await flushFocusWork();
        expect(selected).not.toContain("next");
        expect(store.getState().tasks.activeSessionId).toBe("latest");
        deletion.resolve(membership("latest"));
        await flushFocusWork();
        expect(store.getState().taskSessions.items.next).toBeUndefined();
        expect(store.getState().taskSessions.items.latest.last_read_message_id).toBe(
          SURVIVOR_CURSOR,
        );
      } finally {
        release();
      }
    },
  );
});

// @covers AC-TASKS-TASK-UNREAD-DOT-001.6
describe("durable chronology after a real partial lifecycle event", () => {
  it.each(["continue", "newer-pin", "newer-navigation", "predecessor-starts"] as const)(
    "waits for normal membership admission with %s",
    async (outcome) => {
      const store = withSiblings("RUNNING");
      store.getState().removeTaskSession("t1", "next");
      store.getState().setActiveTask(OTHER_TASK);
      promote(store, "latest");
      store.getState().setActiveSessionAuto("t1", "old");
      const ended = deferred();
      vi.mocked(fetchTaskSession).mockReturnValue(ended.promise);
      const hydration = proofRead();
      const admitted = hydration.promise.then(({ sessions }) => {
        store.getState().setTaskSessionsForTask("t1", sessions, {});
      });
      registerTaskSessionHandlers(store)[SESSION_STATE_CHANGED]!({
        id: "new-partial",
        type: "notification",
        action: SESSION_STATE_CHANGED,
        payload: {
          task_id: "t1",
          session_id: "next",
          new_state: "WAITING_FOR_INPUT",
          updated_at: LATER,
        },
      });
      const selected: Array<string | null> = [];
      const release = store.subscribe((state) => selected.push(state.tasks.activeSessionId));
      try {
        const endTime = "2026-11-19T12:00:02Z";
        notifyState(store, "COMPLETED", endTime);
        const endedRow = response("COMPLETED");
        endedRow.session.updated_at = endTime;
        ended.resolve(endedRow);
        await flushFocusWork();
        expect(store.getState().tasks.activeSessionId).toBe("old");
        expect(selected).not.toContain("latest");
        if (outcome === "newer-pin") store.getState().setActiveSession("t1", "latest");
        if (outcome === "newer-navigation") store.getState().setActiveTask(OTHER_TASK);
        if (outcome === "predecessor-starts") {
          notifyState(store, "STARTING", "2026-11-19T12:00:03Z");
        }
        const rows = [
          {
            ...response("WAITING_FOR_INPUT", "next").session,
            started_at: LATER,
            updated_at: LATER,
          },
          response("WAITING_FOR_INPUT", "latest").session,
          endedRow.session,
        ];
        hydration.resolve({ sessions: rows, total: rows.length });
        await admitted;
        await flushFocusWork();
        if (outcome === "continue") expect(store.getState().tasks.activeSessionId).toBe("next");
        if (outcome === "newer-pin") {
          expect(store.getState().tasks.activeSessionId).toBe("latest");
          expect(store.getState().tasks.pinnedSessionId).toBe("latest");
        }
        if (outcome === "newer-navigation")
          expect(store.getState().tasks.activeTaskId).toBe(OTHER_TASK);
        if (outcome === "predecessor-starts")
          expect(store.getState().tasks.activeSessionId).toBe("old");
      } finally {
        release();
      }
    },
  );
});

// @covers AC-TASKS-TASK-UNREAD-DOT-001.6, AC-TASKS-TASK-UNREAD-DOT-001.11
describe("workflow intent preceding automatic focus", () => {
  it.each(["primary", "terminal", NEW_SESSION_ENTRY, "initial-adoption"] as const)(
    "keeps the committed workflow as the %s winner",
    async (entry) => {
      const store = withSiblings(entry === NEW_SESSION_ENTRY ? "IDLE" : "RUNNING");
      if (entry === NEW_SESSION_ENTRY || entry === "initial-adoption") {
        store.getState().removeTaskSession("t1", "latest");
      }
      if (entry === "initial-adoption") {
        store.setState(({ tasks }) => ({ tasks: { ...tasks, activeSessionId: null } }));
      }
      const expectedBeforeCommit = store.getState().tasks.activeSessionId;
      const move = beginMove(store);
      let proofState: "RUNNING" | "COMPLETED" | "IDLE" = "RUNNING";
      if (entry === "terminal") proofState = "COMPLETED";
      if (entry === NEW_SESSION_ENTRY) proofState = "IDLE";
      vi.mocked(fetchTaskSession).mockResolvedValue(response(proofState));
      if (entry === "primary") promote(store);
      else if (entry === "terminal") notifyState(store, "COMPLETED", LATER);
      else notifySibling(store);
      await flushFocusWork();
      expect(store.getState().tasks.activeSessionId).toBe(expectedBeforeCommit);
      commitMove(store, move, "next");
      expect(store.getState().tasks.activeSessionId).toBe("next");
    },
  );
});
