import { describe, expect, it } from "vitest";
import type { TaskSwitcherItem } from "@/components/task/task-switcher";
import { DEFAULT_VIEW } from "@/lib/state/slices/ui/sidebar-view-builtins";
import { applyView, countGroupTasks } from "./apply-view";

function task(overrides: Partial<TaskSwitcherItem>): TaskSwitcherItem {
  return {
    id: overrides.id ?? "task",
    title: overrides.title ?? "Task",
    state: "IN_PROGRESS",
    ...overrides,
  };
}

describe("applyView direct subtasks", () => {
  it("keeps all 17 direct subtasks in the default all-tasks view", () => {
    const parent = task({ id: "parent", title: "Parent" });
    const children = Array.from({ length: 17 }, (_, index) =>
      task({
        id: `child-${index + 1}`,
        title: `Child ${index + 1}`,
        parentTaskId: parent.id,
      }),
    );

    const grouped = applyView([parent, ...children], DEFAULT_VIEW);
    const rootIds = grouped.groups.flatMap((group) => group.tasks.map((item) => item.id));
    const childIds = grouped.subTasksByParentId.get(parent.id)?.map((item) => item.id);

    expect(rootIds).toEqual([parent.id]);
    expect(childIds).toHaveLength(17);
    expect(countGroupTasks(grouped.groups[0].tasks, grouped.subTasksByParentId)).toBe(18);
  });
});
