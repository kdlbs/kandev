import { describe, it, expect } from "vitest";
import { QueueFullError, QueueEntryNotFoundError, rethrowQueueError } from "./queue-api";

describe("rethrowQueueError", () => {
  it("maps queue_full errors to QueueFullError carrying the cap metadata", () => {
    expect(() =>
      rethrowQueueError({
        code: "queue_full",
        message: "Queue is full",
        details: { queue_size: 7, max: 10 },
      }),
    ).toThrow(QueueFullError);

    try {
      rethrowQueueError({
        code: "queue_full",
        details: { queue_size: 9, max: 10 },
      });
    } catch (err) {
      const qf = err as QueueFullError;
      expect(qf.queueSize).toBe(9);
      expect(qf.max).toBe(10);
      expect(qf.code).toBe("queue_full");
    }
  });

  it("defaults missing queue_size / max to 0 when details are sparse", () => {
    try {
      rethrowQueueError({ code: "queue_full" });
    } catch (err) {
      const qf = err as QueueFullError;
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
