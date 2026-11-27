/* eslint-disable sonarjs/no-duplicate-string -- Wire action and fixture IDs are intentionally repeated for readability. */
import { beforeEach, describe, it, expect, vi } from "vitest";
import { WebSocketRequestError } from "@/lib/ws/request-error";

const getWebSocketClientMock = vi.hoisted(() => vi.fn());
const SESSION_ID = `session-1`;
const INCARNATION_ID = `incarnation-1`;

vi.mock("@/lib/ws/connection", () => ({
  getWebSocketClient: getWebSocketClientMock,
}));

import {
  MergeReferenceOverflowError,
  QueueFullError,
  QueueEntryNotFoundError,
  QueueEditConflictError,
  beginQueuedMessageEdit,
  endQueuedMessageEdit,
  mergeQueuedEntry,
  queueMessage,
  renewQueuedMessageEdit,
  reorderQueuedEntries,
  rethrowQueueError,
  sendQueuedNow,
  updateQueuedMessage,
} from "./queue-api";
import * as queueApi from "./queue-api";

beforeEach(() => {
  getWebSocketClientMock.mockReset();
});

describe("bounded queue mutation admission", () => {
  const identity = {
    task_id: "task-1",
    session_id: SESSION_ID,
    session_incarnation_id: INCARNATION_ID,
  };
  const operations = [
    { action: "message.queue.add", invoke: () => queueMessage({ ...identity, content: "queued" }) },
    {
      action: "message.queue.append",
      invoke: () => queueApi.appendToQueue({ ...identity, content: "appended" }),
    },
    {
      action: "message.queue.update",
      invoke: () => updateQueuedMessage({ ...identity, entry_id: "entry", content: "edited" }),
    },
    {
      action: "message.queue.edit.begin",
      invoke: () => beginQueuedMessageEdit(SESSION_ID, "entry"),
    },
    {
      action: "message.queue.edit.renew",
      invoke: () =>
        renewQueuedMessageEdit({ session_id: SESSION_ID, entry_id: "entry", lease_id: "lease" }),
    },
    {
      action: "message.queue.edit.end",
      invoke: () =>
        endQueuedMessageEdit({ session_id: SESSION_ID, entry_id: "entry", lease_id: "lease" }),
    },
    { action: "message.queue.drain", invoke: () => queueApi.drainQueuedMessage(identity) },
    {
      action: "message.queue.send_now",
      invoke: () => sendQueuedNow({ ...identity, scope: "all" }),
    },
    { action: "message.queue.cancel", invoke: () => queueApi.clearQueue(identity) },
    {
      action: "message.queue.auto_run.set",
      invoke: () => queueApi.setQueueAutoRun(identity, false),
    },
    {
      action: "message.queue.auto_merge.set",
      invoke: () => queueApi.setQueueAutoMerge(identity, false),
    },
    {
      action: "message.queue.remove",
      invoke: () => queueApi.removeQueuedEntry({ ...identity, entry_id: "entry" }),
    },
    {
      action: "message.queue.merge",
      invoke: () => mergeQueuedEntry({ ...identity, entry_id: "entry" }),
    },
    {
      action: "message.queue.reorder",
      invoke: () => reorderQueuedEntries({ ...identity, ordered_ids: ["entry"] }),
    },
  ];

  it.each(operations)("$action preserves the retryable wire error", async ({ invoke }) => {
    const request = vi.fn().mockRejectedValue(new WebSocketRequestError("busy", "UNAVAILABLE"));
    getWebSocketClientMock.mockReturnValue({ request });
    await expect(invoke()).rejects.toMatchObject({
      name: "QueueAdmissionError",
      code: "unavailable",
    });
  });

  const mutationOperations = operations.filter(
    ({ action }) =>
      !["message.queue.add", "message.queue.auto_run.set", "message.queue.remove"].includes(action),
  );
  it.each(mutationOperations)(
    "$action retains non-admission wire error details",
    async ({ invoke }) => {
      const original = new WebSocketRequestError("Invalid mutation", "VALIDATION_ERROR", {
        field: "content",
      });
      const request = vi.fn().mockRejectedValue(original);
      getWebSocketClientMock.mockReturnValue({ request });
      await expect(invoke()).rejects.toBe(original);
    },
  );

  it.each(operations)(
    "$action waits for the server admission budget",
    async ({ action, invoke }) => {
      const request = vi.fn().mockResolvedValue({});
      getWebSocketClientMock.mockReturnValue({ request });
      await invoke();
      expect(request).toHaveBeenCalledWith(
        action,
        expect.objectContaining({ session_id: SESSION_ID }),
        35_000,
      );
    },
  );
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

  it("maps edit lease conflicts to QueueEditConflictError", () => {
    expect(() =>
      rethrowQueueError({
        code: "edit_conflict",
        message: "Edit lease expired",
      }),
    ).toThrow(QueueEditConflictError);
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
});

describe("rethrowQueueError values", () => {
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
