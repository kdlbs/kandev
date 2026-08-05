"use client";

import { useMemo } from "react";
import { useTranslation } from "react-i18next";
import { applyView } from "@/lib/sidebar/apply-view";
import { useEffectiveSidebarView } from "@/hooks/domains/sidebar/use-effective-sidebar-view";
import { useSidebarTaskPrefs } from "@/hooks/domains/sidebar/use-sidebar-task-prefs";
import type { TaskSwitcherItem } from "./task-switcher";

export function useGroupedSidebarView(displayTasks: TaskSwitcherItem[]) {
  const prefs = useSidebarTaskPrefs();
  const effectiveView = useEffectiveSidebarView();
  const { pinnedTaskIds, orderedTaskIds, subtaskOrderByParentId } = prefs;
  // `applyGroup`'s executorType label comes from `getExecutorLabel`, which reads
  // the catalog. Without the language in the deps the group heading keeps the
  // previous locale until task data changes.
  const { i18n } = useTranslation();
  const grouped = useMemo(
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
  return { grouped, effectiveView, prefs };
}
