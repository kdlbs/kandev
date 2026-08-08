"use client";

import { memo, useMemo } from "react";
import { useTranslation } from "react-i18next";
import { TaskSwitcherSkeleton } from "./task-switcher-group";
import { GroupSection, type GroupSectionProps, type TaskRowBaseProps } from "./task-switcher-tree";
import type { TaskSwitcherProps } from "./task-switcher-types";

export type {
  StepDef,
  TaskLinkHandler,
  TaskSwitcherItem,
  TaskSwitcherProps,
} from "./task-switcher-types";
export { dispatchSidebarRowClick } from "./task-switcher-click";

function buildTaskRowProps(props: TaskSwitcherProps): TaskRowBaseProps {
  return {
    workflows: props.workflows,
    stepsByWorkflowId: props.stepsByWorkflowId,
    activeTaskId: props.activeTaskId,
    selectedTaskId: props.selectedTaskId,
    onSelectTask: props.onSelectTask,
    onEditTask: props.onEditTask,
    onRenameTask: props.onRenameTask,
    onArchiveTask: props.onArchiveTask,
    onCreateSubtask: props.onCreateSubtask,
    onDeleteTask: props.onDeleteTask,
    onDetachTask: props.onDetachTask,
    onLinkPullRequest: props.onLinkPullRequest,
    onLinkIssue: props.onLinkIssue,
    onLinkMergeRequest: props.onLinkMergeRequest,
    onLinkJiraTicket: props.onLinkJiraTicket,
    onLinkLinearIssue: props.onLinkLinearIssue,
    onLinkSentryIssue: props.onLinkSentryIssue,
    onMoveToStep: props.onMoveToStep,
    onTogglePin: props.onTogglePin,
    pinnedTaskIds: props.pinnedTaskIds,
    deletingTaskId: props.deletingTaskId,
    selectedTaskIds: props.selectedTaskIds,
    onToggleSelectTask: props.onToggleSelectTask,
    onSelectTaskRange: props.onSelectTaskRange,
    onBulkArchive: props.onBulkArchive,
    onBulkDelete: props.onBulkDelete,
    onBulkPin: props.onBulkPin,
    onBulkMove: props.onBulkMove,
    onClearSelection: props.onClearSelection,
    isMixedWorkflowSelection: props.isMixedWorkflowSelection,
  };
}

function buildGroupSectionProps(
  props: TaskSwitcherProps,
  options: {
    group: GroupSectionProps["group"];
    rowProps: TaskRowBaseProps;
    pinnedSet: Set<string>;
    collapsedSet: Set<string>;
    showHeader: boolean;
  },
): GroupSectionProps {
  const { group, rowProps, pinnedSet, collapsedSet, showHeader } = options;
  return {
    group,
    subTasksByParentId: props.grouped.subTasksByParentId,
    rowProps,
    pinnedSet,
    isCollapsed: collapsedSet.has(group.key),
    onToggleCollapsed: () => props.onToggleGroup?.(group.key),
    collapsedSubtaskParentIds: props.collapsedSubtaskParentIds,
    onToggleSubtasks: props.onToggleSubtasks,
    showHeader,
    onReorderGroup: props.onReorderGroup,
    onReorderSubtasks: props.onReorderSubtasks,
    onNestTask: props.onNestTask,
  };
}

function LoadErrorNotice({
  error,
  onRetry,
  retryLabel,
}: {
  error?: string | null;
  onRetry?: () => void;
  retryLabel?: string;
}) {
  if (!error) return null;
  return (
    <div
      className="flex items-center gap-2 px-3 py-2 text-xs text-destructive"
      data-testid="sidebar-task-load-error"
    >
      <span className="min-w-0 flex-1">{error}</span>
      {onRetry && retryLabel && (
        <button type="button" className="shrink-0 underline underline-offset-2" onClick={onRetry}>
          {retryLabel}
        </button>
      )}
    </div>
  );
}

export const TaskSwitcher = memo(function TaskSwitcher(props: TaskSwitcherProps) {
  const { t } = useTranslation("sidebar");
  const { grouped, isLoading = false, loadError, onRetryLoad, retryLabel, totalTaskCount } = props;
  const pinnedSet = useMemo(() => new Set(props.pinnedTaskIds ?? []), [props.pinnedTaskIds]);

  if (isLoading) return <TaskSwitcherSkeleton />;

  const totalTasks =
    totalTaskCount ?? grouped.groups.reduce((sum, group) => sum + group.tasks.length, 0);
  const loadErrorNotice = (
    <LoadErrorNotice error={loadError} onRetry={onRetryLoad} retryLabel={retryLabel} />
  );
  if (totalTasks === 0) {
    return (
      <>
        {loadErrorNotice}
        <div className="px-3 py-3 text-xs text-muted-foreground">{t("sidebar:noTasksYet")}</div>
      </>
    );
  }

  const collapsedSet = new Set(props.collapsedGroupKeys ?? []);
  const showHeaders =
    grouped.groups.length > 1 ||
    (grouped.groups.length === 1 && grouped.groups[0].key !== "__all__");
  const rowProps = buildTaskRowProps(props);

  return (
    <div>
      {loadErrorNotice}
      {grouped.groups.map((group) => (
        <GroupSection
          key={group.key}
          {...buildGroupSectionProps(props, {
            group,
            rowProps,
            pinnedSet,
            collapsedSet,
            showHeader: showHeaders,
          })}
        />
      ))}
    </div>
  );
});
