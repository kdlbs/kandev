import type { TaskSwitcherItem } from "@/components/task/task-switcher-types";
import type { TaskListFacetValue } from "@/lib/plugins/types";
import type { GroupKey } from "@/lib/state/slices/ui/sidebar-view-types";
import { t } from "@/lib/i18n";
import { applyGroup, type GroupedSidebarList, type SidebarGroup } from "./apply-view";

/** Preserve the accepted page's filter, sibling order, pinning and continuation projection. */
export function sidebarPageTasks(grouped: GroupedSidebarList): TaskSwitcherItem[] {
  const tasks: TaskSwitcherItem[] = [];
  const visited = new Set<string>();
  const visit = (task: TaskSwitcherItem) => {
    if (visited.has(task.id)) return;
    visited.add(task.id);
    tasks.push(task);
    for (const child of grouped.subTasksByParentId.get(task.id) ?? []) visit(child);
  };
  for (const group of grouped.groups) for (const task of group.tasks) visit(task);
  return tasks;
}

export function groupSidebarTasksByFacet(
  tasks: TaskSwitcherItem[],
  facetKey: GroupKey,
  values: Record<string, readonly TaskListFacetValue[]>,
): GroupedSidebarList {
  const buckets = new Map<string, { label: string; color?: string; tasks: TaskSwitcherItem[] }>();
  const unassigned: TaskSwitcherItem[] = [];
  for (const task of tasks) {
    const seen = new Set<string>();
    for (const value of values[`${facetKey}:${task.id}`] ?? []) {
      if (!value.value || !value.label || seen.has(value.value)) continue;
      seen.add(value.value);
      const bucket = buckets.get(value.value) ?? {
        label: value.label,
        color: value.color,
        tasks: [],
      };
      bucket.tasks.push(task);
      buckets.set(value.value, bucket);
    }
    if (!seen.size) unassigned.push(task);
  }
  const groups = [...buckets].map(([value, bucket]) =>
    facetGroup(JSON.stringify([facetKey, value]), bucket.label, bucket.tasks, bucket.color),
  );
  groups.sort(
    (a, b) =>
      a.label.localeCompare(b.label, undefined, { sensitivity: "base" }) ||
      a.key.localeCompare(b.key),
  );
  if (unassigned.length) {
    groups.push(facetGroup(JSON.stringify([facetKey]), t("sidebar:groupUnassigned"), unassigned));
  }
  return { groups, subTasksByParentId: new Map(), groupKey: facetKey };
}

function facetGroup(
  key: string,
  label: string,
  tasks: TaskSwitcherItem[],
  color?: string,
): SidebarGroup {
  const tree = applyGroup(tasks, "none");
  return {
    key,
    label,
    ...(color ? { color } : {}),
    tasks: tree.groups[0].tasks,
    subTasksByParentId: tree.subTasksByParentId,
  };
}
