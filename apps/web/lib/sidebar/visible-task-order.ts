import type { MultiSelectOccurrence } from "@/hooks/task-multi-select-state";
import type { TaskSwitcherItem } from "@/components/task/task-switcher-types";
import type { GroupedSidebarList } from "./apply-view";

export type SidebarTaskOrder = {
  occurrences: MultiSelectOccurrence[];
  taskIds: string[];
};

export function sidebarTaskOccurrenceKey(groupKey: string, taskId: string): string {
  return JSON.stringify([groupKey, taskId]);
}

export function flattenVisibleSidebarTasks(
  grouped: GroupedSidebarList,
  collapsedGroupKeys: string[],
  collapsedSubtaskParentIds: string[],
): SidebarTaskOrder {
  const collapsedGroups = new Set(collapsedGroupKeys);
  const collapsedSubs = new Set(collapsedSubtaskParentIds);
  const occurrences: MultiSelectOccurrence[] = [];
  const taskIds: string[] = [];
  const canonical = new Set<string>();
  for (const group of grouped.groups) {
    if (collapsedGroups.has(group.key) || group.tasks.length === 0) continue;
    const children = group.subTasksByParentId ?? grouped.subTasksByParentId;
    const visited = new Set<string>();
    const visit = (task: TaskSwitcherItem) => {
      if (visited.has(task.id)) return;
      visited.add(task.id);
      occurrences.push({ key: sidebarTaskOccurrenceKey(group.key, task.id), taskId: task.id });
      if (!canonical.has(task.id)) {
        canonical.add(task.id);
        taskIds.push(task.id);
      }
      if (collapsedSubs.has(task.id)) return;
      const subs = children.get(task.id);
      if (subs) for (const child of subs) visit(child);
    };
    for (const task of group.tasks) visit(task);
  }
  return { occurrences, taskIds };
}
