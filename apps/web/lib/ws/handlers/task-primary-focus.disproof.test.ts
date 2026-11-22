import { beforeEach, expect, it, vi } from "vitest";
import type * as SessionApi from "@/lib/api/domains/session-api";
import { listTaskSessions } from "@/lib/api/domains/session-api";
import { observedRemovals } from "@/lib/state/session-removal-observations";
import { isRequestedSessionUnavailable } from "@/lib/state/task-navigation-selection";
import { followRemovedSession } from "./task-primary-focus";
import { flushFocusWork, membership, withSiblings } from "./task-primary-focus.test-helpers";

vi.mock("@/lib/api/domains/session-api", async (original) => ({
  ...(await original<typeof SessionApi>()),
  fetchTaskSession: vi.fn(),
  listTaskSessions: vi.fn(),
}));

beforeEach(() => {
  vi.mocked(listTaskSessions).mockReset();
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
