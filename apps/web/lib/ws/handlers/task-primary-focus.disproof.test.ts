import { beforeEach, expect, it, vi } from "vitest";
import type * as SessionApi from "@/lib/api/domains/session-api";
import { fetchTaskSession, listTaskSessions } from "@/lib/api/domains/session-api";
import { ApiError } from "@/lib/api/client";
import { observedRemovals } from "@/lib/state/session-removal-observations";
import { isRequestedSessionUnavailable } from "@/lib/state/task-navigation-selection";
import { followRemovedSession } from "./task-primary-focus";
import {
  flushFocusWork,
  membership,
  notifySibling,
  notifyState,
  promote,
  response,
  selectedStore,
  withSiblings,
} from "./task-primary-focus.test-helpers";

vi.mock("@/lib/api/domains/session-api", async (original) => ({
  ...(await original<typeof SessionApi>()),
  fetchTaskSession: vi.fn(),
  listTaskSessions: vi.fn(),
}));

beforeEach(() => {
  vi.mocked(listTaskSessions).mockReset();
  vi.mocked(fetchTaskSession).mockReset();
});

// @covers AC-TASKS-TASK-UNREAD-DOT-001.11
it.each([
  ["complete", true],
  ["incomplete", false],
] as const)(
  "withdraws a removal observation only when a %s membership still lists the session",
  async (_kind, complete) => {
    const store = withSiblings();
    const present = membership("old", "next", "latest");
    vi.mocked(listTaskSessions).mockResolvedValue(
      complete ? present : { ...present, total: present.total + 1 },
    );
    followRemovedSession(store, "next", "t1");
    expect(observedRemovals(store)?.has("next")).toBe(true);
    await flushFocusWork();
    expect(observedRemovals(store)?.has("next")).toBe(!complete);
    expect(store.getState().taskSessions.items.next).toBeDefined();
    // Explicit route/command admission uses the same unavailability predicate.
    expect(isRequestedSessionUnavailable(store, "next")).toBe(!complete);
  },
);

// @covers AC-TASKS-TASK-UNREAD-DOT-001.6
it.each([true, false])(
  "follows a later primary after the viewed session's removal is disproved (observed=%s)",
  async (observed) => {
    const store = withSiblings();
    vi.mocked(listTaskSessions).mockResolvedValue(membership("old", "next", "latest"));
    vi.mocked(fetchTaskSession).mockResolvedValue(response("WAITING_FOR_INPUT"));
    if (observed) followRemovedSession(store, "old", "t1");
    await flushFocusWork();
    expect(observedRemovals(store)?.has("old") ?? false).toBe(false);
    expect(store.getState().tasks.activeSessionId).toBe("old");
    promote(store, "next");
    await flushFocusWork();
    expect(store.getState().tasks.activeSessionId).toBe("next");
  },
);

// @covers AC-TASKS-TASK-UNREAD-DOT-001.6
it.each([true, false])(
  "keeps a primary promotion that arrives while the removal read is pending (observed=%s)",
  async (observed) => {
    const store = withSiblings();
    vi.mocked(listTaskSessions).mockResolvedValue(membership("old", "next", "latest"));
    vi.mocked(fetchTaskSession).mockResolvedValue(response("WAITING_FOR_INPUT"));
    if (observed) followRemovedSession(store, "old", "t1");
    promote(store, "next");
    await flushFocusWork();
    expect(observedRemovals(store)?.has("old") ?? false).toBe(false);
    expect(store.getState().tasks.activeSessionId).toBe("next");
  },
);

it("does not follow anything when no intent arrived during a disproved removal", async () => {
  const store = withSiblings();
  vi.mocked(listTaskSessions).mockResolvedValue(membership("next", "old", "latest"));
  vi.mocked(fetchTaskSession).mockResolvedValue(response("WAITING_FOR_INPUT"));
  followRemovedSession(store, "old", "t1");
  await flushFocusWork();
  expect(store.getState().tasks.activeSessionId).toBe("old");
  expect(fetchTaskSession).not.toHaveBeenCalled();
});

// @covers AC-TASKS-TASK-UNREAD-DOT-001.6
it.each([true, false])(
  "keeps a new-session adoption that arrives while the removal read is pending (observed=%s)",
  async (observed) => {
    const store = selectedStore("COMPLETED");
    store.getState().removeTaskSession("t1", "latest");
    vi.mocked(listTaskSessions).mockResolvedValue(membership("next", "old"));
    vi.mocked(fetchTaskSession).mockResolvedValue(response("COMPLETED"));
    if (observed) followRemovedSession(store, "old", "t1");
    notifySibling(store);
    await flushFocusWork();
    expect(store.getState().tasks.activeSessionId).toBe("latest");
  },
);

// @covers AC-TASKS-TASK-UNREAD-DOT-001.6
it.each([1, 2])(
  "resumes a primary follow after a disproved 404 (%i absent reads)",
  async (misses) => {
    const store = withSiblings();
    vi.mocked(listTaskSessions).mockResolvedValue(membership("old", "next", "latest"));
    const fetch = vi.mocked(fetchTaskSession);
    for (let i = 0; i < misses; i++) fetch.mockRejectedValueOnce(new ApiError("", 404, null));
    fetch.mockResolvedValue(response("WAITING_FOR_INPUT"));
    promote(store, "next");
    await flushFocusWork();
    // A second 404 after the disproof is not retried: fetch and list disagreement stays bounded.
    expect(fetch).toHaveBeenCalledTimes(2);
    expect(store.getState().tasks.activeSessionId).toBe(misses === 1 ? "next" : "old");
  },
);

// @covers AC-TASKS-TASK-UNREAD-DOT-001.6
it.each(["single", "duplicate"] as const)(
  "keeps an in-flight promotion when a %s removal arrives after it and is disproved",
  async (removals) => {
    const store = withSiblings();
    vi.mocked(listTaskSessions).mockResolvedValue(membership("old", "next", "latest"));
    vi.mocked(fetchTaskSession).mockResolvedValue(response("WAITING_FOR_INPUT"));
    if (removals === "duplicate") followRemovedSession(store, "old", "t1");
    promote(store, "next");
    followRemovedSession(store, "old", "t1");
    await flushFocusWork();
    expect(store.getState().tasks.activeSessionId).toBe("next");
  },
);

// @covers AC-TASKS-TASK-UNREAD-DOT-001.6
it.each([true, false])(
  "keeps the primary target when a terminal transition follows it (observed=%s)",
  async (observed) => {
    const store = withSiblings();
    vi.mocked(listTaskSessions).mockResolvedValue(membership("old", "next", "latest"));
    vi.mocked(fetchTaskSession).mockResolvedValue(response("COMPLETED"));
    if (observed) followRemovedSession(store, "old", "t1");
    promote(store, "next");
    notifyState(store, "COMPLETED", "2026-11-19T12:00:01Z");
    await flushFocusWork();
    expect(store.getState().tasks.activeSessionId).toBe("next");
  },
);
