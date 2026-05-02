import { describe, it, expect, beforeEach } from "vitest";

import {
  DEFAULT_CAPACITY,
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
});
