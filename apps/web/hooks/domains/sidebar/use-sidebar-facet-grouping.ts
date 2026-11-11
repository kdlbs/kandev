import { useMemo } from "react";
import { useTranslation } from "react-i18next";
import { useTaskListFacets } from "@/hooks/use-task-list-facets";
import type { GroupedSidebarList } from "@/lib/sidebar/apply-view";
import { groupSidebarTasksByFacet, sidebarPageTasks } from "@/lib/sidebar/sidebar-facet-groups";
import type { GroupKey } from "@/lib/state/slices/ui/sidebar-view-types";
import { isTaskListFacetOption } from "@/lib/tasks/tasks-list-options";
import type { TaskSwitcherItem } from "@/components/task/task-switcher-types";

const NO_TASKS: TaskSwitcherItem[] = [];

export function useSidebarFacetGrouping(
  page: GroupedSidebarList,
  group: GroupKey,
  workspaceId: string | null,
): GroupedSidebarList {
  const tasks = useMemo(
    () => (isTaskListFacetOption(group) ? sidebarPageTasks(page) : NO_TASKS),
    [page, group],
  );
  const { facets, values } = useTaskListFacets(tasks, workspaceId);
  const { i18n } = useTranslation();
  return useMemo(
    () =>
      workspaceId && isTaskListFacetOption(group) && facets.some((facet) => facet.key === group)
        ? groupSidebarTasksByFacet(tasks, group, values)
        : page,
    [page, tasks, group, workspaceId, facets, values, i18n.language],
  );
}
