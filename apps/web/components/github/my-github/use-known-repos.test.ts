import { describe, it, expect, beforeEach } from "vitest";
import { __getSnapshotForTests, recordForKey } from "./use-known-repos";

describe("recordForKey", () => {
  beforeEach(() => {
    // Each test starts with a fresh key; since recordForKey clears when the
    // key changes, this resets the module-level accumulator.
    recordForKey(`__reset__${Math.random().toString(36).slice(2)}`, []);
  });

  it("accumulates unique repos under the same key", () => {
    recordForKey("k1", ["a/b", "c/d"]);
    recordForKey("k1", ["e/f"]);
    expect(__getSnapshotForTests()).toEqual(["a/b", "c/d", "e/f"]);
  });

  it("returns sorted output", () => {
    recordForKey("k2", ["z/z", "a/a", "m/m"]);
    expect(__getSnapshotForTests()).toEqual(["a/a", "m/m", "z/z"]);
  });

  it("deduplicates across successive calls with the same key", () => {
    recordForKey("k3", ["a/b"]);
    recordForKey("k3", ["a/b", "c/d"]);
    expect(__getSnapshotForTests()).toEqual(["a/b", "c/d"]);
  });

  it("clears accumulator when key changes", () => {
    recordForKey("k4a", ["a/a", "b/b"]);
    recordForKey("k4b", ["x/x"]);
    expect(__getSnapshotForTests()).toEqual(["x/x"]);
  });

  it("ignores empty strings", () => {
    recordForKey("k5", ["", "a/a", ""]);
    expect(__getSnapshotForTests()).toEqual(["a/a"]);
  });

  it("empty input under an existing key is a no-op", () => {
    recordForKey("k6", ["a/a"]);
    recordForKey("k6", []);
    expect(__getSnapshotForTests()).toEqual(["a/a"]);
  });
});
