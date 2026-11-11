import { describe, expect, it } from "vitest";
import type { TaskSwitcherItem } from "@/components/task/task-switcher-types";
import { DEFAULT_VIEW } from "@/lib/state/slices/ui/sidebar-view-builtins";
import {
  applyView,
  flattenVisibleTaskIds,
  sidebarSubtaskCollapseKey,
  sortIdsByVisibleOrder,
} from "./apply-view";
import { groupSidebarTasksByFacet, sidebarPageTasks } from "./sidebar-facet-groups";

const FACET = "facet:com.example.tags:tags";
const task = (id: string, parentTaskId?: string): TaskSwitcherItem => ({
  id,
  title: id,
  parentTaskId,
  state: "TODO",
});
const alpha = { value: "alpha", label: "Alpha", color: "#123456" };
const beta = { value: "beta", label: "beta" };

describe("sidebar facet groups", () => {
  it("orders deduplicated bulk tasks by their first visible occurrence", () => {
    expect(sortIdsByVisibleOrder(["b", "a", "x"], ["a", "x", "b", "a"])).toEqual(["a", "x", "b"]);
  });

  it("repeats multi-value tasks, deduplicates membership and puts only nonempty Unassigned last", () => {
    const result = groupSidebarTasksByFacet([task("a"), task("b"), task("c")], FACET, {
      [`${FACET}:a`]: [beta, alpha, alpha],
      [`${FACET}:b`]: [alpha],
      [`${FACET}:c`]: [{ value: "ignored", label: "" }],
    });
    expect(result.groups.map((group) => group.label)).toEqual(["Alpha", "beta", "Unassigned"]);
    expect(result.groups.map((group) => group.color)).toEqual(["#123456", undefined, undefined]);
    expect(result.groups.map((group) => group.tasks.map((row) => row.id))).toEqual([
      ["a", "b"],
      ["a"],
      ["c"],
    ]);
    expect(new Set(result.groups.map((group) => group.key)).size).toBe(3);
    expect(groupSidebarTasksByFacet([], FACET, {}).groups).toEqual([]);
  });

  it("builds each section's tree only from matching tasks and isolates parent collapse", () => {
    const result = groupSidebarTasksByFacet(
      [
        task("parent"),
        task("shared", "parent"),
        task("alpha-child", "parent"),
        task("beta-root", "missing"),
      ],
      FACET,
      {
        [`${FACET}:parent`]: [alpha, beta],
        [`${FACET}:shared`]: [alpha, beta],
        [`${FACET}:alpha-child`]: [alpha],
        [`${FACET}:beta-root`]: [beta],
      },
    );
    const [a, b] = result.groups;
    expect(a.subTasksByParentId?.get("parent")?.map((row) => row.id)).toEqual([
      "shared",
      "alpha-child",
    ]);
    expect(b.subTasksByParentId?.get("parent")?.map((row) => row.id)).toEqual(["shared"]);
    expect(b.tasks.map((row) => row.id)).toEqual(["parent", "beta-root"]);
    const collapse = sidebarSubtaskCollapseKey(a, "parent");
    expect(collapse).not.toBe(sidebarSubtaskCollapseKey(b, "parent"));
    expect(flattenVisibleTaskIds(result, [], [collapse])).toEqual([
      "parent",
      "parent",
      "shared",
      "beta-root",
    ]);
    expect(flattenVisibleTaskIds(result, [a.key], [])).toEqual(["parent", "shared", "beta-root"]);
  });

  it("promotes a child when its parent belongs only to another section", () => {
    const result = groupSidebarTasksByFacet([task("parent"), task("child", "parent")], FACET, {
      [`${FACET}:parent`]: [alpha],
      [`${FACET}:child`]: [beta],
    });
    expect(result.groups.map((group) => group.tasks.map((row) => row.id))).toEqual([
      ["parent"],
      ["child"],
    ]);
    expect(result.groups[1].subTasksByParentId?.size).toBe(0);
  });
});

describe("sidebar facet source order and identities", () => {
  it("keeps filters, source sorting, pinning and saved sibling order before grouping", () => {
    const page = applyView(
      [task("b"), task("a"), task("child-b", "a"), task("child-a", "a"), task("excluded")],
      {
        ...DEFAULT_VIEW,
        filters: [{ id: "filter", dimension: "titleMatch", op: "not_matches", value: "excluded" }],
        sort: { key: "title", direction: "asc" },
        group: FACET,
      },
      {
        pinnedTaskIds: ["b"],
        orderedTaskIds: [],
        subtaskOrderByParentId: { a: ["child-b", "child-a"] },
      },
    );
    const tasks = sidebarPageTasks(page);
    expect(tasks.map((row) => row.id)).toEqual(["b", "a", "child-b", "child-a"]);
    const values = Object.fromEntries(tasks.map((row) => [`${FACET}:${row.id}`, [alpha]]));
    const grouped = groupSidebarTasksByFacet(tasks, FACET, values);
    expect(flattenVisibleTaskIds(grouped, [], [])).toEqual(["b", "a", "child-b", "child-a"]);
    expect(grouped.groups[0].subTasksByParentId?.get("a")?.map((row) => row.id)).toEqual([
      "child-b",
      "child-a",
    ]);
  });

  it("separates equal labels, reserved-looking values and different facet identities", () => {
    const values = [
      { value: "", label: "Ignored" },
      { value: "__unassigned__", label: "Unassigned" },
      { value: "other", label: "Unassigned" },
    ];
    const first = groupSidebarTasksByFacet([task("a"), task("b")], FACET, {
      [`${FACET}:a`]: values,
    });
    expect(new Set(first.groups.map((group) => group.key)).size).toBe(3);
    const other = groupSidebarTasksByFacet([task("a")], "facet:other:tags", {
      "facet:other:tags:a": values,
    });
    expect(other.groups[0].key).not.toBe(first.groups[0].key);
  });
});
