"use client";

import { useMemo, useRef } from "react";
import { useTranslation } from "react-i18next";
import { getExecutorLabel } from "@/lib/executor-icons";
import { t } from "@/lib/i18n";
import { applyView, type GroupedSidebarList, type SidebarGroup } from "@/lib/sidebar/apply-view";
import { formatTaskStateLabel } from "@/lib/ui/state-labels";
import type { SidebarTaskPageEntry } from "@/lib/types/http";
import type { TaskState } from "@/lib/types/http";
import { useEffectiveSidebarView } from "@/hooks/domains/sidebar/use-effective-sidebar-view";
import { useSidebarTaskPrefs } from "@/hooks/domains/sidebar/use-sidebar-task-prefs";
import type { TaskSwitcherItem } from "./task-switcher-types";

function shallowObjectEqual(previous: object, next: object): boolean {
  const previousRecord = previous as Record<string, unknown>;
  const nextRecord = next as Record<string, unknown>;
  const keys = Object.keys(previousRecord);
  return (
    keys.length === Object.keys(nextRecord).length &&
    keys.every((key) => Object.is(previousRecord[key], nextRecord[key]))
  );
}

function taskFieldEqual(previous: unknown, next: unknown): boolean {
  if (Object.is(previous, next)) return true;
  if (previous === null || next === null) return false;
  if (Array.isArray(previous) && Array.isArray(next)) {
    return (
      previous.length === next.length &&
      previous.every((value, index) => {
        const nextValue = next[index];
        if (Object.is(value, nextValue)) return true;
        return (
          typeof value === "object" &&
          value !== null &&
          typeof nextValue === "object" &&
          nextValue !== null &&
          shallowObjectEqual(value, nextValue)
        );
      })
    );
  }
  return (
    typeof previous === "object" && typeof next === "object" && shallowObjectEqual(previous, next)
  );
}

function taskItemEqual(previous: TaskSwitcherItem, next: TaskSwitcherItem): boolean {
  const previousRecord = previous as Record<string, unknown>;
  const nextRecord = next as Record<string, unknown>;
  const keys = Object.keys(previousRecord);
  return (
    keys.length === Object.keys(nextRecord).length &&
    keys.every((key) => taskFieldEqual(previousRecord[key], nextRecord[key]))
  );
}

function previousTasksById(grouped: GroupedSidebarList): Map<string, TaskSwitcherItem> {
  const result = new Map<string, TaskSwitcherItem>();
  for (const group of grouped.groups) {
    for (const task of group.tasks) result.set(task.id, task);
  }
  for (const tasks of grouped.subTasksByParentId.values()) {
    for (const task of tasks) result.set(task.id, task);
  }
  return result;
}

function shareTaskArray(
  previous: TaskSwitcherItem[] | undefined,
  next: TaskSwitcherItem[],
  priorById: Map<string, TaskSwitcherItem>,
): TaskSwitcherItem[] {
  const shared = next.map((task) => {
    const prior = priorById.get(task.id);
    if (!prior || prior === task) return task;
    return taskItemEqual(prior, task) ? prior : task;
  });
  return previous &&
    previous.length === shared.length &&
    previous.every((task, index) => task === shared[index])
    ? previous
    : shared;
}

function shareGroup(
  previous: SidebarGroup | undefined,
  next: SidebarGroup,
  priorById: Map<string, TaskSwitcherItem>,
): SidebarGroup {
  if (
    !previous ||
    previous.label !== next.label ||
    previous.isContinuation !== next.isContinuation ||
    previous.matchingCount !== next.matchingCount
  )
    return next;
  const tasks = shareTaskArray(previous.tasks, next.tasks, priorById);
  return tasks === previous.tasks ? previous : { ...next, tasks };
}

function pageGroupLabel(group: GroupedSidebarList["groupKey"], key: string, label: string): string {
  if (key === "__unassigned__") return t("sidebar:groupUnassigned");
  if (key === "__all__") return t("sidebar:groupAll");
  if (key === "__multi__") return t("sidebar:groupMultiRepo");
  if (group === "state") {
    return key === "__not_started__"
      ? formatTaskStateLabel(undefined)
      : formatTaskStateLabel(key as TaskState);
  }
  if (group === "executorType") return getExecutorLabel(key);
  return label;
}

function shareGroups(
  previous: SidebarGroup[],
  next: SidebarGroup[],
  priorById: Map<string, TaskSwitcherItem>,
): SidebarGroup[] {
  const previousByKey = new Map(previous.map((group) => [group.key, group]));
  const shared = next.map((group) => shareGroup(previousByKey.get(group.key), group, priorById));
  return shared.length === previous.length &&
    shared.every((group, index) => group === previous[index])
    ? previous
    : shared;
}

function shareSubtaskMap(
  previous: GroupedSidebarList["subTasksByParentId"],
  next: GroupedSidebarList["subTasksByParentId"],
  priorById: Map<string, TaskSwitcherItem>,
): GroupedSidebarList["subTasksByParentId"] {
  let changed = previous.size !== next.size;
  const shared = new Map<string, TaskSwitcherItem[]>();
  for (const [parentId, tasks] of next) {
    const previousTasks = previous.get(parentId);
    const value = shareTaskArray(previousTasks, tasks, priorById);
    shared.set(parentId, value);
    if (value !== previousTasks) changed = true;
  }
  return changed ? shared : previous;
}

export function shareGroupedSidebarList(
  previous: GroupedSidebarList | null,
  next: GroupedSidebarList,
): GroupedSidebarList {
  // Desktop and mobile build equivalent task models independently. Reusing
  // equal models here gives both surfaces the same memoization boundary.
  if (!previous || previous.groupKey !== next.groupKey) return next;
  const priorById = previousTasksById(previous);
  const groups = shareGroups(previous.groups, next.groups, priorById);
  const subTasksByParentId = shareSubtaskMap(
    previous.subTasksByParentId,
    next.subTasksByParentId,
    priorById,
  );
  return groups === previous.groups && subTasksByParentId === previous.subTasksByParentId
    ? previous
    : { ...next, groups, subTasksByParentId };
}

export function useSharedGroupedSidebarList(next: GroupedSidebarList): GroupedSidebarList {
  const previousRef = useRef<GroupedSidebarList | null>(null);
  const shared = shareGroupedSidebarList(previousRef.current, next);
  previousRef.current = shared;
  return shared;
}

function ensurePageGroup(params: {
  groups: SidebarGroup[];
  groupByKey: Map<string, SidebarGroup>;
  groupKey: GroupedSidebarList["groupKey"];
  key: string;
  label: string;
  matchingCount?: number;
  isContinuation?: boolean;
}): SidebarGroup {
  const { groups, groupByKey, groupKey, key, label, matchingCount, isContinuation } = params;
  const existing = groupByKey.get(key);
  if (existing) return existing;
  const group: SidebarGroup = {
    key,
    label: pageGroupLabel(groupKey, key, label),
    tasks: [],
    matchingCount,
    isContinuation,
  };
  groupByKey.set(key, group);
  groups.push(group);
  return group;
}

function appendPageTask(
  entry: SidebarTaskPageEntry,
  task: TaskSwitcherItem,
  group: SidebarGroup,
  taskById: Map<string, TaskSwitcherItem>,
  subTasksByParentId: Map<string, TaskSwitcherItem[]>,
): void {
  const parent = entry.parent_id ? taskById.get(entry.parent_id) : undefined;
  if ((entry.depth ?? 0) > 0 && parent && entry.parent_id) {
    const children = subTasksByParentId.get(entry.parent_id) ?? [];
    children.push(task);
    subTasksByParentId.set(entry.parent_id, children);
    return;
  }
  if (entry.parent_id) {
    task.parentTaskId = entry.parent_id;
    task.continuationParentTitle = entry.parent_title ?? task.parentTaskTitle;
  }
  group.tasks.push(task);
}

/** Rebuild the display tree from the server's already-filtered and ordered page. */
export function groupSidebarTaskPage(
  tasks: TaskSwitcherItem[],
  entries: SidebarTaskPageEntry[],
  groupKey: GroupedSidebarList["groupKey"],
): GroupedSidebarList {
  const taskById = new Map(tasks.map((task) => [task.id, task]));
  const groups: SidebarGroup[] = [];
  const groupByKey = new Map<string, SidebarGroup>();
  const subTasksByParentId = new Map<string, TaskSwitcherItem[]>();
  for (const entry of entries) {
    if (entry.kind === "group") {
      if (entry.group_key)
        ensurePageGroup({
          groups,
          groupByKey,
          groupKey,
          key: entry.group_key,
          label: entry.group_label ?? entry.group_key,
          matchingCount: entry.matching_count,
          isContinuation: entry.continuation,
        });
      continue;
    }
    if (entry.kind !== "task" || !entry.task_id) continue;
    const task = taskById.get(entry.task_id);
    if (!task) continue;
    const groupKeyForTask = entry.group_key ?? "__all__";
    const group = ensurePageGroup({
      groups,
      groupByKey,
      groupKey,
      key: groupKeyForTask,
      label: entry.group_label ?? groupKeyForTask,
      matchingCount: entry.matching_count,
    });
    appendPageTask(entry, task, group, taskById, subTasksByParentId);
  }
  return { groups, subTasksByParentId, groupKey };
}

export function useGroupedSidebarView(displayTasks: TaskSwitcherItem[]) {
  const prefs = useSidebarTaskPrefs();
  const effectiveView = useEffectiveSidebarView();
  const { pinnedTaskIds, orderedTaskIds, subtaskOrderByParentId } = prefs;
  // `applyGroup`'s executorType label comes from `getExecutorLabel`, which reads
  // the catalog. Without the language in the deps the group heading keeps the
  // previous locale until task data changes.
  const { i18n } = useTranslation();
  const nextGrouped = useMemo(
    () =>
      applyView(displayTasks, effectiveView, {
        pinnedTaskIds,
        orderedTaskIds,
        subtaskOrderByParentId,
      }),
    [
      displayTasks,
      effectiveView,
      pinnedTaskIds,
      orderedTaskIds,
      subtaskOrderByParentId,
      i18n.language,
    ],
  );
  const grouped = useSharedGroupedSidebarList(nextGrouped);
  return { grouped, effectiveView, prefs };
}
