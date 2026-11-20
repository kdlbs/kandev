import { beforeEach, describe, expect, it, vi } from "vitest";
import { fetchTaskSession, listTaskSessions } from "@/lib/api/domains/session-api";
import type * as SessionApi from "@/lib/api/domains/session-api";
import { ApiError } from "@/lib/api/client";
import { getOwnedTaskSessionId } from "@/src/task-route-projection";
import { invalidateTaskSessionMembership } from "@/lib/state/task-session-reads";
import { registerTaskSessionHandlers } from "./agent-session";
import { registerTasksHandlers } from "./tasks";
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
  vi.clearAllMocks();
  vi.mocked(fetchTaskSession).mockReset();
  vi.mocked(listTaskSessions).mockReset();
});

const SNAPSHOT_TIME = "2026-11-19T12:00:00Z";
const REMOVED = "session.removed";
const RECIPIENT_EXECUTION_ID = "recipient-execution";
const FAILURE_TIME = "2026-11-19T12:00:02Z";
const OTHER_TASK_ID = "other-task";
const STARTING_TIME = "2026-11-19T12:00:01Z";
const TERMINAL_FIRST = "terminal-first";
const PROMOTION_FIRST = "promotion-first";
const TERMINAL_LATE = "terminal-late";
const TERMINAL_OMITTED = "terminal-omitted";
function removeDisplayed(store: ReturnType<typeof withSiblings>, taskId?: string) {
  registerTaskSessionHandlers(store)[REMOVED]?.({
    id: "removed",
    type: "notification",
    action: REMOVED,
    payload: { session_id: "old", ...(taskId === undefined ? {} : { task_id: taskId }) },
  });
}

function notifyRecipientAgentctl(
  store: ReturnType<typeof withSiblings>,
  status: "ready" | "error" | "starting",
) {
  const handlers = registerTaskSessionHandlers(store);
  const event = {
    id: "recipient-status",
    type: "notification" as const,
    timestamp: SNAPSHOT_TIME,
    payload: { task_id: "t1", session_id: "latest", agent_execution_id: RECIPIENT_EXECUTION_ID },
  };
  if (status === "ready") {
    handlers["session.agentctl_ready"]!({ ...event, action: "session.agentctl_ready" });
  } else if (status === "error") {
    handlers["session.agentctl_error"]!({
      ...event,
      action: "session.agentctl_error",
      payload: { ...event.payload, error_message: "recipient error" },
    });
  } else {
    handlers["session.agentctl_starting"]!({ ...event, action: "session.agentctl_starting" });
  }
}

// @covers AC-TASKS-TASK-UNREAD-DOT-001.6
// @covers AC-TASKS-TASK-UNREAD-DOT-001.8
describe("lifecycle observations before promotion", () => {
  it.each([
    ["terminal", "FAILED"],
    ["terminal", "STARTING"],
    ["sibling", "FAILED"],
    ["sibling", "STARTING"],
  ] as const)("keeps the view for %s before a durable %s", async (event, durableState) => {
    const store = withSiblings("CANCELLED");
    store.getState().removeTaskSession("t1", "latest");
    const read = deferred();
    vi.mocked(fetchTaskSession).mockReturnValue(read.promise);
    if (event === "terminal") notifyState(store, "CANCELLED", SNAPSHOT_TIME);
    else notifySibling(store);
    expect(store.getState().tasks.activeSessionId).toBe("old");
    read.resolve(response(durableState));
    await flushFocusWork();
    expect(store.getState().tasks.activeSessionId).toBe("old");
    promote(store);
    notifyState(store, "STARTING", STARTING_TIME);
    notifyState(store, "FAILED", FAILURE_TIME);
    await flushFocusWork();
    expect(store.getState().tasks.activeSessionId).toBe("old");
  });

  it.each(["COMPLETED", "CANCELLED"] as const)(
    "confirms genuine %s before list fallback",
    async (state) => {
      const store = withSiblings(state);
      const read = deferred();
      vi.mocked(fetchTaskSession).mockReturnValue(read.promise);
      notifyState(store, state, SNAPSHOT_TIME);
      expect(store.getState().tasks.activeSessionId).toBe("old");
      read.resolve(response(state));
      await flushFocusWork();
      expect(store.getState().tasks.activeSessionId).toBe("latest");
    },
  );

  it("does not use an ended response after a newer STARTING observation", async () => {
    const store = withSiblings("CANCELLED");
    const read = deferred();
    vi.mocked(fetchTaskSession).mockReturnValue(read.promise);
    notifyState(store, "CANCELLED", SNAPSHOT_TIME);
    notifyState(store, "STARTING", STARTING_TIME);
    read.resolve(response("CANCELLED"));
    await flushFocusWork();
    expect(store.getState().tasks.activeSessionId).toBe("old");
  });

  it("rechecks a lifecycle candidate after its primary promotion", async () => {
    const store = withSiblings("CANCELLED");
    store.getState().removeTaskSession("t1", "latest");
    const oldRead = deferred();
    const promotedRead = deferred();
    vi.mocked(fetchTaskSession)
      .mockReturnValueOnce(oldRead.promise)
      .mockReturnValue(promotedRead.promise);
    notifyState(store, "CANCELLED", SNAPSHOT_TIME);
    promote(store);
    oldRead.resolve(response("CANCELLED"));
    await flushFocusWork();
    expect(store.getState().tasks.activeSessionId).toBe("old");
    promotedRead.resolve(response("FAILED"));
    await flushFocusWork();
    expect(store.getState().tasks.activeSessionId).toBe("old");
  });
});

// @covers AC-TASKS-TASK-UNREAD-DOT-001.6
describe("lifecycle authority at publication", () => {
  it.each(["STARTING", "CANCELLED"] as const)(
    "keeps lifecycle authority across an unchanged primary with durable %s",
    async (durable) => {
      const store = withSiblings("CANCELLED");
      store.getState().setActiveTask(OTHER_TASK_ID);
      promote(store);
      store.getState().setActiveSessionAuto("t1", "old");
      const read = deferred();
      vi.mocked(fetchTaskSession).mockReturnValue(read.promise);
      notifyState(store, "CANCELLED", SNAPSHOT_TIME);
      const payload = makeTask("t1", "next");
      payload.title = "Edited task title";
      registerTasksHandlers(store)["task.updated"]!(makeMessage(payload));
      read.resolve(response(durable));
      await flushFocusWork();
      expect(store.getState().tasks.activeSessionId).toBe(
        durable === "STARTING" ? "old" : "latest",
      );
      if (durable === "STARTING") {
        notifyState(store, "STARTING", STARTING_TIME);
        notifyState(store, "FAILED", FAILURE_TIME);
        await flushFocusWork();
        expect(store.getState().tasks.activeSessionId).toBe("old");
      }
    },
  );

  it.each(["FAILED", "WAITING_FOR_INPUT"] as const)(
    "revalidates the lifecycle recipient after its %s observation",
    async (state) => {
      const store = withSiblings("CANCELLED");
      store.getState().setActiveTask(OTHER_TASK_ID);
      promote(store);
      store.getState().setActiveSessionAuto("t1", "old");
      store.getState().upsertTaskSessionFromEvent("t1", {
        ...store.getState().taskSessions.items.latest,
        state: "STARTING",
      });
      const read = deferred();
      vi.mocked(fetchTaskSession).mockReturnValue(read.promise);
      const selected: Array<string | null> = [];
      const release = store.subscribe((value) => selected.push(value.tasks.activeSessionId));
      try {
        notifyState(store, "CANCELLED", SNAPSHOT_TIME);
        registerTaskSessionHandlers(store)[SESSION_STATE_CHANGED]!({
          id: "recipient-settled",
          type: "notification",
          action: SESSION_STATE_CHANGED,
          payload: {
            task_id: "t1",
            session_id: "latest",
            new_state: state,
            old_state: "STARTING",
            updated_at: STARTING_TIME,
          },
        });
        read.resolve(response("CANCELLED"));
        await flushFocusWork();
        expect(store.getState().tasks.activeSessionId).toBe(state === "FAILED" ? "next" : "latest");
        if (state === "FAILED") expect(selected).not.toContain("latest");
      } finally {
        release();
      }
    },
  );
});

// @covers AC-TASKS-TASK-UNREAD-DOT-001.6
describe("lifecycle recipient after promotion", () => {
  it.each([
    ["CANCELLED", "FAILED", TERMINAL_FIRST],
    ["CANCELLED", "WAITING_FOR_INPUT", TERMINAL_FIRST],
    ["COMPLETED", "FAILED", TERMINAL_FIRST],
    ["COMPLETED", "WAITING_FOR_INPUT", TERMINAL_FIRST],
    ["CANCELLED", "FAILED", PROMOTION_FIRST],
    ["CANCELLED", "WAITING_FOR_INPUT", PROMOTION_FIRST],
    ["COMPLETED", "FAILED", PROMOTION_FIRST],
    ["COMPLETED", "WAITING_FOR_INPUT", PROMOTION_FIRST],
    ["CANCELLED", "FAILED", TERMINAL_LATE],
    ["CANCELLED", "WAITING_FOR_INPUT", TERMINAL_LATE],
    ["COMPLETED", "FAILED", TERMINAL_LATE],
    ["COMPLETED", "WAITING_FOR_INPUT", TERMINAL_LATE],
    ["CANCELLED", "FAILED", TERMINAL_OMITTED],
    ["CANCELLED", "WAITING_FOR_INPUT", TERMINAL_OMITTED],
    ["COMPLETED", "FAILED", TERMINAL_OMITTED],
    ["COMPLETED", "WAITING_FOR_INPUT", TERMINAL_OMITTED],
  ] as const)(
    "retains recipient eligibility after %s retargets to a now %s primary (%s)",
    async (endedState, recipientState, order) => {
      const store = withSiblings("RUNNING");
      store.getState().upsertTaskSessionFromEvent("t1", {
        ...store.getState().taskSessions.items.next,
        state: "STARTING",
        updated_at: SNAPSHOT_TIME,
      });
      const original = deferred();
      const retargeted = deferred();
      vi.mocked(fetchTaskSession)
        .mockReturnValueOnce(original.promise)
        .mockReturnValueOnce(retargeted.promise)
        .mockResolvedValue(response(endedState));
      const selected: Array<string | null> = [];
      const release = store.subscribe((value) => selected.push(value.tasks.activeSessionId));
      try {
        if (order !== TERMINAL_FIRST) {
          promote(store);
          if (order === PROMOTION_FIRST) notifyState(store, endedState, SNAPSHOT_TIME);
        } else {
          notifyState(store, endedState, SNAPSHOT_TIME);
          promote(store);
        }
        registerTaskSessionHandlers(store)[SESSION_STATE_CHANGED]!({
          id: "recipient-retarget-settled",
          type: "notification",
          action: SESSION_STATE_CHANGED,
          payload: {
            task_id: "t1",
            session_id: "next",
            old_state: "STARTING",
            new_state: recipientState,
            updated_at: STARTING_TIME,
          },
        });
        original.resolve(response(endedState));
        retargeted.resolve(response(endedState));
        await flushFocusWork();
        if (recipientState === "FAILED") {
          expect(selected).not.toContain("next");
          if (order === TERMINAL_LATE) notifyState(store, endedState, SNAPSHOT_TIME);
          promote(store, "latest");
          await flushFocusWork();
          expect(store.getState().tasks.activeSessionId).toBe("latest");
        } else {
          expect(store.getState().tasks.activeSessionId).toBe("next");
        }
      } finally {
        release();
      }
    },
  );
});

// @covers AC-TASKS-TASK-UNREAD-DOT-001.6
describe("handoff execution ownership", () => {
  it("does not inherit a foreign session execution during initial adoption", () => {
    const store = withSiblings("RUNNING");
    store.getState().setTaskSession(response("RUNNING", "old", "foreign-task").session);
    store.getState().removeTaskSession("t1", "latest");
    store.getState().setSessionAgentctlStatus("old", {
      status: "ready",
      agentExecutionId: "foreign-execution",
      updatedAt: SNAPSHOT_TIME,
    });
    notifySibling(store);
    expect(store.getState().tasks.activeSessionId).toBe("latest");
    expect(
      store.getState().sessionAgentctl.itemsBySessionId.latest?.agentExecutionId,
    ).toBeUndefined();
  });

  it.each(["ready", "error", "starting", "snapshot", "metadata"] as const)(
    "preserves recipient-owned %s execution at delayed activation",
    async (status) => {
      const store = withSiblings("RUNNING");
      if (status === "metadata") {
        store
          .getState()
          .setTaskSession({ ...store.getState().taskSessions.items.latest, state: "STARTING" });
        store.setState({ sessionAgentctl: { itemsBySessionId: {} } });
      }
      store.getState().setSessionAgentctlStatus("old", {
        status: "ready",
        agentExecutionId: "predecessor-execution",
        updatedAt: SNAPSHOT_TIME,
      });
      const read = deferred();
      vi.mocked(fetchTaskSession).mockReturnValue(read.promise);
      notifyState(store, "COMPLETED", SNAPSHOT_TIME);
      if (status === "snapshot" || status === "metadata") {
        store.getState().setTaskSession({
          ...store.getState().taskSessions.items.latest,
          state: status === "metadata" ? "STARTING" : "RUNNING",
          agent_execution_id: RECIPIENT_EXECUTION_ID,
        });
      } else {
        notifyRecipientAgentctl(store, status);
      }
      const recipientStatus = store.getState().sessionAgentctl.itemsBySessionId.latest;
      if (status !== "metadata")
        expect(recipientStatus?.agentExecutionId).toBe(RECIPIENT_EXECUTION_ID);
      let observed: unknown;
      const release = store.subscribe((state) => {
        if (state.tasks.activeSessionId === "latest")
          observed = state.sessionAgentctl.itemsBySessionId.latest;
      });
      try {
        read.resolve(response("COMPLETED"));
        await flushFocusWork();
        expect(store.getState().tasks.activeSessionId).toBe("latest");
        expect(observed).toEqual(recipientStatus);
        if (status === "metadata") {
          expect(observed).toBeUndefined();
          expect(store.getState().taskSessions.items.latest.agent_execution_id).toBe(
            RECIPIENT_EXECUTION_ID,
          );
        }
        if (status === "error") expect(observed).toMatchObject({ errorMessage: "recipient error" });
      } finally {
        release();
      }
    },
  );
});

describe("removing a retained non-primary failure", () => {
  it.each([
    ["t1", false],
    [undefined, false],
    ["t1", true],
    [undefined, true],
  ])("proves removal with task scope %s and newer pin %s", async (scope, pin) => {
    const store = withSiblings("FAILED");
    vi.mocked(fetchTaskSession).mockResolvedValue(response("FAILED"));
    promote(store);
    await flushFocusWork();
    expect(store.getState().tasks.activeSessionId).toBe("old");
    const proof = Promise.withResolvers<Awaited<ReturnType<typeof listTaskSessions>>>();
    vi.mocked(listTaskSessions).mockReturnValue(proof.promise);
    removeDisplayed(store, scope as string | undefined);
    if (pin) store.getState().setActiveSession("t1", "latest");
    proof.resolve(membership("next", "latest"));
    await flushFocusWork();
    expect(store.getState().tasks.activeSessionId).toBe(pin ? "latest" : "next");
    expect(store.getState().taskSessions.items.old).toBeUndefined();
    expect(
      store.getState().taskSessionsByTask.itemsByTaskId.t1.some((row) => row.id === "old"),
    ).toBe(false);
  });

  it.each([PROMOTION_FIRST, "proof-first"])("follows proven membership with %s", async (order) => {
    const store = withSiblings("FAILED");
    vi.mocked(fetchTaskSession).mockRejectedValue(new ApiError("", 404, null));
    const proof = Promise.withResolvers<Awaited<ReturnType<typeof listTaskSessions>>>();
    vi.mocked(listTaskSessions).mockReturnValue(proof.promise);
    removeDisplayed(store);
    if (order === PROMOTION_FIRST) promote(store);
    proof.resolve(membership("next", "latest"));
    await flushFocusWork();
    expect(store.getState().tasks.activeSessionId).toBe("next");
    if (order === "proof-first") promote(store);
    await flushFocusWork();
    expect(store.getState().tasks.activeSessionId).toBe("next");
    expect(store.getState().taskSessions.items.old).toBeUndefined();
  });

  it("admits a proven recipient before its lifecycle projection arrives", async () => {
    const store = withSiblings("FAILED");
    store.getState().removeTaskSession("t1", "next");
    const proof = membership("next", "latest");
    proof.sessions[0].last_read_message_id = "unobserved-cursor";
    vi.mocked(listTaskSessions).mockResolvedValue(proof);
    removeDisplayed(store);
    await flushFocusWork();
    const state = store.getState();
    expect(getOwnedTaskSessionId(state, "t1", state.tasks.activeSessionId)).toBe("next");
    expect(state.taskSessions.items.next.last_read_message_id).toBeUndefined();
    expect(state.taskSessions.items.old).toBeUndefined();
  });

  it("refreshes removal proof after the background recipient is deleted", async () => {
    const store = withSiblings("FAILED");
    vi.mocked(fetchTaskSession).mockResolvedValue(response("FAILED"));
    promote(store);
    await flushFocusWork();
    const oldProof = Promise.withResolvers<Awaited<ReturnType<typeof listTaskSessions>>>();
    const freshProof = Promise.withResolvers<Awaited<ReturnType<typeof listTaskSessions>>>();
    vi.mocked(listTaskSessions)
      .mockReturnValueOnce(oldProof.promise)
      .mockReturnValue(freshProof.promise);
    const selected: Array<string | null> = [];
    const release = store.subscribe((state) => selected.push(state.tasks.activeSessionId));
    try {
      removeDisplayed(store);
      invalidateTaskSessionMembership(store, "t1");
      store.getState().removeTaskSession("t1", "next");
      oldProof.resolve(membership("next", "latest"));
      await flushFocusWork();
      expect(selected).not.toContain("next");
      expect(store.getState().taskSessions.items.next).toBeUndefined();
      freshProof.resolve(membership("latest"));
      await flushFocusWork();
      expect(store.getState().tasks.activeSessionId).toBe("latest");
    } finally {
      release();
    }
  });

  it("ignores a removal explicitly scoped to another task", async () => {
    const store = withSiblings("FAILED");
    vi.mocked(listTaskSessions).mockResolvedValue(membership("next", "latest"));
    removeDisplayed(store, "foreign-task");
    await flushFocusWork();
    expect(store.getState().tasks.activeSessionId).toBe("old");
    expect(store.getState().taskSessions.items.old?.id).toBe("old");
  });
});

// @covers AC-TASKS-TASK-UNREAD-DOT-001.6
it("does not inherit readiness into an independent recipient environment", async () => {
  const store = withSiblings("RUNNING");
  store.getState().setTaskSession({
    ...store.getState().taskSessions.items.old,
    task_environment_id: "predecessor-env",
  });
  store.getState().setTaskSession({
    ...store.getState().taskSessions.items.latest,
    state: "STARTING",
    task_environment_id: "recipient-env",
  });
  store.setState({ sessionAgentctl: { itemsBySessionId: {} } });
  store.getState().setSessionAgentctlStatus("old", {
    status: "ready",
    agentExecutionId: "predecessor-execution",
    updatedAt: SNAPSHOT_TIME,
  });
  const read = deferred();
  vi.mocked(fetchTaskSession).mockReturnValue(read.promise);
  notifyState(store, "COMPLETED", SNAPSHOT_TIME);
  read.resolve(response("COMPLETED"));
  await flushFocusWork();
  expect(store.getState().tasks.activeSessionId).toBe("latest");
  expect(store.getState().sessionAgentctl.itemsBySessionId.latest).toBeUndefined();
});

// @covers AC-TASKS-TASK-UNREAD-DOT-001.11
describe("retirement without a view successor", () => {
  it.each(["empty", "terminal-only"] as const)("retires with %s membership", async (kind) => {
    const store = withSiblings("FAILED");
    const stale = store.getState().taskSessionsByTask.itemsByTaskId.t1;
    const proof = membership(...(kind === "empty" ? [] : ["next", "latest"]));
    for (const row of proof.sessions) {
      row.is_primary = false;
      row.state = "COMPLETED";
      store.getState().upsertTaskSessionFromEvent("t1", row);
    }
    vi.mocked(listTaskSessions).mockResolvedValue(proof);
    removeDisplayed(store);
    await flushFocusWork();
    expect(store.getState().tasks.activeSessionId).toBeNull();
    expect(store.getState().taskSessions.items.old).toBeUndefined();
    store.getState().setTaskSessionsForTask("t1", stale, {});
    notifyState(store, "FAILED", FAILURE_TIME);
    expect(store.getState().taskSessions.items.old).toBeUndefined();
    expect(
      store.getState().taskSessionsByTask.itemsByTaskId.t1.some((row) => row.id === "old"),
    ).toBe(false);
    expect(store.getState().tasks.activeSessionId).toBeNull();
  });

  it("retires after navigation while preserving the new task and surviving cursor", async () => {
    const store = withSiblings("FAILED");
    store.getState().setTaskSession({
      ...store.getState().taskSessions.items.next,
      last_read_message_id: "next-read",
    });
    const proof = Promise.withResolvers<Awaited<ReturnType<typeof listTaskSessions>>>();
    vi.mocked(listTaskSessions).mockReturnValue(proof.promise);
    removeDisplayed(store);
    store.getState().setActiveTask(OTHER_TASK_ID);
    proof.resolve(membership("next", "latest"));
    await flushFocusWork();
    expect(store.getState().taskSessions.items.old).toBeUndefined();
    expect(store.getState().tasks.activeTaskId).toBe(OTHER_TASK_ID);
    expect(store.getState().tasks.activeSessionId).toBeNull();
    expect(store.getState().taskSessions.items.next.last_read_message_id).toBe("next-read");
  });

  it("retires a background removal without cancelling another session's pending follow", async () => {
    const store = withSiblings("FAILED");
    store.getState().setActiveTask(OTHER_TASK_ID);
    promote(store);
    store.getState().setActiveSessionAuto("t1", "next");
    const read = deferred();
    vi.mocked(fetchTaskSession).mockReturnValue(read.promise);
    promote(store, "latest");
    vi.mocked(listTaskSessions).mockResolvedValue(membership("latest", "next"));
    removeDisplayed(store, "t1");
    await flushFocusWork();
    expect(store.getState().taskSessions.items.old).toBeUndefined();
    expect(store.getState().tasks.activeSessionId).toBe("next");
    read.resolve(response("WAITING_FOR_INPUT", "next"));
    await flushFocusWork();
    expect(store.getState().tasks.activeSessionId).toBe("latest");
  });

  it("does not retire from a prior workspace-context proof", async () => {
    const store = withSiblings("FAILED");
    const proof = Promise.withResolvers<Awaited<ReturnType<typeof listTaskSessions>>>();
    vi.mocked(listTaskSessions).mockReturnValue(proof.promise);
    removeDisplayed(store);
    store.setState({ workspaceContextGeneration: 1 });
    proof.resolve(membership("next", "latest"));
    await flushFocusWork();
    expect(store.getState().taskSessions.items.old?.id).toBe("old");
    expect(store.getState().tasks.activeSessionId).toBe("old");
  });
});
