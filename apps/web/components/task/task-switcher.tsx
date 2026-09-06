"use client";

import { memo, useLayoutEffect, useMemo, useRef } from "react";
import { useTranslation } from "react-i18next";
import { TaskSwitcherSkeleton } from "./task-switcher-group";
import { GroupSection, type GroupSectionProps, type TaskRowBaseProps } from "./task-switcher-tree";
import { useSharedGroupedSidebarList } from "./task-session-sidebar-grouped-view";
import type { TaskSwitcherProps } from "./task-switcher-types";

export type {
  StepDef,
  TaskLinkHandler,
  TaskSwitcherItem,
  TaskSwitcherProps,
} from "./task-switcher-types";
export { dispatchSidebarRowClick } from "./task-switcher-click";

/**
 * Rows name their repository unless the list is already grouped by it, where
 * the section header says it once for the whole group. A list whose grouping is
 * unknown (a hand-built `grouped` without `groupKey`) shows the label.
 */
function shouldShowRowRepository(grouped: TaskSwitcherProps["grouped"]): boolean {
  return grouped.groupKey !== "repository";
}

const TASK_ROW_HANDLER_KEYS = [
  "onSelectTask",
  "onEditTask",
  "onRenameTask",
  "onArchiveTask",
  "onCreateSubtask",
  "onDeleteTask",
  "onDetachTask",
  "onLinkPullRequest",
  "onLinkIssue",
  "onLinkMergeRequest",
  "onLinkJiraTicket",
  "onLinkLinearIssue",
  "onLinkSentryIssue",
  "onMoveToStep",
  "onRequestMoveOptions",
  "onBeforeMoveOptionsOpen",
  "onTogglePin",
  "onToggleSelectTask",
  "onSelectTaskRange",
  "onBulkArchive",
  "onBulkDelete",
  "onBulkPin",
  "onBulkMove",
  "onClearSelection",
] as const;

type TaskRowHandlerKey = (typeof TASK_ROW_HANDLER_KEYS)[number];
type StableTaskRowHandlers = {
  [Key in TaskRowHandlerKey]-?: NonNullable<TaskSwitcherProps[Key]>;
};

function useStableTaskRowHandlers(props: TaskSwitcherProps): StableTaskRowHandlers {
  // A memoized row must keep its function identities without retaining a
  // range-selection or dialog handler from an older committed task tree.
  const committedPropsRef = useRef(props);
  useLayoutEffect(() => {
    committedPropsRef.current = props;
  }, [props]);
  return useMemo(() => {
    const handlers: Partial<Record<TaskRowHandlerKey, (...args: unknown[]) => unknown>> = {};
    for (const key of TASK_ROW_HANDLER_KEYS) {
      handlers[key] = (...args: unknown[]) => {
        const handler = committedPropsRef.current[key];
        if (typeof handler === "function") {
          return (handler as (...handlerArgs: unknown[]) => unknown)(...args);
        }
      };
    }
    return handlers as StableTaskRowHandlers;
  }, []);
}

function optionalHandler<Key extends TaskRowHandlerKey>(
  props: TaskSwitcherProps,
  handlers: StableTaskRowHandlers,
  key: Key,
): StableTaskRowHandlers[Key] | undefined {
  return props[key] ? handlers[key] : undefined;
}

function buildTaskRowProps(
  props: TaskSwitcherProps,
  handlers: StableTaskRowHandlers,
): TaskRowBaseProps {
  return {
    workflows: props.workflows,
    stepsByWorkflowId: props.stepsByWorkflowId,
    activeTaskId: props.activeTaskId,
    selectedTaskId: props.selectedTaskId,
    showActivityTime: props.showActivityTime,
    taskRowPresentation: props.taskRowPresentation,
    showRepository: shouldShowRowRepository(props.grouped),
    onSelectTask: handlers.onSelectTask,
    onEditTask: optionalHandler(props, handlers, "onEditTask"),
    onRenameTask: optionalHandler(props, handlers, "onRenameTask"),
    onArchiveTask: optionalHandler(props, handlers, "onArchiveTask"),
    onCreateSubtask: optionalHandler(props, handlers, "onCreateSubtask"),
    onDeleteTask: optionalHandler(props, handlers, "onDeleteTask"),
    onDetachTask: optionalHandler(props, handlers, "onDetachTask"),
    onLinkPullRequest: optionalHandler(props, handlers, "onLinkPullRequest"),
    onLinkIssue: optionalHandler(props, handlers, "onLinkIssue"),
    onLinkMergeRequest: optionalHandler(props, handlers, "onLinkMergeRequest"),
    onLinkJiraTicket: optionalHandler(props, handlers, "onLinkJiraTicket"),
    onLinkLinearIssue: optionalHandler(props, handlers, "onLinkLinearIssue"),
    onLinkSentryIssue: optionalHandler(props, handlers, "onLinkSentryIssue"),
    onMoveToStep: optionalHandler(props, handlers, "onMoveToStep"),
    onRequestMoveOptions: optionalHandler(props, handlers, "onRequestMoveOptions"),
    onBeforeMoveOptionsOpen: optionalHandler(props, handlers, "onBeforeMoveOptionsOpen"),
    onTogglePin: optionalHandler(props, handlers, "onTogglePin"),
    pinnedTaskIds: props.pinnedTaskIds,
    deletingTaskId: props.deletingTaskId,
    archivingTaskId: props.archivingTaskId,
    isArchiving: props.isArchiving,
    selectedTaskIds: props.selectedTaskIds,
    onToggleSelectTask: optionalHandler(props, handlers, "onToggleSelectTask"),
    onSelectTaskRange: optionalHandler(props, handlers, "onSelectTaskRange"),
    onBulkArchive: optionalHandler(props, handlers, "onBulkArchive"),
    onBulkDelete: optionalHandler(props, handlers, "onBulkDelete"),
    onBulkPin: optionalHandler(props, handlers, "onBulkPin"),
    onBulkMove: optionalHandler(props, handlers, "onBulkMove"),
    onClearSelection: optionalHandler(props, handlers, "onClearSelection"),
    isMixedWorkflowSelection: props.isMixedWorkflowSelection,
  };
}

function shallowRowPropsEqual(previous: TaskRowBaseProps, next: TaskRowBaseProps): boolean {
  const keys = Object.keys(previous) as Array<keyof TaskRowBaseProps>;
  return (
    keys.length === Object.keys(next).length &&
    keys.every((key) => Object.is(previous[key], next[key]))
  );
}

function useTaskRowProps(props: TaskSwitcherProps): TaskRowBaseProps {
  const handlers = useStableTaskRowHandlers(props);
  const previousRef = useRef<TaskRowBaseProps | null>(null);
  const next = buildTaskRowProps(props, handlers);
  if (previousRef.current && shallowRowPropsEqual(previousRef.current, next)) {
    return previousRef.current;
  }
  previousRef.current = next;
  return next;
}

function buildGroupSectionProps(
  props: TaskSwitcherProps,
  grouped: TaskSwitcherProps["grouped"],
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
    subTasksByParentId: grouped.subTasksByParentId,
    rowProps,
    pinnedSet,
    isCollapsed: collapsedSet.has(group.key),
    onToggleGroup: props.onToggleGroup,
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
  const { isLoading = false, loadError, onRetryLoad, retryLabel, totalTaskCount } = props;
  const grouped = useSharedGroupedSidebarList(props.grouped);
  const pinnedSet = useMemo(() => new Set(props.pinnedTaskIds ?? []), [props.pinnedTaskIds]);
  const rowProps = useTaskRowProps(props);
  const collapsedSet = useMemo(
    () => new Set(props.collapsedGroupKeys ?? []),
    [props.collapsedGroupKeys],
  );

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
        <div
          data-slot="task-switcher-empty-state"
          className="px-3 py-3 text-xs text-muted-foreground"
        >
          {t("sidebar:noTasksYet")}
        </div>
      </>
    );
  }

  const showHeaders =
    grouped.groups.length > 1 ||
    (grouped.groups.length === 1 && grouped.groups[0].key !== "__all__");

  return (
    <div>
      {loadErrorNotice}
      {grouped.groups.map((group) => (
        <GroupSection
          key={group.key}
          {...buildGroupSectionProps(props, grouped, {
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
