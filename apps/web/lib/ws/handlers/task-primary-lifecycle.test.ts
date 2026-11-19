import { beforeEach, describe, expect, it, vi } from "vitest";
import { fetchTaskSession, listTaskSessions } from "@/lib/api/domains/session-api";
import type * as SessionApi from "@/lib/api/domains/session-api";
import { ApiError } from "@/lib/api/client";
import { getOwnedTaskSessionId } from "@/src/task-route-projection";
import { invalidateTaskSessionMembership } from "@/lib/state/task-session-reads";
import { registerTaskSessionHandlers } from "./agent-session";
import {
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
    notifyState(store, "STARTING", "2026-11-19T12:00:01Z");
    notifyState(store, "FAILED", "2026-11-19T12:00:02Z");
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
    notifyState(store, "STARTING", "2026-11-19T12:00:01Z");
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
    if (!pin) expect(store.getState().taskSessions.items.old).toBeUndefined();
  });

  it.each(["promotion-first", "proof-first"])(
    "follows proven membership with %s",
    async (order) => {
      const store = withSiblings("FAILED");
      vi.mocked(fetchTaskSession).mockRejectedValue(new ApiError("", 404, null));
      const proof = Promise.withResolvers<Awaited<ReturnType<typeof listTaskSessions>>>();
      vi.mocked(listTaskSessions).mockReturnValue(proof.promise);
      removeDisplayed(store);
      if (order === "promotion-first") promote(store);
      proof.resolve(membership("next", "latest"));
      await flushFocusWork();
      expect(store.getState().tasks.activeSessionId).toBe("next");
      if (order === "proof-first") promote(store);
      await flushFocusWork();
      expect(store.getState().tasks.activeSessionId).toBe("next");
      expect(store.getState().taskSessions.items.old).toBeUndefined();
    },
  );

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
