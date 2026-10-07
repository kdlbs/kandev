"use client";
import { useCallback, useMemo, type MutableRefObject } from "react";
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
function useSidebarGroupToggle(viewId: string) {
  const toggleSidebarGroupCollapsed = useAppStore((s) => s.toggleSidebarGroupCollapsed);
  return useCallback(
    (groupKey: string) => toggleSidebarGroupCollapsed(viewId, groupKey),
    [toggleSidebarGroupCollapsed, viewId],
  );
}

export type MobileTaskListProps = {
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
  onPageChange?: (page: number, afterSuccess: () => void) => void;
  onPageRetry?: () => void;
  scrollContainerRef?: MutableRefObject<HTMLDivElement | null>;
};

/**
 * The mobile task tree surface: renders the shared TaskSwitcher with the
 * mobile drawer's view state (grouping, ordering, collapse, reorder, nest).
 */
export function MobileTaskList(props: MobileTaskListProps) {
  const view = useEffectiveSidebarView();
  const {
    pinnedTaskIds,
    orderedTaskIds,
    subtaskOrderByParentId,
    togglePinnedTask,
    handleReorderGroup,
    handleReorderSubtasks,
  } = useSidebarTaskPrefs();
  const collapsedSubtaskParents = useAppStore((s) => s.collapsedSubtaskParents);
  const toggleSubtaskCollapsed = useAppStore((s) => s.toggleSubtaskCollapsed);
  const handleToggleGroup = useSidebarGroupToggle(view.id);
  // See useGroupedSidebarView: the executorType group label is catalog-backed.
  const { i18n } = useTranslation();
  const grouped = useMemo(() => {
    if (props.pageEntries !== undefined) {
      return groupSidebarTaskPage(
        props.tasks,
        props.pageEntries,
        view.group,
        subtaskOrderByParentId,
      );
    }
    return applyView(props.tasks, view, {
      pinnedTaskIds,
      orderedTaskIds,
      subtaskOrderByParentId,
    });
  }, [
    i18n.language,
    pinnedTaskIds,
    orderedTaskIds,
    props.pageEntries,
    props.tasks,
    subtaskOrderByParentId,
    view,
  ]);
  const switcherProps = buildMobileTaskSwitcherProps(props, {
    grouped,
    collapsedGroupKeys: view.collapsedGroups,
    onToggleGroup: handleToggleGroup,
    collapsedSubtaskParentIds: collapsedSubtaskParents,
    onToggleSubtasks: toggleSubtaskCollapsed,
    onTogglePin: togglePinnedTask,
    onReorderGroup: handleReorderGroup,
    onReorderSubtasks: handleReorderSubtasks,
    pinnedTaskIds,
    showActivityTime: sidebarSortHasKey(view.sort, "lastActivityAt"),
    groupIndent: view.groupIndent,
    taskRowPresentation: view.taskRow,
  });
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
      {!(props.pageError && !props.page && !props.loadError) && <TaskSwitcher {...switcherProps} />}
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
