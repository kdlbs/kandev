import { describe, expect, it } from "vitest";
import type { TaskSwitcherItem } from "@/components/task/task-switcher";
import type { SidebarView } from "@/lib/state/slices/ui/sidebar-view-types";
import { applySort, applyView, flattenVisibleTaskIds } from "./apply-view";

const ACTIVITY = {
  early: "2026-04-01",
  second: "2026-04-02",
  third: "2026-04-03",
  fourth: "2026-04-04",
  latest: "2026-04-05",
  farFuture: "2026-04-10",
  fractional120: "2026-04-01T10:00:00.12Z",
  fractional121: "2026-04-01T10:00:00.121Z",
  fractional120Equivalent: "2026-04-01T11:00:00.120+01:00",
  fractional123: "2026-04-01T10:00:00.123Z",
  fractional125: "2026-04-01T10:00:00.125Z",
  createdParent: "2026-03-01",
  createdChild: "2026-03-02",
} as const;
const SHORT_FRACTION_TASK_ID = "short-fraction";

function task(overrides: Partial<TaskSwitcherItem>): TaskSwitcherItem {
  return {
    id: overrides.id ?? "task",
    title: overrides.title ?? "Task",
    ...overrides,
  };
}

function lastActivityView(
  direction: "asc" | "desc" = "desc",
  filters: SidebarView["filters"] = [],
): SidebarView {
  return {
    id: "last-activity",
    name: "Last activity",
    filters,
    sort: { key: "lastActivityAt", direction },
    group: "none",
    collapsedGroups: [],
  };
}

// @covers AC-UI-SIDEBAR-LAST-ACTIVITY-SORT-002.1, .2, .4, .5
describe("task tree activity root and child order", () => {
  it("ranks roots and child subtrees by their newest included descendant in either direction", () => {
    const tasks = [
      task({ id: "peer", lastActivityAt: ACTIVITY.fourth }),
      task({ id: "parent", lastActivityAt: ACTIVITY.early }),
      task({ id: "sibling", parentTaskId: "parent", lastActivityAt: ACTIVITY.third }),
      task({ id: "child", parentTaskId: "parent", lastActivityAt: ACTIVITY.second }),
      task({ id: "grandchild", parentTaskId: "child", lastActivityAt: ACTIVITY.latest }),
    ];

    const descending = applyView(tasks, lastActivityView("desc"));
    expect(descending.groups[0].tasks.map((item) => item.id)).toEqual(["parent", "peer"]);
    expect(descending.subTasksByParentId.get("parent")?.map((item) => item.id)).toEqual([
      "child",
      "sibling",
    ]);

    const ascending = applyView(tasks, lastActivityView("asc"));
    expect(ascending.groups[0].tasks.map((item) => item.id)).toEqual(["peer", "parent"]);
    expect(ascending.subTasksByParentId.get("parent")?.map((item) => item.id)).toEqual([
      "sibling",
      "child",
    ]);

    expect(tasks.find((item) => item.id === "parent")?.lastActivityAt).toBe(ACTIVITY.early);
  });

  it("ignores filtered-out descendants and promotes a child when its parent is filtered out", () => {
    const parent = task({
      id: "parent",
      title: "Visible parent",
      lastActivityAt: ACTIVITY.early,
    });
    const includedChild = task({
      id: "included-child",
      title: "Visible child",
      parentTaskId: "parent",
      lastActivityAt: ACTIVITY.latest,
    });
    const excludedChild = task({
      id: "excluded-child",
      title: "Hidden child",
      parentTaskId: "parent",
      isArchived: true,
      lastActivityAt: ACTIVITY.farFuture,
    });
    const peer = task({
      id: "peer",
      title: "Visible peer",
      lastActivityAt: ACTIVITY.fourth,
    });

    const visible = applyView(
      [parent, includedChild, excludedChild, peer],
      lastActivityView("desc", [
        { id: "active-only", dimension: "archived", op: "is", value: false },
      ]),
    );
    expect(visible.groups[0].tasks.map((item) => item.id)).toEqual(["parent", "peer"]);
    expect(visible.subTasksByParentId.get("parent")?.map((item) => item.id)).toEqual([
      "included-child",
    ]);

    const promoted = applyView(
      [parent, includedChild, peer],
      lastActivityView("desc", [
        { id: "visible-child", dimension: "titleMatch", op: "matches", value: "child" },
      ]),
    );
    expect(promoted.groups[0].tasks.map((item) => item.id)).toEqual(["included-child"]);
    expect(promoted.subTasksByParentId.has("parent")).toBe(false);
  });

  it("keeps equal tree keys stable and collapse does not change root rank", () => {
    const tasks = [
      task({ id: "parent", lastActivityAt: ACTIVITY.early }),
      task({ id: "child", parentTaskId: "parent", lastActivityAt: ACTIVITY.latest }),
      task({ id: "peer", lastActivityAt: ACTIVITY.latest }),
    ];
    const grouped = applyView(tasks, lastActivityView());

    expect(grouped.groups[0].tasks.map((item) => item.id)).toEqual(["parent", "peer"]);
    expect(flattenVisibleTaskIds(grouped, [], ["parent"])).toEqual(["parent", "peer"]);
    expect(grouped.groups[0].tasks.map((item) => item.id)).toEqual(["parent", "peer"]);
  });
});

describe("task activity fallback timestamps", () => {
  it("uses task update and creation timestamps when Last activity is unavailable", () => {
    const tasks = [
      task({
        id: "parent",
        updatedAt: ACTIVITY.early,
        createdAt: ACTIVITY.createdParent,
      }),
      task({
        id: "child",
        parentTaskId: "parent",
        updatedAt: ACTIVITY.latest,
        createdAt: ACTIVITY.createdChild,
      }),
      task({ id: "peer", lastActivityAt: ACTIVITY.fourth }),
      task({ id: "created-only", createdAt: ACTIVITY.third }),
    ];

    const grouped = applyView(tasks, lastActivityView());
    expect(grouped.groups[0].tasks.map((item) => item.id)).toEqual([
      "parent",
      "peer",
      "created-only",
    ]);
  });
});

describe("fractional activity timestamp precision", () => {
  it("orders two root tasks by their chronological instant in both directions", () => {
    const tasks = [
      task({ id: SHORT_FRACTION_TASK_ID, lastActivityAt: ACTIVITY.fractional120 }),
      task({ id: "long-fraction", lastActivityAt: ACTIVITY.fractional123 }),
    ];

    expect(
      applyView(tasks, lastActivityView("desc")).groups[0].tasks.map((item) => item.id),
    ).toEqual(["long-fraction", SHORT_FRACTION_TASK_ID]);
    expect(
      applyView(tasks, lastActivityView("asc")).groups[0].tasks.map((item) => item.id),
    ).toEqual([SHORT_FRACTION_TASK_ID, "long-fraction"]);
  });

  it("aggregates a child timestamp precisely when ranking the parent tree in both directions", () => {
    const tasks = [
      task({ id: "parent", lastActivityAt: ACTIVITY.fractional120 }),
      task({
        id: "child",
        parentTaskId: "parent",
        lastActivityAt: ACTIVITY.fractional123,
      }),
      task({ id: "peer", lastActivityAt: ACTIVITY.fractional121 }),
    ];

    expect(
      applyView(tasks, lastActivityView("desc")).groups[0].tasks.map((item) => item.id),
    ).toEqual(["parent", "peer"]);
    expect(
      applyView(tasks, lastActivityView("asc")).groups[0].tasks.map((item) => item.id),
    ).toEqual(["peer", "parent"]);
  });

  it("keeps equivalent fractional timestamps stable and missing timestamps in their prior position", () => {
    const tiedTasks = [
      task({ id: SHORT_FRACTION_TASK_ID, lastActivityAt: ACTIVITY.fractional120 }),
      task({ id: "padded-fraction", lastActivityAt: ACTIVITY.fractional120Equivalent }),
    ];
    const missingTasks = [
      task({ id: "first-missing" }),
      task({ id: "dated", lastActivityAt: ACTIVITY.fractional120 }),
      task({ id: "second-missing" }),
    ];

    for (const direction of ["asc", "desc"] as const) {
      expect(
        applyView(tiedTasks, lastActivityView(direction)).groups[0].tasks.map((item) => item.id),
      ).toEqual([SHORT_FRACTION_TASK_ID, "padded-fraction"]);
    }
    expect(
      applyView(missingTasks, lastActivityView("desc")).groups[0].tasks.map((item) => item.id),
    ).toEqual(["dated", "first-missing", "second-missing"]);
    expect(
      applyView(missingTasks, lastActivityView("asc")).groups[0].tasks.map((item) => item.id),
    ).toEqual(["first-missing", "second-missing", "dated"]);
  });
});

describe("large and malformed task trees", () => {
  it("resolves activity on a deep descendant without recursive traversal", () => {
    const tasks: TaskSwitcherItem[] = [task({ id: "root", lastActivityAt: ACTIVITY.early })];
    let parentTaskId = "root";
    for (let index = 0; index < 6_000; index += 1) {
      const id = "child-" + index;
      tasks.push(
        task({
          id,
          parentTaskId,
          lastActivityAt: index === 5_999 ? ACTIVITY.farFuture : ACTIVITY.second,
        }),
      );
      parentTaskId = id;
    }
    tasks.push(task({ id: "peer", lastActivityAt: ACTIVITY.latest }));

    const grouped = applyView(tasks, lastActivityView());
    expect(grouped.groups[0].tasks.map((item) => item.id)).toEqual(["root", "peer"]);
  });

  it("terminates and returns a key for every task in a malformed parent cycle", () => {
    const a = task({ id: "a", lastActivityAt: ACTIVITY.early });
    const b = task({ id: "b", lastActivityAt: ACTIVITY.second });
    const children = new Map<string, TaskSwitcherItem[]>([
      ["a", [b]],
      ["b", [a]],
    ]);

    const sorted = applySort([a, b], { key: "lastActivityAt", direction: "desc" }, [], children);
    expect(sorted.map((item) => item.id)).toEqual(["a", "b"]);
  });
});
