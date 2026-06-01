import { describe, it, expect, beforeEach, vi } from "vitest";
import { appendToRing, clearRing, destroyRing, getRingBuffer } from "./ring";

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function getSnapshot(key: string): string[] {
  const buf = getRingBuffer(key);
  const result: string[] = [];
  const start = buf.size < buf.capacity ? 0 : buf.head;
  for (let i = 0; i < buf.size; i++) {
    result.push(buf.lines[(start + i) % buf.capacity]);
  }
  return result;
}

// ---------------------------------------------------------------------------
// destroyRing
// ---------------------------------------------------------------------------

describe("destroyRing", () => {
  const KEY = "test:destroy-ring";

  beforeEach(() => {
    clearRing(KEY);
  });

  it("notifies listeners with an empty snapshot before removing the entry", () => {
    appendToRing(KEY, "line 1");
    appendToRing(KEY, "line 2");

    const buf = getRingBuffer(KEY);
    const snapshots: string[][] = [];
    const listener = vi.fn(() => {
      snapshots.push(getSnapshot(KEY));
    });
    buf.listeners.add(listener);

    destroyRing(KEY);

    expect(listener).toHaveBeenCalledOnce();
    // Snapshot captured inside the listener should be empty
    expect(snapshots[0]).toEqual([]);
  });

  it("removes the map entry so subsequent getRingBuffer returns a fresh buffer", () => {
    appendToRing(KEY, "old data");
    destroyRing(KEY);

    // After destroy, getRingBuffer re-creates an empty entry
    const fresh = getRingBuffer(KEY);
    expect(fresh.size).toBe(0);
    expect(getSnapshot(KEY)).toEqual([]);
  });

  it("is a no-op for a key that was never created", () => {
    expect(() => destroyRing("nonexistent:key")).not.toThrow();
  });

  it("leaves other ring buffers untouched", () => {
    const OTHER = "test:destroy-ring-other";
    appendToRing(KEY, "mine");
    appendToRing(OTHER, "theirs");

    destroyRing(KEY);

    expect(getSnapshot(OTHER)).toEqual(["theirs"]);
    clearRing(OTHER);
  });

  it("does not fire listeners after destroy (entry is gone)", () => {
    const buf = getRingBuffer(KEY);
    const listener = vi.fn();
    buf.listeners.add(listener);

    destroyRing(KEY);
    listener.mockClear();

    // Appending to the same key re-creates the buffer — the old listener
    // is no longer attached, so it must not be called.
    appendToRing(KEY, "new data");
    expect(listener).not.toHaveBeenCalled();
  });
});
