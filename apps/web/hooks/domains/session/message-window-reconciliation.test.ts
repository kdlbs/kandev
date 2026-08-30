import { describe, expect, it } from "vitest";
import type { Message } from "@/lib/types/http";
import { sessionId, taskId } from "@/lib/types/ids";
import { reconcileLatestMessageWindow } from "./message-window-reconciliation";

function message(id: string, minute: number): Message {
  return {
    id,
    task_id: taskId("task-1"),
    session_id: sessionId("session-1"),
    author_type: "agent",
    content: id,
    type: "message",
    created_at: `2026-08-30T09:${String(minute).padStart(2, "0")}:00Z`,
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

    const result = reconcileLatestMessageWindow({
      cachedAtRequest: cached,
      cachedAtResponse: cached,
      fetched,
    });

    expect(result.messages.map(({ id }) => id)).toEqual(["cached-1", "shared-3", "fetched-4"]);
    expect(result.oldestCursor).toBe("cached-1");
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
