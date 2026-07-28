import { describe, expect, it } from "vitest";
import { computeNestCandidates } from "./nest-candidates";

type T = { id: string; title: string; parentTaskId?: string | null };

const tasks: T[] = [
  { id: "a", title: "A" }, // root with a child
  { id: "b", title: "B", parentTaskId: "a" }, // child of A (leaf)
  { id: "d", title: "D" }, // root, leaf
  { id: "e", title: "E" }, // root, leaf
];

describe("computeNestCandidates", () => {
  it("excludes the task itself", () => {
    const ids = computeNestCandidates(tasks, "d").map((t) => t.id);
    expect(ids).not.toContain("d");
    expect(ids).toEqual(expect.arrayContaining(["a", "e"]));
  });

  it("returns no candidates when the task already has children", () => {
    // A has child B; nesting A under anything would push B to depth 2,
    // exceeding the one-level kanban subtask limit.
    expect(computeNestCandidates(tasks, "a")).toEqual([]);
  });

  it("excludes tasks that are already subtasks (would create a grandchild)", () => {
    // B is a subtask, so nesting D under B would exceed the one-level limit.
    const ids = computeNestCandidates(tasks, "d").map((t) => t.id);
    expect(ids).not.toContain("b");
  });

  it("excludes the current parent (already nested there)", () => {
    // B is already nested under A, so A is not offered as a candidate.
    const ids = computeNestCandidates(tasks, "b").map((t) => t.id);
    expect(ids).not.toContain("a");
    expect(ids).toEqual(expect.arrayContaining(["d", "e"]));
  });

  it("only offers roots, so cycles can never be introduced", () => {
    const deep: T[] = [
      { id: "a", title: "A" },
      { id: "b", title: "B", parentTaskId: "a" },
      { id: "c", title: "C", parentTaskId: "b" },
    ];
    // A has children, so no candidates are offered at all.
    expect(computeNestCandidates(deep, "a")).toEqual([]);
    // C (a leaf grandchild) can only be re-nested under a root: A qualifies
    // (A -> C stays one level), while B is a subtask and is excluded.
    expect(computeNestCandidates(deep, "c").map((t) => t.id)).toEqual(["a"]);
  });

  it("returns empty when the task is the only one", () => {
    expect(computeNestCandidates([{ id: "solo", title: "Solo" }], "solo")).toEqual([]);
  });
});
