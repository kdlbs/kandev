import { describe, expect, it } from "vitest";
import type { Message } from "@/lib/types/http";
import { sessionId, taskId } from "@/lib/types/ids";
import { reconcileLatestMessageWindow } from "./message-window-reconciliation";

function message(id: string, minute: number, fraction = ""): Message {
  return {
    id,
    task_id: taskId("task-1"),
    session_id: sessionId("session-1"),
    author_type: "agent",
    content: id,
    type: "message",
    created_at: `2026-08-30T09:${String(minute).padStart(2, "0")}:00${fraction}Z`,
  } as Message;
}

describe("reconcileLatestMessageWindow", () => {
  // @covers AC-UI-TASK-PROMPT-TRANSCRIPT-VISIBILITY-001.10
  it("drops a disjoint stale prefix and retains a row received during the request", () => {
    const stale = [message("stale-1", 0), message("stale-2", 1)];
    const fetched = [message("fetched-3", 3), message("fetched-4", 4)];
    const live = message("live-5", 5);

    const result = reconcileLatestMessageWindow({
      cachedAtRequest: stale,
      cachedAtResponse: [...stale, live],
      fetched,
    });

    expect(result.messages.map(({ id }) => id)).toEqual(["fetched-3", "fetched-4", "live-5"]);
    expect(result.oldestCursor).toBe("fetched-3");
  });

  it("keeps cached older pages when the fetched suffix overlaps them", () => {
    const cached = [message("cached-1", 1), message("shared-3", 3)];
    const fetched = [message("shared-3", 3), message("fetched-4", 4)];
    const live = message("live-6", 6);

    const result = reconcileLatestMessageWindow({
      cachedAtRequest: cached,
      cachedAtResponse: [...cached, live],
      fetched,
    });

    expect(result.messages.map(({ id }) => id)).toEqual([
      "cached-1",
      "shared-3",
      "fetched-4",
      "live-6",
    ]);
    expect(result.oldestCursor).toBe("cached-1");
  });

  it("leaves an older live row for the next page when the fetched window is disjoint", () => {
    const stale = [message("stale-1", 0), message("stale-2", 1)];
    const fetched = [message("fetched-3", 3), message("fetched-4", 4)];
    const olderLive = message("live-old-2", 2);
    const newerLive = message("live-5", 5);

    const result = reconcileLatestMessageWindow({
      cachedAtRequest: stale,
      cachedAtResponse: [...stale, olderLive, newerLive],
      fetched,
    });

    expect(result.messages.map(({ id }) => id)).toEqual(["fetched-3", "fetched-4", "live-5"]);
    expect(result.oldestCursor).toBe("fetched-3");
  });

  it("sorts timestamps using sub-millisecond precision before the id tie-breaker", () => {
    const earlier = message("z-earlier", 0, ".000001");
    const later = message("a-later", 0, ".000002");

    const result = reconcileLatestMessageWindow({
      cachedAtRequest: [],
      cachedAtResponse: [],
      fetched: [later, earlier],
    });

    expect(result.messages.map(({ id }) => id)).toEqual(["z-earlier", "a-later"]);
  });

  it("uses the id tie-breaker after truncating timestamps to backend microseconds", () => {
    const first = message("z-tie", 0, ".0000011");
    const second = message("a-tie", 0, ".0000012");

    const result = reconcileLatestMessageWindow({
      cachedAtRequest: [],
      cachedAtResponse: [],
      fetched: [first, second],
    });

    expect(result.messages.map(({ id }) => id)).toEqual(["a-tie", "z-tie"]);
  });

  it("keeps the current cache when an empty response has no replacement boundary", () => {
    const cached = [message("cached-1", 1)];

    expect(
      reconcileLatestMessageWindow({
        cachedAtRequest: cached,
        cachedAtResponse: cached,
        fetched: [],
      }),
    ).toEqual({ messages: cached, oldestCursor: "cached-1" });
  });
});
