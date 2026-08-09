import { beforeEach, describe, it, expect, vi } from "vitest";
import type { EntityReference } from "@/lib/types/entity-reference";

const getWebSocketClientMock = vi.hoisted(() => vi.fn());

vi.mock("@/lib/ws/connection", () => ({
  getWebSocketClient: getWebSocketClientMock,
}));

import {
  MergeReferenceOverflowError,
  QueueSendNowError,
  QueueFullError,
  QueueEntryNotFoundError,
  QueueReorderError,
  mergeQueuedEntry,
  queueMessage,
  reorderQueuedEntries,
  rethrowQueueError,
  sendQueuedNow,
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

  it("maps merge_reference_overflow errors to MergeReferenceOverflowError", () => {
    expect(() =>
      rethrowQueueError({
        code: "merge_reference_overflow",
        message: "merge would exceed the per-message entity reference limit",
      }),
    ).toThrow(MergeReferenceOverflowError);
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

  it("forwards context file metadata through message.queue.add", async () => {
    const request = vi.fn().mockResolvedValue({ id: "q-1" });
    getWebSocketClientMock.mockReturnValue({ request });

    await queueMessage({
      session_id: "session-1",
      task_id: "task-1",
      content: "queued context",
      context_files: [{ path: "src/components", name: "components", is_directory: true }],
    });

    expect(request).toHaveBeenCalledWith("message.queue.add", {
      session_id: "session-1",
      task_id: "task-1",
      content: "queued context",
      context_files: [{ path: "src/components", name: "components", is_directory: true }],
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

describe("mergeQueuedEntry", () => {
  it("forwards session, entry, and caller identity through message.queue.merge", async () => {
    const request = vi.fn().mockResolvedValue({ entry_id: "q-a" });
    getWebSocketClientMock.mockReturnValue({ request });

    await mergeQueuedEntry({
      session_id: "session-1",
      entry_id: "q-b",
      user_id: "user-1",
    });

    expect(request).toHaveBeenCalledWith("message.queue.merge", {
      session_id: "session-1",
      entry_id: "q-b",
      user_id: "user-1",
    });
  });

  it("omits user_id when the caller identity is unknown", async () => {
    const request = vi.fn().mockResolvedValue({ entry_id: "q-a" });
    getWebSocketClientMock.mockReturnValue({ request });

    await mergeQueuedEntry({ session_id: "session-1", entry_id: "q-b" });

    expect(request).toHaveBeenCalledWith("message.queue.merge", {
      session_id: "session-1",
      entry_id: "q-b",
    });
  });

  it("maps a drained-source rejection to QueueEntryNotFoundError", async () => {
    const request = vi
      .fn()
      .mockRejectedValue({ code: "entry_not_found", message: "Already drained" });
    getWebSocketClientMock.mockReturnValue({ request });

    await expect(
      mergeQueuedEntry({ session_id: "session-1", entry_id: "q-b" }),
    ).rejects.toBeInstanceOf(QueueEntryNotFoundError);
  });

  it("maps a reference-overflow rejection to MergeReferenceOverflowError", async () => {
    const request = vi.fn().mockRejectedValue({
      code: "merge_reference_overflow",
      message: "merge would exceed the per-message entity reference limit",
    });
    getWebSocketClientMock.mockReturnValue({ request });

    await expect(
      mergeQueuedEntry({ session_id: "session-1", entry_id: "q-b" }),
    ).rejects.toBeInstanceOf(MergeReferenceOverflowError);
  });
});

describe("sendQueuedNow", () => {
  it("sends an exact entry scope", async () => {
    const request = vi.fn().mockResolvedValue({
      session_id: "session-1",
      dispatched: true,
      sent_count: 1,
    });
    getWebSocketClientMock.mockReturnValue({ request });

    await sendQueuedNow({
      session_id: "session-1",
      scope: "entry",
      entry_id: "q-2",
    });

    expect(request).toHaveBeenCalledWith("message.queue.send_now", {
      session_id: "session-1",
      scope: "entry",
      entry_id: "q-2",
    });
  });

  it("omits entry_id for an all scope snapshot", async () => {
    const request = vi.fn().mockResolvedValue({
      session_id: "session-1",
      dispatched: true,
      sent_count: 3,
    });
    getWebSocketClientMock.mockReturnValue({ request });

    await sendQueuedNow({ session_id: "session-1", scope: "all" });

    expect(request).toHaveBeenCalledWith("message.queue.send_now", {
      session_id: "session-1",
      scope: "all",
    });
  });

  it("maps send-now conflict codes to a typed error", async () => {
    const request = vi.fn().mockRejectedValue({
      code: "send_now_conflict",
      message: "Another cancellation is in progress",
    });
    getWebSocketClientMock.mockReturnValue({ request });

    await expect(sendQueuedNow({ session_id: "session-1", scope: "all" })).rejects.toMatchObject({
      code: "send_now_conflict",
    });
    await expect(sendQueuedNow({ session_id: "session-1", scope: "all" })).rejects.toBeInstanceOf(
      QueueSendNowError,
    );
  });
});

describe("reorderQueuedEntries", () => {
  it("sends the full ordered id list through message.queue.reorder", async () => {
    const request = vi.fn().mockResolvedValue({ session_id: "session-1", reordered: 3 });
    getWebSocketClientMock.mockReturnValue({ request });

    await reorderQueuedEntries({
      session_id: "session-1",
      ordered_ids: ["q-c", "q-a", "q-b"],
    });

    expect(request).toHaveBeenCalledWith("message.queue.reorder", {
      session_id: "session-1",
      ordered_ids: ["q-c", "q-a", "q-b"],
    });
  });

  it("maps a queue_changed rejection to QueueReorderError", async () => {
    const request = vi.fn().mockRejectedValue({
      code: "queue_changed",
      message: "Queue changed before the reorder could be applied",
    });
    getWebSocketClientMock.mockReturnValue({ request });

    await expect(
      reorderQueuedEntries({ session_id: "session-1", ordered_ids: ["q-a"] }),
    ).rejects.toBeInstanceOf(QueueReorderError);
  });

  it("falls through to the shared error mapping for other failures", async () => {
    const request = vi.fn().mockRejectedValue({ code: "entry_not_found", message: "Gone" });
    getWebSocketClientMock.mockReturnValue({ request });

    await expect(
      reorderQueuedEntries({ session_id: "session-1", ordered_ids: ["q-a"] }),
    ).rejects.toBeInstanceOf(QueueEntryNotFoundError);
  });
});
