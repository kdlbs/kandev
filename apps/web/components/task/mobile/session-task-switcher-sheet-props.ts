import type { TaskSwitcherProps } from "../task-switcher";
import type { MobileTaskListProps } from "./session-task-switcher-sheet";

/** Maps mobile view state and actions to the shared task switcher rows. */
export function buildMobileTaskSwitcherProps(
  props: MobileTaskListProps,
  helpers: {
    grouped: TaskSwitcherProps["grouped"];
    collapsedGroupKeys: string[];
    onToggleGroup: (groupKey: string) => void;
    collapsedSubtaskParentIds: string[];
    onToggleSubtasks: (parentTaskId: string) => void;
    onTogglePin: (taskId: string) => void;
    onReorderGroup: (groupTaskIds: string[]) => void;
    onReorderSubtasks: (parentTaskId: string, orderedSubtaskIds: string[]) => void;
    pinnedTaskIds: string[];
    showActivityTime: boolean;
  },
): TaskSwitcherProps {
  return {
    grouped: helpers.grouped,
    workflows: props.workflows,
    stepsByWorkflowId: props.stepsByWorkflowId,
    activeTaskId: props.activeTaskId,
    selectedTaskId: props.selectedTaskId,
    collapsedGroupKeys: helpers.collapsedGroupKeys,
    showActivityTime: helpers.showActivityTime,
    onToggleGroup: helpers.onToggleGroup,
    collapsedSubtaskParentIds: helpers.collapsedSubtaskParentIds,
    onToggleSubtasks: helpers.onToggleSubtasks,
    onSelectTask: props.onSelectTask,
    onEditTask: props.onEditTask,
    onRenameTask: props.onRenameTask,
    onCreateSubtask: props.onCreateSubtask,
    onArchiveTask: props.onArchiveTask,
    onDeleteTask: props.onDeleteTask,
    onDetachTask: props.onDetachTask,
    onNestTask: props.onNestTask,
    onLinkPullRequest: props.onLinkPullRequest,
    onLinkIssue: props.onLinkIssue,
    onLinkMergeRequest: props.onLinkMergeRequest,
    onLinkJiraTicket: props.onLinkJiraTicket,
    onLinkLinearIssue: props.onLinkLinearIssue,
    onLinkSentryIssue: props.onLinkSentryIssue,
    onTogglePin: helpers.onTogglePin,
    onReorderGroup: helpers.onReorderGroup,
    onReorderSubtasks: helpers.onReorderSubtasks,
    pinnedTaskIds: helpers.pinnedTaskIds,
    deletingTaskId: props.deletingTaskId,
    archivingTaskId: props.archivingTaskId,
    isArchiving: props.isArchiving,
    isLoading: props.isLoading,
    loadError: props.loadError,
    onRetryLoad: props.onRetryLoad,
    retryLabel: props.retryLabel,
    totalTaskCount: props.tasks.length,
  };
}
