"use client";

import type { StepDef, TaskLinkHandler, TaskSwitcherItem } from "./task-switcher-types";
import { TaskItem } from "./task-item";
import { TaskItemWithContextMenu } from "./task-switcher-context-menu";
import { dispatchSidebarRowClick } from "./task-switcher-click";
import type { TaskMoveWorkflow } from "@/components/task/task-move-context-menu";

export type SubtaskToggleInfo = {
  subtaskCount: number;
  subtasksCollapsed: boolean;
  onToggleSubtasks: () => void;
};

export type TaskRowProps = {
  task: TaskSwitcherItem;
  isSubTask?: boolean;
  depth?: number;
  subtaskToggle?: SubtaskToggleInfo;
  workflows?: TaskMoveWorkflow[];
  stepsByWorkflowId?: Record<string, StepDef[]>;
  activeTaskId: string | null;
  selectedTaskId: string | null;
  onSelectTask: (taskId: string) => void;
  onEditTask?: (task: TaskSwitcherItem) => void;
  onRenameTask?: (taskId: string, currentTitle: string) => void;
  onArchiveTask?: (taskId: string) => void;
  onCreateSubtask?: (taskId: string, taskTitle: string) => void;
  onDeleteTask?: (taskId: string) => void;
  onDetachTask?: (taskId: string) => void;
  onLinkPullRequest?: TaskLinkHandler;
  onLinkIssue?: TaskLinkHandler;
  onLinkMergeRequest?: TaskLinkHandler;
  onLinkJiraTicket?: TaskLinkHandler;
  onLinkLinearIssue?: TaskLinkHandler;
  onLinkSentryIssue?: TaskLinkHandler;
  onMoveToStep?: (taskId: string, workflowId: string, targetStepId: string) => void;
  onTogglePin?: (taskId: string) => void;
  isPinned?: boolean;
  pinnedTaskIds?: string[];
  deletingTaskId?: string | null;
  selectedTaskIds?: Set<string>;
  onToggleSelectTask?: (taskId: string) => void;
  onSelectTaskRange?: (taskId: string) => void;
  onBulkArchive?: (taskIds: string[]) => void;
  onBulkDelete?: (taskIds: string[]) => void;
  onBulkPin?: (taskIds: string[]) => void;
  onBulkMove?: (taskIds: string[], targetWorkflowId: string, targetStepId: string) => void;
  onClearSelection?: () => void;
  isMixedWorkflowSelection?: boolean;
};

function archiveAware<T>(value: T | undefined, isArchived: boolean): T | undefined {
  return isArchived ? undefined : value;
}

function getContextMenuProps(props: TaskRowProps, isArchived: boolean) {
  return {
    onEditTask: archiveAware(props.onEditTask, isArchived),
    onRenameTask: archiveAware(props.onRenameTask, isArchived),
    onArchiveTask: archiveAware(props.onArchiveTask, isArchived),
    onCreateSubtask: archiveAware(props.onCreateSubtask, isArchived),
    onDeleteTask: props.onDeleteTask,
    onDetachTask: archiveAware(props.onDetachTask, isArchived),
    onLinkPullRequest: archiveAware(props.onLinkPullRequest, isArchived),
    onLinkIssue: archiveAware(props.onLinkIssue, isArchived),
    onLinkMergeRequest: archiveAware(props.onLinkMergeRequest, isArchived),
    onLinkJiraTicket: archiveAware(props.onLinkJiraTicket, isArchived),
    onLinkLinearIssue: archiveAware(props.onLinkLinearIssue, isArchived),
    onLinkSentryIssue: archiveAware(props.onLinkSentryIssue, isArchived),
    onMoveToStep: archiveAware(props.onMoveToStep, isArchived),
    onTogglePin: archiveAware(props.onTogglePin, isArchived),
    isPinned: isArchived ? false : props.isPinned,
    pinnedTaskIds: props.pinnedTaskIds,
    isDeleting: props.deletingTaskId === props.task.id,
    selectedTaskIds: archiveAware(props.selectedTaskIds, isArchived),
    onBulkArchive: archiveAware(props.onBulkArchive, isArchived),
    onBulkDelete: props.onBulkDelete,
    onBulkPin: archiveAware(props.onBulkPin, isArchived),
    onBulkMove: archiveAware(props.onBulkMove, isArchived),
    onClearSelection: archiveAware(props.onClearSelection, isArchived),
    isMixedWorkflowSelection: props.isMixedWorkflowSelection,
  };
}

type TaskRowItemProps = Pick<
  TaskRowProps,
  | "task"
  | "isSubTask"
  | "depth"
  | "subtaskToggle"
  | "activeTaskId"
  | "selectedTaskId"
  | "onSelectTask"
  | "selectedTaskIds"
  | "onToggleSelectTask"
  | "onSelectTaskRange"
  | "deletingTaskId"
  | "isPinned"
  | "stepsByWorkflowId"
> & { isArchived: boolean };

function TaskRowItem({
  task,
  isSubTask,
  depth,
  subtaskToggle,
  activeTaskId,
  selectedTaskId,
  onSelectTask,
  selectedTaskIds,
  onToggleSelectTask,
  onSelectTaskRange,
  deletingTaskId,
  isPinned,
  stepsByWorkflowId,
  isArchived,
}: TaskRowItemProps) {
  const taskSteps = task.workflowId ? stepsByWorkflowId?.[task.workflowId] : undefined;
  const isSelected = task.id === selectedTaskId || task.id === activeTaskId;
  const stepId = task.workflowStepId;
  const isDeleting = deletingTaskId === task.id;

  return (
    <TaskItem
      isMultiSelected={!isArchived && (selectedTaskIds?.has(task.id) ?? false)}
      onSelect={(e) =>
        dispatchSidebarRowClick(
          e,
          task.id,
          !isArchived && (selectedTaskIds?.size ?? 0) > 0,
          {
            onSelectTask,
            onToggleSelectTask: archiveAware(onToggleSelectTask, isArchived),
            onSelectTaskRange: archiveAware(onSelectTaskRange, isArchived),
          },
          isArchived,
        )
      }
      title={task.title}
      autopilot={task.autopilot}
      state={task.state}
      sessionState={task.sessionState}
      foregroundActivity={task.foregroundActivity}
      interrupted={task.interrupted}
      isArchived={task.isArchived}
      isSelected={isSelected}
      diffStats={task.diffStats}
      isRemoteExecutor={task.isRemoteExecutor}
      remoteExecutorType={task.remoteExecutorType}
      remoteExecutorName={task.remoteExecutorName}
      taskId={task.id}
      primarySessionId={task.primarySessionId ?? null}
      hasPendingClarification={task.hasPendingClarification}
      hasPendingPermission={task.hasPendingPermission}
      updatedAt={task.updatedAt}
      repositories={task.repositories}
      prInfo={task.prInfo}
      queuedCount={task.queuedCount}
      issueInfo={task.issueInfo}
      agentErrorMessage={task.agentErrorMessage}
      isSubTask={isSubTask}
      isOnLastWorkflowStep={!!stepId && taskSteps?.at(-1)?.id === stepId}
      depth={depth}
      subtaskCount={subtaskToggle?.subtaskCount}
      subtasksCollapsed={subtaskToggle?.subtasksCollapsed}
      onToggleSubtasks={subtaskToggle?.onToggleSubtasks}
      onClick={() => onSelectTask(task.id)}
      isDeleting={isDeleting}
      isPinned={isArchived ? false : isPinned}
    />
  );
}

export function TaskRow(props: TaskRowProps) {
  const { task, isSubTask, depth, subtaskToggle, workflows, stepsByWorkflowId } = props;
  const isArchived = task.isArchived === true;
  return (
    <TaskItemWithContextMenu
      task={task}
      workflows={workflows}
      stepsByWorkflowId={stepsByWorkflowId}
      steps={task.workflowId ? stepsByWorkflowId?.[task.workflowId] : undefined}
      {...getContextMenuProps(props, isArchived)}
    >
      <TaskRowItem
        {...props}
        isArchived={isArchived}
        isSubTask={isSubTask}
        depth={depth}
        subtaskToggle={subtaskToggle}
      />
    </TaskItemWithContextMenu>
  );
}
