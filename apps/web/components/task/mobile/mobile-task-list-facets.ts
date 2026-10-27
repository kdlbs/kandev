import type { TaskListFacetValue } from "@/lib/plugins/types";
import { countGroupTasks, type GroupedSidebarList } from "@/lib/sidebar/apply-view";
import { sortItemsByFacet } from "@/lib/tasks/tasks-list-options";
import type { SidebarTaskPageEntry } from "@/lib/types/http";
import type { TaskSwitcherItem } from "../task-switcher";

export type MobileFacetValues = Record<string, readonly TaskListFacetValue[]>;

type FacetSection = {
  key: string;
  label: string;
  tasks: TaskSwitcherItem[];
};

type BuildMobileFacetGroupedPageOptions = {
  facetValues: MobileFacetValues;
  unassignedLabel: string;
  facetSortKey?: string | null;
};

export function buildMobileFacetGroupedPage(
  tasks: TaskSwitcherItem[],
  entries: SidebarTaskPageEntry[],
  facetGroupKey: string,
  { facetValues, unassignedLabel, facetSortKey = null }: BuildMobileFacetGroupedPageOptions,
): GroupedSidebarList {
  const pageTasks = pageTasksInEntryOrder(tasks, entries);
  const sortedTasks = facetSortKey
    ? sortItemsByFacet(pageTasks, facetSortKey, facetValues)
    : pageTasks;
  const sections = new Map<string, FacetSection>();
  const unassigned: TaskSwitcherItem[] = [];

  for (const task of sortedTasks) {
    const values = facetValues[`${facetGroupKey}:${task.id}`] ?? [];
    const namedValues = values.filter((value) => value.value && value.label);
    if (!namedValues.length) {
      unassigned.push(task);
      continue;
    }
    for (const value of namedValues) {
      const key = `facet:value:${value.value}`;
      const section = sections.get(key) ?? { key, label: value.label, tasks: [] };
      section.tasks.push(task);
      sections.set(key, section);
    }
  }

  const ordered = [...sections.values()].sort((left, right) =>
    left.label.localeCompare(right.label, undefined, { sensitivity: "base" }),
  );
  if (unassigned.length) {
    ordered.push({
      key: "facet:host:ungrouped",
      label: unassignedLabel,
      tasks: unassigned,
    });
  }
  const subTasksByParentId = new Map<string, TaskSwitcherItem[]>();
  const groups = ordered.map((section) => {
    const ids = new Set(section.tasks.map((task) => task.id));
    const groupSubTasksByParentId = new Map<string, TaskSwitcherItem[]>();
    for (const task of section.tasks) {
      const parentId = task.parentTaskId;
      if (!parentId || !ids.has(parentId)) continue;
      const children = groupSubTasksByParentId.get(parentId) ?? [];
      if (!children.some((child) => child.id === task.id)) children.push(task);
      groupSubTasksByParentId.set(parentId, children);
    }
    for (const [parentId, children] of groupSubTasksByParentId) {
      subTasksByParentId.set(parentId, [...(subTasksByParentId.get(parentId) ?? []), ...children]);
    }
    const roots = section.tasks
      .filter((task) => !task.parentTaskId || !ids.has(task.parentTaskId))
      .map((task) => ({
        ...task,
        subtaskCount: Math.max(0, countGroupTasks([task], groupSubTasksByParentId) - 1),
      }));
    return {
      key: section.key,
      label: section.label,
      tasks: roots,
      subTasksByParentId: groupSubTasksByParentId,
      matchingCount: section.tasks.length,
    };
  });
  return { groups, subTasksByParentId, groupKey: "none" };
}

export function sortMobileFacetPageGroups(
  grouped: GroupedSidebarList,
  facetSortKey: string,
  facetValues: MobileFacetValues,
): GroupedSidebarList {
  const allTasks = grouped.groups.flatMap((group) => [
    ...group.tasks,
    ...[...(group.subTasksByParentId?.values() ?? [])].flat(),
  ]);
  const sortedIds = sortItemsByFacet(allTasks, facetSortKey, facetValues).map((task) => task.id);
  const rank = new Map(sortedIds.map((id, index) => [id, index]));
  const sortTasks = (tasks: TaskSwitcherItem[]) =>
    [...tasks].sort((left, right) => (rank.get(left.id) ?? 0) - (rank.get(right.id) ?? 0));
  return {
    ...grouped,
    groups: grouped.groups.map((group) => ({
      ...group,
      tasks: sortTasks(group.tasks),
      subTasksByParentId: group.subTasksByParentId
        ? new Map([...group.subTasksByParentId].map(([id, children]) => [id, sortTasks(children)]))
        : undefined,
    })),
    subTasksByParentId: new Map(
      [...grouped.subTasksByParentId].map(([id, children]) => [id, sortTasks(children)]),
    ),
  };
}

function pageTasksInEntryOrder(
  tasks: TaskSwitcherItem[],
  entries: SidebarTaskPageEntry[],
): TaskSwitcherItem[] {
  const taskById = new Map(tasks.map((task) => [task.id, task]));
  const seen = new Set<string>();
  const ordered: TaskSwitcherItem[] = [];
  for (const entry of entries) {
    if (entry.kind !== "task" || !entry.task_id || seen.has(entry.task_id)) continue;
    const task = taskById.get(entry.task_id);
    if (!task) continue;
    seen.add(task.id);
    ordered.push(task);
  }
  return ordered;
}
