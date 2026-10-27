"use client";
import { useCallback, useMemo, useState, type MutableRefObject, type ReactNode } from "react";
import { useTranslation } from "react-i18next";
import { TaskSwitcher, type TaskSwitcherItem } from "../task-switcher";
import type { TaskMoveWorkflow } from "../task-move-context-menu";
import type { StepDef } from "../task-switcher-context-menu";
import { applyView } from "@/lib/sidebar/apply-view";
import { useAppStore } from "@/components/state-provider";
import { useEffectiveSidebarView } from "@/hooks/domains/sidebar/use-effective-sidebar-view";
import { useSidebarTaskPrefs } from "@/hooks/domains/sidebar/use-sidebar-task-prefs";
import { buildMobileTaskSwitcherProps } from "./session-task-switcher-sheet-props";
import { SidebarTaskQueryStatus } from "../sidebar-task-query-status";
import { SidebarTaskPagination } from "../sidebar-task-pagination";
import { groupSidebarTaskPage } from "../task-session-sidebar-grouped-view";
import { sidebarSortHasKey } from "@/lib/sidebar/sidebar-sort-chain";
import type { SidebarTaskPageEntry, SidebarTaskPageResponse } from "@/lib/types/http";
import type { SidebarView } from "@/lib/state/slices/ui/sidebar-view-types";
import {
  buildMobileFacetGroupedPage,
  sortMobileFacetPageGroups,
  type MobileFacetValues,
} from "./mobile-task-list-facets";
function useSidebarGroupToggle(viewId: string) {
  const toggleSidebarGroupCollapsed = useAppStore((s) => s.toggleSidebarGroupCollapsed);
  return useCallback(
    (groupKey: string) => toggleSidebarGroupCollapsed(viewId, groupKey),
    [toggleSidebarGroupCollapsed, viewId],
  );
}

export type MobileTaskListProps = {
  workspaceId?: string | null;
  view?: ReturnType<typeof useEffectiveSidebarView>;
  tasks: TaskSwitcherItem[];
  workflows: TaskMoveWorkflow[];
  stepsByWorkflowId: Record<string, StepDef[]>;
  activeTaskId: string | null;
  selectedTaskId: string | null;
  onSelectTask: (taskId: string) => void;
  onMoveToStep?: (taskId: string, workflowId: string, targetStepId: string) => void;
  onRequestMoveOptions?: (taskId: string, workflowId: string, targetStepId: string) => void;
  onBeforeMoveOptionsOpen?: () => void;
  onEditTask?: (task: TaskSwitcherItem) => void;
  onRenameTask?: (taskId: string, currentTitle: string) => void;
  onCreateSubtask?: (taskId: string, taskTitle: string) => void;
  onArchiveTask: (taskId: string, opts?: { cascade?: boolean }) => void;
  onDeleteTask: (taskId: string) => Promise<void> | void;
  onDetachTask: (taskId: string) => Promise<void> | void;
  archivingTaskId?: string | null;
  isArchiving?: boolean;
  onNestTask?: (taskId: string, parentTaskId: string) => void;
  onLinkPullRequest?: (taskId: string, taskTitle?: string) => void;
  onLinkIssue?: (taskId: string, taskTitle?: string) => void;
  onLinkMergeRequest?: (taskId: string, taskTitle?: string) => void;
  onLinkJiraTicket?: (taskId: string, taskTitle?: string) => void;
  onLinkLinearIssue?: (taskId: string, taskTitle?: string) => void;
  onLinkSentryIssue?: (taskId: string, taskTitle?: string) => void;
  deletingTaskId: string | null;
  isLoading?: boolean;
  loadError?: string | null;
  onRetryLoad?: () => void;
  retryLabel?: string;
  pageEntries?: SidebarTaskPageEntry[];
  page?: SidebarTaskPageResponse | null;
  pagePending?: boolean;
  pageTransitioning?: boolean;
  pageError?: string | null;
  pageCanRetry?: boolean;
  emptyState?: ReactNode;
  facetSortKey?: string | null;
  facetGroupKey?: string | null;
  facetValues?: MobileFacetValues;
  resultSort?: SidebarView["sort"];
  resultGroup?: SidebarView["group"];
  hasLinkSortOverride?: boolean;
  hasLinkGroupOverride?: boolean;
  onPageChange?: (page: number, afterSuccess: () => void) => void;
  onPageRetry?: () => void;
  scrollContainerRef?: MutableRefObject<HTMLDivElement | null>;
};

type MobileTaskListGroupState = {
  facetCollapsedGroupKeys: string[];
  temporaryGroupKey: string | null;
  temporaryGroupOverride: boolean;
  toggleGroup: (groupKey: string) => void;
};

function useMobileTaskListGroupState(
  props: MobileTaskListProps,
  view: ReturnType<typeof useEffectiveSidebarView>,
  resultGroup: SidebarView["group"],
): MobileTaskListGroupState {
  const temporaryGroupOverride = props.hasLinkGroupOverride === true;
  let temporaryGroupKey: string | null = null;
  if (props.facetGroupKey) temporaryGroupKey = `facet:${props.facetGroupKey}`;
  else if (temporaryGroupOverride) temporaryGroupKey = `link:${resultGroup}`;
  const facetGroupIdentity = `${props.workspaceId ?? ""}:${view.id}:${temporaryGroupKey ?? ""}`;
  const [facetCollapseState, setFacetCollapseState] = useState({
    identity: facetGroupIdentity,
    keys: [] as string[],
  });
  const facetCollapsedGroupKeys =
    facetCollapseState.identity === facetGroupIdentity ? facetCollapseState.keys : [];
  const toggleSavedGroup = useSidebarGroupToggle(view.id);
  const toggleGroup = useCallback(
    (groupKey: string) => {
      if (!temporaryGroupKey) {
        toggleSavedGroup(groupKey);
        return;
      }
      setFacetCollapseState((current) => {
        const keys = current.identity === facetGroupIdentity ? current.keys : [];
        return {
          identity: facetGroupIdentity,
          keys: keys.includes(groupKey)
            ? keys.filter((key) => key !== groupKey)
            : [...keys, groupKey],
        };
      });
    },
    [facetGroupIdentity, temporaryGroupKey, toggleSavedGroup],
  );
  return { facetCollapsedGroupKeys, temporaryGroupKey, temporaryGroupOverride, toggleGroup };
}

function useMobileTaskListGroupedPage(
  props: MobileTaskListProps,
  view: ReturnType<typeof useEffectiveSidebarView>,
  resultSort: SidebarView["sort"],
  resultGroup: SidebarView["group"],
  prefs: ReturnType<typeof useSidebarTaskPrefs>,
): ReturnType<typeof groupSidebarTaskPage> {
  // See useGroupedSidebarView: the executorType group label is catalog-backed.
  const { i18n, t } = useTranslation();
  return useMemo(() => {
    const facetValues = props.facetValues ?? {};
    if (props.facetGroupKey) {
      const entries =
        props.pageEntries ??
        props.tasks.map((task) => ({
          kind: "task" as const,
          task_id: task.id,
          parent_id: task.parentTaskId,
          depth: task.parentTaskId ? 1 : 0,
        }));
      return buildMobileFacetGroupedPage(props.tasks, entries, props.facetGroupKey, {
        facetValues,
        unassignedLabel: t("sidebar:groupUnassigned"),
        facetSortKey: props.facetSortKey,
      });
    }
    const pageGrouped =
      props.pageEntries !== undefined
        ? groupSidebarTaskPage(
            props.tasks,
            props.pageEntries,
            resultGroup,
            prefs.subtaskOrderByParentId,
          )
        : applyView(
            props.tasks,
            { ...view, group: resultGroup, sort: resultSort },
            {
              pinnedTaskIds: prefs.pinnedTaskIds,
              orderedTaskIds: prefs.orderedTaskIds,
              subtaskOrderByParentId: prefs.subtaskOrderByParentId,
            },
          );
    if (!props.facetSortKey) return pageGrouped;
    return sortMobileFacetPageGroups(pageGrouped, props.facetSortKey, facetValues);
  }, [
    props.facetGroupKey,
    props.facetSortKey,
    props.facetValues,
    resultGroup,
    resultSort,
    i18n.language,
    t,
    prefs.pinnedTaskIds,
    prefs.orderedTaskIds,
    props.pageEntries,
    props.tasks,
    prefs.subtaskOrderByParentId,
    view,
  ]);
}

function useMobileTaskListSwitcherProps(args: {
  props: MobileTaskListProps;
  view: ReturnType<typeof useEffectiveSidebarView>;
  resultSort: SidebarView["sort"];
  grouped: ReturnType<typeof groupSidebarTaskPage>;
  groupState: MobileTaskListGroupState;
  prefs: ReturnType<typeof useSidebarTaskPrefs>;
}) {
  const { props, view, resultSort, grouped, groupState, prefs } = args;
  const collapsedSubtaskParents = useAppStore((state) => state.collapsedSubtaskParents);
  const toggleSubtaskCollapsed = useAppStore((state) => state.toggleSubtaskCollapsed);
  const canReorder = !(
    props.facetSortKey ||
    props.facetGroupKey ||
    groupState.temporaryGroupOverride ||
    props.hasLinkSortOverride
  );
  return buildMobileTaskSwitcherProps(props, {
    grouped,
    collapsedGroupKeys: groupState.temporaryGroupKey
      ? groupState.facetCollapsedGroupKeys
      : view.collapsedGroups,
    onToggleGroup: groupState.toggleGroup,
    collapsedSubtaskParentIds: collapsedSubtaskParents,
    onToggleSubtasks: toggleSubtaskCollapsed,
    onTogglePin: prefs.togglePinnedTask,
    onReorderGroup: canReorder ? prefs.handleReorderGroup : undefined,
    onReorderSubtasks: canReorder ? prefs.handleReorderSubtasks : undefined,
    pinnedTaskIds: prefs.pinnedTaskIds,
    showActivityTime: sidebarSortHasKey(resultSort, "lastActivityAt"),
    groupIndent: view.groupIndent,
    taskRowPresentation: view.taskRow,
  });
}

function useMobileTaskListModel(props: MobileTaskListProps) {
  const activeView = useEffectiveSidebarView(props.workspaceId);
  const view = props.view ?? activeView;
  const resultSort = props.resultSort ?? view.sort;
  const resultGroup = props.resultGroup ?? view.group;
  const prefs = useSidebarTaskPrefs();
  const groupState = useMobileTaskListGroupState(props, view, resultGroup);
  const grouped = useMobileTaskListGroupedPage(props, view, resultSort, resultGroup, prefs);
  const switcherProps = useMobileTaskListSwitcherProps({
    props,
    view,
    resultSort,
    grouped,
    groupState,
    prefs,
  });
  return { switcherProps };
}

/**
 * The mobile task tree surface: renders the shared TaskSwitcher with the
 * mobile drawer's view state (grouping, ordering, collapse, reorder, nest).
 */
export function MobileTaskList(props: MobileTaskListProps) {
  const { switcherProps } = useMobileTaskListModel(props);
  return (
    <>
      {!props.loadError && (
        <SidebarTaskQueryStatus
          error={props.pageError ?? null}
          pending={props.pagePending ?? false}
          hasPage={Boolean(props.page)}
          canRetry={props.pageCanRetry}
          onRetry={props.onPageRetry ?? (() => undefined)}
          touchTargets
        />
      )}
      {!(props.pageError && !props.page && !props.loadError) && (
        <TaskSwitcher {...switcherProps} emptyState={props.emptyState} />
      )}
      <SidebarTaskPagination
        page={props.page ?? null}
        pending={props.pagePending === true || props.pageTransitioning === true}
        onPageChange={(nextPage) =>
          props.onPageChange?.(nextPage, () =>
            props.scrollContainerRef?.current?.scrollTo({ top: 0 }),
          )
        }
        touchTargets
      />
    </>
  );
}
