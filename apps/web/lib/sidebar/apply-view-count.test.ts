import { describe, it, expect } from "vitest";
import type { TaskSwitcherItem } from "@/components/task/task-switcher";
import { countGroupTasks } from "./apply-view";

function task(id: string, parentTaskId?: string): TaskSwitcherItem {
  return { id, title: id, parentTaskId };
}

describe("countGroupTasks — recursive subtree count", () => {
  it("counts a root with no children as 1", () => {
    expect(countGroupTasks([task("r1")], new Map())).toBe(1);
  });

  it("counts root + direct children (depth 1)", () => {
    const subMap = new Map([["r1", [task("c1", "r1"), task("c2", "r1")]]]);
    expect(countGroupTasks([task("r1")], subMap)).toBe(3);
  });

  it("counts nested grandchildren (depth 2+)", () => {
    const subMap = new Map([
      ["r1", [task("c1", "r1")]],
      ["c1", [task("g1", "c1"), task("g2", "c1")]],
      ["g1", [task("gg1", "g1")]],
    ]);
    // r1 + c1 + g1 + g2 + gg1 = 5
    expect(countGroupTasks([task("r1")], subMap)).toBe(5);
  });

  it("sums across multiple roots", () => {
    const subMap = new Map([
      ["r1", [task("c1", "r1")]],
      ["c1", [task("g1", "c1")]],
    ]);
    // (r1 + c1 + g1) + (r2) = 4
    expect(countGroupTasks([task("r1"), task("r2")], subMap)).toBe(4);
  });

  it("terminates on cyclic parent links instead of looping forever", () => {
    const subMap = new Map([
      ["a", [task("b", "a")]],
      ["b", [task("a", "b")]],
    ]);
    expect(countGroupTasks([task("a")], subMap)).toBe(2);
  });
});
