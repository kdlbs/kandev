import { describe, it, expect } from "vitest";
import type { TaskSwitcherItem } from "@/components/task/task-switcher";
import {
  flattenVisibleTaskIds,
  sortIdsByVisibleOrder,
  type GroupedSidebarList,
} from "./apply-view";

function task(id: string): TaskSwitcherItem {
  return { id, title: id };
}

function grouped(
  groups: GroupedSidebarList["groups"],
  subs: Record<string, TaskSwitcherItem[]> = {},
): GroupedSidebarList {
  return { groups, subTasksByParentId: new Map(Object.entries(subs)) };
}

describe("flattenVisibleTaskIds", () => {
  it("walks groups then tasks in render order", () => {
    const g = grouped([
      { key: "g1", label: "G1", tasks: [task("a"), task("b")] },
      { key: "g2", label: "G2", tasks: [task("c")] },
    ]);
    expect(flattenVisibleTaskIds(g, [], [])).toEqual(["a", "b", "c"]);
  });

  it("expands subtasks depth-first under their parent", () => {
    const g = grouped([{ key: "g1", label: "G1", tasks: [task("a"), task("d")] }], {
      a: [task("a1"), task("a2")],
    });
    expect(flattenVisibleTaskIds(g, [], [])).toEqual(["a", "a1", "a2", "d"]);
  });

  it("skips tasks in collapsed groups", () => {
    const g = grouped([
      { key: "g1", label: "G1", tasks: [task("a")] },
      { key: "g2", label: "G2", tasks: [task("b")] },
    ]);
    expect(flattenVisibleTaskIds(g, ["g2"], [])).toEqual(["a"]);
  });

  it("hides subtasks of a collapsed parent but keeps the parent", () => {
    const g = grouped([{ key: "g1", label: "G1", tasks: [task("a")] }], { a: [task("a1")] });
    expect(flattenVisibleTaskIds(g, [], ["a"])).toEqual(["a"]);
  });
});

describe("sortIdsByVisibleOrder", () => {
  const visible = ["a", "b", "c", "d"];

  it("reorders a backward range selection into visible order", () => {
    // Anchor 'd' then shift to 'b' leaves the Set as [d, b, c] (insertion order).
    expect(sortIdsByVisibleOrder(["d", "b", "c"], visible)).toEqual(["b", "c", "d"]);
  });

  it("leaves an already-ordered list unchanged", () => {
    expect(sortIdsByVisibleOrder(["a", "c"], visible)).toEqual(["a", "c"]);
  });

  it("sorts ids missing from the visible list to the front", () => {
    expect(sortIdsByVisibleOrder(["c", "zzz", "a"], visible)).toEqual(["zzz", "a", "c"]);
  });
});
