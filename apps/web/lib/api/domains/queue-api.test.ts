import { beforeEach, describe, it, expect, vi } from "vitest";
import type { EntityReference } from "@/lib/types/entity-reference";

const getWebSocketClientMock = vi.hoisted(() => vi.fn());

vi.mock("@/lib/ws/connection", () => ({
  getWebSocketClient: getWebSocketClientMock,
}));

import {
  QueueFullError,
  QueueEntryNotFoundError,
  queueMessage,
  rethrowQueueError,
  updateQueuedMessage,
} from "./queue-api";

const reference: EntityReference = {
  version: 1,
  ref: "mention:v1:github:issue:acme%2Frepo:42",
  provider: "github",
  kind: "issue",
  id: "42",
  key: "acme/repo#42",
  title: "Fix composer references",
  url: "https://github.com/acme/repo/issues/42",
  scope: "acme/repo",
};

beforeEach(() => {
  getWebSocketClientMock.mockReset();
});

describe("rethrowQueueError", () => {
  it("maps queue_full errors to QueueFullError carrying the cap metadata", () => {
    expect(() =>
      rethrowQueueError({
        code: "queue_full",
        message: "Queue is full",
        details: { queue_size: 7, max: 10 },
      }),
    ).toThrow(QueueFullError);

    expect.assertions(5);
    try {
      rethrowQueueError({
        code: "queue_full",
        details: { queue_size: 9, max: 10 },
      });
    } catch (err) {
      const qf = err as QueueFullError;
      expect(qf).toBeInstanceOf(QueueFullError);
      expect(qf.queueSize).toBe(9);
      expect(qf.max).toBe(10);
      expect(qf.code).toBe("queue_full");
    }
  });

  it("defaults missing queue_size / max to 0 when details are sparse", () => {
    expect.assertions(3);
    try {
      rethrowQueueError({ code: "queue_full" });
    } catch (err) {
      const qf = err as QueueFullError;
      expect(qf).toBeInstanceOf(QueueFullError);
      expect(qf.queueSize).toBe(0);
      expect(qf.max).toBe(0);
    }
  });

  it("maps entry_not_found errors to QueueEntryNotFoundError", () => {
    expect(() =>
      rethrowQueueError({
        code: "entry_not_found",
        message: "Already drained",
      }),
    ).toThrow(QueueEntryNotFoundError);
  });

  it("rethrows non-queue WS errors as plain Error instances", () => {
    let caught: unknown;
    try {
      rethrowQueueError({ code: "internal_error", message: "Boom" });
    } catch (err) {
      caught = err;
    }
    expect(caught).toBeInstanceOf(Error);
    expect((caught as Error).message).toContain("Boom");
  });

  it("preserves Error instances supplied by the WS client", () => {
    const original = new Error("boom");
    let caught: unknown;
    try {
      rethrowQueueError(original);
    } catch (err) {
      caught = err;
    }
    expect(caught).toBe(original);
  });

  it("wraps non-Error non-WSError values in an Error so callers can rely on stack traces", () => {
    let caught: unknown;
    try {
      rethrowQueueError("just a string");
    } catch (err) {
      caught = err;
    }
    expect(caught).toBeInstanceOf(Error);
    expect((caught as Error).message).toBe("just a string");
  });
});

describe("queue reference payloads", () => {
  it("forwards entity references through message.queue.add", async () => {
    const request = vi.fn().mockResolvedValue({ id: "q-1" });
    getWebSocketClientMock.mockReturnValue({ request });

    await queueMessage({
      session_id: "session-1",
      task_id: "task-1",
      content: "queued reference",
      entity_references: [reference],
    });

    expect(request).toHaveBeenCalledWith("message.queue.add", {
      session_id: "session-1",
      task_id: "task-1",
      content: "queued reference",
      entity_references: [reference],
    });
  });

  it("sends an explicit empty reference array when replacing a queued message", async () => {
    const request = vi.fn().mockResolvedValue({ entry_id: "q-1" });
    getWebSocketClientMock.mockReturnValue({ request });

    await updateQueuedMessage({
      session_id: "session-1",
      entry_id: "q-1",
      content: "reference removed",
    } as never);

    expect(request).toHaveBeenCalledWith("message.queue.update", {
      session_id: "session-1",
      entry_id: "q-1",
      content: "reference removed",
      entity_references: [],
    });
  });

  it("forwards surviving references through message.queue.update", async () => {
    const request = vi.fn().mockResolvedValue({ entry_id: "q-1" });
    getWebSocketClientMock.mockReturnValue({ request });

    await updateQueuedMessage({
      session_id: "session-1",
      entry_id: "q-1",
      content: "reference kept",
      entity_references: [reference],
    });

    expect(request).toHaveBeenCalledWith("message.queue.update", {
      session_id: "session-1",
      entry_id: "q-1",
      content: "reference kept",
      entity_references: [reference],
    });
  });
});
