import { describe, expect, it } from "vitest";
import type { TaskSwitcherItem } from "../task-switcher-types";
import { buildMobileFacetGroupedPage, sortMobileFacetPageGroups } from "./mobile-task-list-facets";

function task(id: string, parentTaskId?: string, subtaskCount?: number): TaskSwitcherItem {
  return { id, title: id, parentTaskId, subtaskCount };
}

describe("mobile plugin facet grouping", () => {
  const tasks = [task("multi"), task("single"), task("empty")];
  const values = {
    "facet:plugin:tags:multi": [
      { value: "alpha", label: "Alpha" },
      { value: "beta", label: "Beta" },
    ],
    "facet:plugin:tags:single": [{ value: "beta", label: "Beta" }],
  };

  it("groups multi-value tasks in every section and keeps unassigned rows", () => {
    const grouped = buildMobileFacetGroupedPage(
      tasks,
      tasks.map(({ id }) => ({ kind: "task" as const, task_id: id })),
      "facet:plugin:tags",
      { facetValues: values, unassignedLabel: "Unassigned" },
    );

    expect(
      grouped.groups.map((group) => [group.label, group.tasks.map((item) => item.id)]),
    ).toEqual([
      ["Alpha", ["multi"]],
      ["Beta", ["multi", "single"]],
      ["Unassigned", ["empty"]],
    ]);
  });

  it("sorts only the supplied page and retains the unassigned order last", () => {
    const pageOrder = [tasks[1]!, tasks[0]!, tasks[2]!];
    const grouped = {
      groupKey: "none" as const,
      groups: [{ key: "all", label: "", tasks: pageOrder, matchingCount: pageOrder.length }],
      subTasksByParentId: new Map<string, TaskSwitcherItem[]>(),
    };
    const sorted = sortMobileFacetPageGroups(grouped, "facet:plugin:tags", values);

    expect(sorted.groups[0]?.tasks.map((item) => item.id)).toEqual(["multi", "single", "empty"]);
  });

  it("keeps each task's child membership inside its own facet group", () => {
    const parent = task("parent", undefined, 1);
    const child = task("child", "parent");
    const grouped = buildMobileFacetGroupedPage(
      [parent, child],
      [
        { kind: "task", task_id: "parent", depth: 0 },
        { kind: "task", task_id: "child", parent_id: "parent", depth: 1 },
      ],
      "tags",
      {
        facetValues: {
          "tags:parent": [
            { value: "alpha", label: "Alpha" },
            { value: "beta", label: "Beta" },
          ],
          "tags:child": [{ value: "alpha", label: "Alpha" }],
        },
        unassignedLabel: "Unassigned",
      },
    );

    const alpha = grouped.groups.find((group) => group.label === "Alpha");
    const beta = grouped.groups.find((group) => group.label === "Beta");
    expect(alpha?.tasks.map((item) => item.id)).toEqual(["parent"]);
    expect(alpha?.subTasksByParentId?.get("parent")?.map((item) => item.id)).toEqual(["child"]);
    expect(beta?.tasks.map((item) => item.id)).toEqual(["parent"]);
    expect(beta?.subTasksByParentId?.get("parent")).toBeUndefined();
    expect(beta?.tasks[0]?.subtaskCount).toBe(0);
  });
});
