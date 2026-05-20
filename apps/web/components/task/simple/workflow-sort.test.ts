import { describe, expect, it } from "vitest";
import { hasBlockerChain, topoSort } from "./workflow-sort";

describe("workflow sort", () => {
  it("preserves input order when no blockers exist", () => {
    const sorted = topoSort([{ id: "a" }, { id: "b" }, { id: "c" }]);
    expect(sorted.map((item) => item.id)).toEqual(["a", "b", "c"]);
  });

  it("places blocked items after their blockers", () => {
    const sorted = topoSort([
      { id: "build", blockedBy: ["design"] },
      { id: "design" },
      { id: "release", blockedBy: ["build"] },
    ]);
    expect(sorted.map((item) => item.id)).toEqual(["design", "build", "release"]);
  });

  it("ignores blockers outside the provided list", () => {
    const sorted = topoSort([{ id: "a", blockedBy: ["external"] }, { id: "b" }]);
    expect(sorted.map((item) => item.id)).toEqual(["a", "b"]);
  });

  it("detects whether any item has a blocker chain", () => {
    expect(hasBlockerChain([{ id: "a" }, { id: "b", blockedBy: ["a"] }])).toBe(true);
    expect(hasBlockerChain([{ id: "a" }, { id: "b", blockedBy: [] }])).toBe(false);
  });
});
