import { describe, it, expect, beforeEach } from "vitest";
import { __getSnapshotForTests, recordForKey } from "./use-known-projects";

describe("recordForKey (gitlab)", () => {
  beforeEach(() => {
    recordForKey(`__reset__${Math.random().toString(36).slice(2)}`, []);
  });

  it("accumulates unique projects under the same key", () => {
    recordForKey("k1", ["acme/api", "acme/web"]);
    recordForKey("k1", ["acme/infra"]);
    expect(__getSnapshotForTests()).toEqual(["acme/api", "acme/infra", "acme/web"]);
  });

  it("returns sorted output", () => {
    recordForKey("k2", ["z/z", "a/a", "m/m"]);
    expect(__getSnapshotForTests()).toEqual(["a/a", "m/m", "z/z"]);
  });

  it("deduplicates across successive calls", () => {
    recordForKey("k3", ["acme/api"]);
    recordForKey("k3", ["acme/api", "acme/web"]);
    expect(__getSnapshotForTests()).toEqual(["acme/api", "acme/web"]);
  });

  it("clears accumulator when key changes", () => {
    recordForKey("k4a", ["acme/api", "acme/web"]);
    recordForKey("k4b", ["other/repo"]);
    expect(__getSnapshotForTests()).toEqual(["other/repo"]);
  });

  it("ignores empty strings", () => {
    recordForKey("k5", ["", "acme/api", ""]);
    expect(__getSnapshotForTests()).toEqual(["acme/api"]);
  });
});
