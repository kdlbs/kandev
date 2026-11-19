import { beforeEach, describe, expect, it, vi } from "vitest";
import { fetchTaskSession, listTaskSessions } from "@/lib/api/domains/session-api";
import type * as SessionApi from "@/lib/api/domains/session-api";
import { ApiError } from "@/lib/api/client";
import { getOwnedTaskSessionId } from "@/src/task-route-projection";
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
function removeDisplayed(store: ReturnType<typeof withSiblings>, taskId?: string) {
  registerTaskSessionHandlers(store)[REMOVED]?.({
    id: "removed",
    type: "notification",
    action: REMOVED,
    payload: { session_id: "old", ...(taskId === undefined ? {} : { task_id: taskId }) },
  });
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

  it("ignores a removal explicitly scoped to another task", async () => {
    const store = withSiblings("FAILED");
    vi.mocked(listTaskSessions).mockResolvedValue(membership("next", "latest"));
    removeDisplayed(store, "foreign-task");
    await flushFocusWork();
    expect(store.getState().tasks.activeSessionId).toBe("old");
    expect(store.getState().taskSessions.items.old?.id).toBe("old");
  });
});
