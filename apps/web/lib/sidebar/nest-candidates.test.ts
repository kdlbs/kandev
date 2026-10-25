import { describe, expect, it } from "vitest";
import { computeNestCandidates } from "./nest-candidates";

type T = {
  id: string;
  title: string;
  parentTaskId?: string | null;
  isFromOffice?: boolean;
};

const tasks: T[] = [
  { id: "a", title: "A" }, // root with a child
  { id: "b", title: "B", parentTaskId: "a" }, // child of A (leaf)
  { id: "d", title: "D" }, // root, leaf
  { id: "e", title: "E" }, // root, leaf
];
const OFFICE_ROOT = "office-root";
const OFFICE_CHILD = "office-child";

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

  it("returns empty when the subject is missing", () => {
    expect(computeNestCandidates(tasks, "missing")).toEqual([]);
  });
});

describe("computeNestCandidates for Office hierarchies", () => {
  // @covers AC-TASKS-SUBTASK-REPARENTING-DRAG-DROP-001.4
  it("allows deep Office targets and excludes descendants", () => {
    const officeTasks: T[] = [
      { id: OFFICE_ROOT, title: "Office root", isFromOffice: true },
      {
        id: OFFICE_CHILD,
        title: "Office child",
        parentTaskId: OFFICE_ROOT,
        isFromOffice: true,
      },
      {
        id: "office-grandchild",
        title: "Office grandchild",
        parentTaskId: OFFICE_CHILD,
        isFromOffice: true,
      },
      { id: "office-target-root", title: "Target root", isFromOffice: true },
      {
        id: "office-target-child",
        title: "Target child",
        parentTaskId: "office-target-root",
        isFromOffice: true,
      },
    ];

    expect(computeNestCandidates(officeTasks, "office-root").map((task) => task.id)).toEqual([
      "office-target-root",
      "office-target-child",
    ]);
  });

  it("uses the complete hierarchy to exclude descendants hidden by the current view", () => {
    const hierarchy: T[] = [
      { id: "subject", title: "Subject", isFromOffice: true },
      {
        id: "hidden-child",
        title: "Hidden child",
        parentTaskId: "subject",
        isFromOffice: true,
      },
      {
        id: "visible-grandchild",
        title: "Visible grandchild",
        parentTaskId: "hidden-child",
        isFromOffice: true,
      },
      { id: "valid-target", title: "Valid target", isFromOffice: true },
    ];
    const visibleTasks = hierarchy.filter((task) => task.id !== "hidden-child");

    expect(
      computeNestCandidates(visibleTasks, "subject", hierarchy).map((task) => task.id),
    ).toEqual(["valid-target"]);
  });

  it("allows an Office subject to nest under a non-Office subtask", () => {
    const mixedTasks: T[] = [
      { id: "office-subject", title: "Office subject", isFromOffice: true },
      { id: "kanban-root", title: "Kanban root" },
      { id: "kanban-child", title: "Kanban child", parentTaskId: "kanban-root" },
    ];

    expect(computeNestCandidates(mixedTasks, "office-subject").map((task) => task.id)).toContain(
      "kanban-child",
    );
  });

  it("allows a non-Office subject to nest under an Office subtask", () => {
    const mixedTasks: T[] = [
      { id: "kanban-subject", title: "Kanban subject" },
      { id: OFFICE_ROOT, title: "Office root", isFromOffice: true },
      {
        id: OFFICE_CHILD,
        title: "Office child",
        parentTaskId: OFFICE_ROOT,
        isFromOffice: true,
      },
    ];

    expect(computeNestCandidates(mixedTasks, "kanban-subject").map((task) => task.id)).toContain(
      OFFICE_CHILD,
    );
  });
});
