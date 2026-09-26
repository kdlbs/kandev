import { describe, expect, it } from "vitest";
import {
  computeNestTargets,
  nestDroppableId,
  parseNestDroppableId,
  resolveSidebarDrop,
} from "./task-switcher-subtask-dnd";

// Fixture ids hoisted so the repeated literals stay below sonarjs's
// no-duplicate-string threshold.
const ROOT_A = "a";
const ROOT_B = "b";
const SUB_C = "c";
const SUB_D = "d";
const SUB_E = "e";
const WORKFLOW = "wf-1";
const OTHER_WORKFLOW = "wf-2";
const PWC = "parent-with-child"; // root that already has a child
const PWC_CHILD = "child-of-pwc";
const OFFICE_SUBJECT = "office-subject";
const OFFICE_TARGET_ROOT = "office-target-root";
const HIDDEN_CHILD = "hidden-child";

function task(
  id: string,
  overrides: {
    workflowId?: string;
    parentTaskId?: string | null;
    isFromOffice?: boolean;
  } = {},
) {
  return {
    id,
    title: id,
    workflowId: overrides.workflowId ?? WORKFLOW,
    parentTaskId: overrides.parentTaskId ?? undefined,
    isFromOffice: overrides.isFromOffice,
  };
}

describe("nestDroppableId / parseNestDroppableId", () => {
  it("round-trips task ids through the nest prefix", () => {
    expect(parseNestDroppableId(nestDroppableId("task-1"))).toBe("task-1");
  });

  it("returns null for non-nest droppable ids", () => {
    expect(parseNestDroppableId("task-1")).toBeNull();
  });
});

describe("resolveSidebarDrop", () => {
  const roots = [ROOT_A, ROOT_B];
  const childrenByParent = new Map<string, string[]>([
    [ROOT_A, [SUB_C, SUB_D]],
    [ROOT_B, [SUB_E]],
  ]);

  it("maps a drop on a nest zone to a re-parent", () => {
    expect(
      resolveSidebarDrop({
        activeId: SUB_C,
        overId: nestDroppableId(ROOT_B),
        groupRootIds: roots,
        childrenByParent,
      }),
    ).toEqual({ kind: "nest", parentTaskId: ROOT_B });
  });

  it("rejects nesting under the dragged task itself", () => {
    expect(
      resolveSidebarDrop({
        activeId: ROOT_A,
        overId: nestDroppableId(ROOT_A),
        groupRootIds: roots,
        childrenByParent,
      }),
    ).toBeNull();
  });

  it("reorders within the group root level", () => {
    expect(
      resolveSidebarDrop({
        activeId: ROOT_A,
        overId: ROOT_B,
        groupRootIds: roots,
        childrenByParent,
      }),
    ).toEqual({ kind: "reorder-group", orderedTaskIds: [ROOT_B, ROOT_A] });
  });

  it("reorders within one parent's children", () => {
    expect(
      resolveSidebarDrop({
        activeId: SUB_C,
        overId: SUB_D,
        groupRootIds: roots,
        childrenByParent,
      }),
    ).toEqual({ kind: "reorder-subtasks", parentTaskId: ROOT_A, orderedTaskIds: [SUB_D, SUB_C] });
  });

  it("rejects cross-level drops (subtask onto a sibling root)", () => {
    expect(
      resolveSidebarDrop({
        activeId: SUB_C,
        overId: ROOT_B,
        groupRootIds: roots,
        childrenByParent,
      }),
    ).toBeNull();
  });

  it("rejects a drop on the dragged task's own row", () => {
    expect(
      resolveSidebarDrop({
        activeId: SUB_C,
        overId: SUB_C,
        groupRootIds: roots,
        childrenByParent,
      }),
    ).toBeNull();
  });

  it("rejects a drop with no over target", () => {
    expect(
      resolveSidebarDrop({
        activeId: SUB_C,
        overId: null,
        groupRootIds: roots,
        childrenByParent,
      }),
    ).toBeNull();
  });
});

describe("computeNestTargets", () => {
  const tasks = [
    task(ROOT_A), // root
    task(SUB_C, { parentTaskId: ROOT_A }), // subtask of a
    task(ROOT_B), // root
    task(SUB_E, { parentTaskId: ROOT_B }), // subtask of b
    task("other-wf", { workflowId: OTHER_WORKFLOW }), // root in another workflow
    task(PWC), // root with a child (valid target)
    task(PWC_CHILD, { parentTaskId: PWC }),
  ];

  it("offers same-workflow roots excluding the task and its current parent", () => {
    // c (subtask of a) may nest under b and parent-with-child only.
    expect(computeNestTargets(task(SUB_C, { parentTaskId: ROOT_A }), tasks)).toEqual(
      new Set([ROOT_B, PWC]),
    );
  });

  it("offers no targets when the task has children (would exceed one level)", () => {
    expect(computeNestTargets(task(PWC), tasks)).toEqual(new Set());
  });

  it("offers no targets when the active task is unknown", () => {
    expect(computeNestTargets(undefined, tasks)).toEqual(new Set());
  });

  it("excludes subtasks and other workflows even when they are roots", () => {
    const targets = computeNestTargets(task(SUB_C, { parentTaskId: ROOT_A }), tasks);
    expect(targets.has(SUB_E)).toBe(false);
    expect(targets.has("other-wf")).toBe(false);
    expect(targets.has(SUB_C)).toBe(false);
  });

  // @covers AC-TASKS-SUBTASK-REPARENTING-DRAG-DROP-001.1
  // @covers AC-TASKS-SUBTASK-REPARENTING-DRAG-DROP-001.4
  it("matches the shared Office candidates for a subject with children", () => {
    const officeTasks = [
      task(OFFICE_SUBJECT, { isFromOffice: true }),
      task("office-descendant", {
        parentTaskId: OFFICE_SUBJECT,
        isFromOffice: true,
      }),
      task(OFFICE_TARGET_ROOT, { isFromOffice: true }),
      task("office-target-child", {
        parentTaskId: OFFICE_TARGET_ROOT,
        isFromOffice: true,
      }),
    ];

    expect(computeNestTargets(officeTasks[0], officeTasks)).toEqual(
      new Set([OFFICE_TARGET_ROOT, "office-target-child"]),
    );
  });

  it("excludes a descendant whose intermediate ancestor is hidden", () => {
    const hierarchy = [
      task(OFFICE_SUBJECT, { isFromOffice: true }),
      task(HIDDEN_CHILD, { parentTaskId: OFFICE_SUBJECT, isFromOffice: true }),
      task("visible-grandchild", { parentTaskId: HIDDEN_CHILD, isFromOffice: true }),
      task("valid-target", { isFromOffice: true }),
    ];
    const visibleTasks = hierarchy.filter((candidate) => candidate.id !== HIDDEN_CHILD);

    expect(computeNestTargets(hierarchy[0], visibleTasks, hierarchy)).toEqual(
      new Set(["valid-target"]),
    );
  });
});
