import type { ComponentProps } from "react";
import type { TaskSwitcher } from "./task-switcher";
import type { useSidebarActions } from "./task-session-sidebar";
import type { useSidebarTaskLinking } from "./task-session-sidebar-task-linking";
import type { useSidebarSelection } from "./task-session-sidebar-selection";

type TaskSwitcherComponentProps = ComponentProps<typeof TaskSwitcher>;

/**
 * Maps the sidebar's assembled hook state onto `TaskSwitcher`'s prop names.
 * Pulled out of `TaskSessionSidebar` so the component body stays focused on
 * hook orchestration rather than prop plumbing.
 */
export function buildTaskSwitcherProps(args: {
  grouped: TaskSwitcherComponentProps["grouped"];
  workflows: TaskSwitcherComponentProps["workflows"];
  stepsByWorkflowId: TaskSwitcherComponentProps["stepsByWorkflowId"];
  highlightedTaskId: string | null;
  highlightedSelectedTaskId: string | null;
  effectiveView: {
    collapsedGroups: TaskSwitcherComponentProps["collapsedGroupKeys"];
    sort: { key: string };
  };
  handleToggleGroup: TaskSwitcherComponentProps["onToggleGroup"];
  collapsedSubtaskParents: TaskSwitcherComponentProps["collapsedSubtaskParentIds"];
  toggleSubtaskCollapsed: TaskSwitcherComponentProps["onToggleSubtasks"];
  sidebarActions: ReturnType<typeof useSidebarActions>;
  taskLinkHandlers: ReturnType<typeof useSidebarTaskLinking>;
  pinnedTaskIds: TaskSwitcherComponentProps["pinnedTaskIds"];
  togglePinnedTask: TaskSwitcherComponentProps["onTogglePin"];
  handleReorderGroup: TaskSwitcherComponentProps["onReorderGroup"];
  handleReorderSubtasks: TaskSwitcherComponentProps["onReorderSubtasks"];
  handleNestTask: TaskSwitcherComponentProps["onNestTask"];
  isLoadingWorkflow: boolean;
  archivedError: string | null;
  retryArchivedTasks: () => void;
  archivedLoadErrorLabel: string;
  archivedRetryLabel: string;
  totalTaskCount: number;
  selection: ReturnType<typeof useSidebarSelection>;
}): TaskSwitcherComponentProps {
  return {
    grouped: args.grouped,
    workflows: args.workflows,
    stepsByWorkflowId: args.stepsByWorkflowId,
    activeTaskId: args.highlightedTaskId,
    selectedTaskId: args.highlightedSelectedTaskId,
    collapsedGroupKeys: args.effectiveView.collapsedGroups,
    showActivityTime: args.effectiveView.sort.key === "lastActivityAt",
    onToggleGroup: args.handleToggleGroup,
    collapsedSubtaskParentIds: args.collapsedSubtaskParents,
    onToggleSubtasks: args.toggleSubtaskCollapsed,
    onSelectTask: args.sidebarActions.handleSelectTask,
    onEditTask: args.sidebarActions.handleEditTask,
    onRenameTask: args.sidebarActions.handleRenameTask,
    onCreateSubtask: args.sidebarActions.handleCreateSubtask,
    onArchiveTask: args.sidebarActions.handleArchiveTask,
    onDeleteTask: args.sidebarActions.handleDeleteTask,
    onDetachTask: args.sidebarActions.handleDetachTask,
    ...args.taskLinkHandlers,
    onMoveToStep: args.sidebarActions.handleMoveToStep,
    onTogglePin: args.togglePinnedTask,
    onReorderGroup: args.handleReorderGroup,
    onReorderSubtasks: args.handleReorderSubtasks,
    onNestTask: args.handleNestTask,
    pinnedTaskIds: args.pinnedTaskIds,
    deletingTaskId: args.sidebarActions.deletingTaskId,
    archivingTaskId: args.sidebarActions.archivingTaskId,
    isArchiving: args.sidebarActions.isArchiving,
    isLoading: args.isLoadingWorkflow,
    loadError: args.archivedError ? args.archivedLoadErrorLabel : null,
    onRetryLoad: args.retryArchivedTasks,
    retryLabel: args.archivedRetryLabel,
    totalTaskCount: args.totalTaskCount,
    ...args.selection.switcherProps,
  };
}
