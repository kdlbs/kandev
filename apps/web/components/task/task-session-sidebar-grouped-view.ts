"use client";

import { useMemo, useRef } from "react";
import { useTranslation } from "react-i18next";
import { applyView, type GroupedSidebarList, type SidebarGroup } from "@/lib/sidebar/apply-view";
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
    return prior && taskItemEqual(prior, task) ? prior : task;
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
  if (!previous || previous.label !== next.label) return next;
  const tasks = shareTaskArray(previous.tasks, next.tasks, priorById);
  return tasks === previous.tasks ? previous : { ...next, tasks };
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
