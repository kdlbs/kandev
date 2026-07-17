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
  effectiveView: { collapsedGroups: TaskSwitcherComponentProps["collapsedGroupKeys"] };
  handleToggleGroup: TaskSwitcherComponentProps["onToggleGroup"];
  collapsedSubtaskParents: TaskSwitcherComponentProps["collapsedSubtaskParentIds"];
  toggleSubtaskCollapsed: TaskSwitcherComponentProps["onToggleSubtasks"];
  sidebarActions: ReturnType<typeof useSidebarActions>;
  taskLinkHandlers: ReturnType<typeof useSidebarTaskLinking>;
  pinnedTaskIds: TaskSwitcherComponentProps["pinnedTaskIds"];
  togglePinnedTask: TaskSwitcherComponentProps["onTogglePin"];
  handleReorderGroup: TaskSwitcherComponentProps["onReorderGroup"];
  handleReorderSubtasks: TaskSwitcherComponentProps["onReorderSubtasks"];
  isLoadingWorkflow: boolean;
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
    onToggleGroup: args.handleToggleGroup,
    collapsedSubtaskParentIds: args.collapsedSubtaskParents,
    onToggleSubtasks: args.toggleSubtaskCollapsed,
    onSelectTask: args.sidebarActions.handleSelectTask,
    onRenameTask: args.sidebarActions.handleRenameTask,
    onArchiveTask: args.sidebarActions.handleArchiveTask,
    onDeleteTask: args.sidebarActions.handleDeleteTask,
    ...args.taskLinkHandlers,
    onMoveToStep: args.sidebarActions.handleMoveToStep,
    onTogglePin: args.togglePinnedTask,
    onReorderGroup: args.handleReorderGroup,
    onReorderSubtasks: args.handleReorderSubtasks,
    pinnedTaskIds: args.pinnedTaskIds,
    deletingTaskId: args.sidebarActions.deletingTaskId,
    isLoading: args.isLoadingWorkflow,
    totalTaskCount: args.totalTaskCount,
    ...args.selection.switcherProps,
  };
}
