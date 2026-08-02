import { describe, it, expect, beforeEach } from "vitest";

import {
  DEFAULT_CAPACITY,
  MAX_ENTRY_BYTES,
  RingBuffer,
  _resetForTesting,
  clearLogs,
  getLogBuffer,
  snapshotLogs,
} from "./buffer";

describe("frontend log buffer", () => {
  beforeEach(() => {
    _resetForTesting();
  });

  it("appends entries in order", () => {
    const buf = getLogBuffer();
    buf.push({ timestamp: "t1", level: "info", source: "console", message: "a" });
    buf.push({ timestamp: "t2", level: "warn", source: "console", message: "b" });
    expect(snapshotLogs().map((e) => e.message)).toEqual(["a", "b"]);
  });

  it("evicts oldest when capacity is exceeded", () => {
    const buf = getLogBuffer();
    for (let i = 0; i < DEFAULT_CAPACITY + 5; i++) {
      buf.push({ timestamp: String(i), level: "info", source: "console", message: `m${i}` });
    }
    const snap = snapshotLogs();
    expect(snap).toHaveLength(DEFAULT_CAPACITY);
    expect(snap[0].message).toBe(`m5`);
    expect(snap[snap.length - 1].message).toBe(`m${DEFAULT_CAPACITY + 4}`);
  });

  it("snapshot is isolated from the live buffer", () => {
    const buf = getLogBuffer();
    buf.push({ timestamp: "t", level: "info", source: "console", message: "x" });
    const snap = snapshotLogs();
    snap[0].message = "mutated";
    expect(snapshotLogs()[0].message).toBe("x");
  });

  it("snapshot deep-copies the args array", () => {
    const buf = getLogBuffer();
    buf.push({
      timestamp: "t",
      level: "info",
      source: "console",
      message: "x",
      args: ["a", "b"],
    });
    const snap = snapshotLogs();
    snap[0].args!.push("mutated");
    expect(snapshotLogs()[0].args).toEqual(["a", "b"]);
  });

  it("clearLogs empties the buffer", () => {
    const buf = getLogBuffer();
    buf.push({ timestamp: "t", level: "info", source: "console", message: "x" });
    clearLogs();
    expect(snapshotLogs()).toHaveLength(0);
  });

  it("partitions snapshots by authenticated identity", () => {
    const buf = getLogBuffer();
    buf.push({
      timestamp: "t1",
      level: "info",
      source: "console",
      message: "alice",
      identity_scope: "alice",
    });
    buf.push({
      timestamp: "t2",
      level: "info",
      source: "console",
      message: "bob",
      identity_scope: "bob",
    });
    expect(snapshotLogs("alice").map((entry) => entry.message)).toEqual(["alice"]);
  });

  it("rejects an entry over 64 KiB and sheds low-priority entries first", () => {
    const buf = new RingBuffer(2, MAX_ENTRY_BYTES * 2);
    expect(
      buf.push({
        timestamp: "t",
        level: "info",
        source: "console",
        message: "x".repeat(MAX_ENTRY_BYTES),
      }),
    ).toBe(false);
    buf.push({ timestamp: "1", level: "warn", source: "console", message: "warn" });
    buf.push({ timestamp: "2", level: "debug", source: "console", message: "debug" });
    buf.push({ timestamp: "3", level: "error", source: "console", message: "error" });
    expect(buf.snapshot().map((entry) => entry.message)).toEqual(["warn", "error"]);
    expect(buf.statistics()).toEqual({ capacity: 1, entry_too_large: 1 });
  });
});
