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
        last_read_message_id: "survivor-read",
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
          "survivor-read",
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
