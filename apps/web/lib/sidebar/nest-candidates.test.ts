import { describe, expect, it } from "vitest";
import { computeNestCandidates } from "./nest-candidates";

type T = { id: string; title: string; parentTaskId?: string | null };

const tasks: T[] = [
  { id: "a", title: "A" },
  { id: "b", title: "B", parentTaskId: "a" }, // child of A
  { id: "c", title: "C", parentTaskId: "b" }, // grandchild of A
  { id: "d", title: "D" },
  { id: "e", title: "E" },
];

describe("computeNestCandidates", () => {
  it("excludes the task itself", () => {
    const ids = computeNestCandidates(tasks, "d").map((t) => t.id);
    expect(ids).not.toContain("d");
  });

  it("excludes descendants to prevent cycles", () => {
    // A's descendants are B and C; nesting A under either would cycle.
    const ids = computeNestCandidates(tasks, "a").map((t) => t.id);
    expect(ids).not.toContain("b");
    expect(ids).not.toContain("c");
    expect(ids).toEqual(expect.arrayContaining(["d", "e"]));
  });

  it("excludes the current parent (already nested there)", () => {
    // B is already nested under A, so A is not offered as a candidate.
    const ids = computeNestCandidates(tasks, "b").map((t) => t.id);
    expect(ids).not.toContain("a");
    // B's own descendant C is also excluded.
    expect(ids).not.toContain("c");
    expect(ids).toEqual(expect.arrayContaining(["d", "e"]));
  });

  it("returns empty when the task is the only one", () => {
    expect(computeNestCandidates([{ id: "solo", title: "Solo" }], "solo")).toEqual([]);
  });
});
