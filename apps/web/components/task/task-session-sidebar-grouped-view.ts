"use client";

import { useMemo } from "react";
import { applyView } from "@/lib/sidebar/apply-view";
import { useEffectiveSidebarView } from "@/hooks/domains/sidebar/use-effective-sidebar-view";
import { useSidebarTaskPrefs } from "@/hooks/domains/sidebar/use-sidebar-task-prefs";
import type { TaskSwitcherItem } from "./task-switcher";

export function useGroupedSidebarView(displayTasks: TaskSwitcherItem[]) {
  const prefs = useSidebarTaskPrefs();
  const effectiveView = useEffectiveSidebarView();
  const { pinnedTaskIds, orderedTaskIds, subtaskOrderByParentId } = prefs;
  const grouped = useMemo(
    () =>
      applyView(displayTasks, effectiveView, {
        pinnedTaskIds,
        orderedTaskIds,
        subtaskOrderByParentId,
      }),
    [displayTasks, effectiveView, pinnedTaskIds, orderedTaskIds, subtaskOrderByParentId],
  );
  return { grouped, effectiveView, prefs };
}
