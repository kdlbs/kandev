import { describe, expect, it, vi } from "vitest";
import type { StoreApi } from "zustand";
import type { AppState } from "@/lib/state/store";
import { createMessageUpdateScheduler } from "./messages";

const WHOLE_SECOND = "2026-08-02T00:00:01Z";

describe("scheduled message timestamp ordering", () => {
  it.each([
    [WHOLE_SECOND, "2026-08-02T00:00:01.1Z", true],
    ["2026-08-02T00:00:01.1Z", WHOLE_SECOND, false],
    ["2026-08-02T01:00:01+01:00", WHOLE_SECOND, true],
    ["2026-08-02T00:00:01.123500001Z", "2026-08-02T00:00:01.123500000Z", false],
    [WHOLE_SECOND, undefined, false],
    [WHOLE_SECOND, "invalid", false],
    [undefined, WHOLE_SECOND, true],
  ])("compares instants: current %s, incoming %s", (current, incoming, accepted) => {
    const updateMessages = vi.fn();
    const store = {
      getState: () => ({
        messages: { bySession: { session: [{ id: "message", updated_at: current }] } },
        updateMessages,
      }),
    } as unknown as StoreApi<AppState>;
    const scheduler = createMessageUpdateScheduler(store, {
      schedule: () => 1,
      cancel: vi.fn(),
    });
    scheduler.enqueue({
      message_id: "message",
      session_id: "session",
      task_id: "task",
      author_type: "agent",
      author_id: "agent",
      type: "message",
      content: "Updated content",
      created_at: "2026-08-02T00:00:00Z",
      updated_at: incoming,
    });
    scheduler.flush();
    expect(updateMessages).toHaveBeenCalledTimes(accepted ? 1 : 0);
    if (accepted)
      expect(updateMessages).toHaveBeenCalledWith([
        expect.objectContaining({ id: "message", content: "Updated content" }),
      ]);
    scheduler.dispose();
  });
});
