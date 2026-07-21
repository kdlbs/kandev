"use client";

import { useState } from "react";
import { useDraggable } from "@dnd-kit/core";
import { KanbanCardContextMenu } from "@/components/kanban-card-context-menu";
import { KanbanCardShell } from "@/components/kanban-card-content";
import {
  buildKanbanCardMenuEntries,
  useKanbanCardMoveTargets,
} from "@/components/kanban-card-menu-items";
import { useAppStore } from "@/components/state-provider";
import { TaskArchiveConfirmDialog } from "@/components/task/task-archive-confirm-dialog";
import { TaskDeleteConfirmDialog } from "@/components/task/task-delete-confirm-dialog";
import { TaskDetachConfirmDialog } from "@/components/task/task-detach-confirm-dialog";
import {
  TaskExternalLinkDialog,
  type ExternalLinkProvider,
} from "@/components/task/task-external-link-dialog";
import type { KanbanExternalLinkAvailability } from "./kanban-external-link-availability";
import { TaskGitHubIssueDialog } from "@/components/task/task-github-issue-dialog";
import { TaskGitHubPRDialog } from "@/components/task/task-github-pr-dialog";
import { useTaskWorkflowMove } from "@/hooks/use-task-workflow-move";
import { useTaskMultiSelectStore } from "@/hooks/use-task-multi-select";
import { useDetachTask } from "@/hooks/use-detach-task";
import { repositorySlug } from "@/lib/repository-slug";
import { formatUserHomePath } from "@/lib/utils";
import {
  repositoryId as toRepositoryId,
  type Repository,
  type TaskPendingAction,
  type TaskState,
} from "@/lib/types/http";

const EMPTY_REPOSITORIES: Repository[] = [];

export interface Task {
  id: string;
  title: string;
  workflowStepId: string;
  state?: TaskState;
  description?: string;
  position?: number;
  repositoryId?: string;
  /** All repositories linked to the task; used to render a "+N" chip for multi-repo. */
  repositories?: Array<{ id: string; repository_id: string; position: number }>;
  sessionCount?: number | null;
  primarySessionId?: string | null;
  /**
   * Primary session's runtime state. Decoupled from `state` (the workflow
   * column). Used to suppress the running-spinner when the agent has already
   * finished — the workflow may leave the task in IN_PROGRESS for review.
   */
  primarySessionState?: string | null;
  primarySessionPendingAction?: TaskPendingAction | null;
  reviewStatus?: "pending" | "approved" | "changes_requested" | "rejected" | null;
  primaryExecutorId?: string | null;
  primaryExecutorType?: string | null;
  primaryExecutorName?: string | null;
  isRemoteExecutor?: boolean;
  parentTaskId?: string | null;
  workspaceMode?: "inherit_parent" | "new_workspace" | "shared_group";
  updatedAt?: string;
  createdAt?: string;
  issueUrl?: string;
  issueNumber?: number;
}

export type RepositoryChip = {
  label: string;
  path?: string;
};

export interface WorkflowStep {
  id: string;
  title: string;
  color: string;
  events?: {
    on_enter?: Array<{ type: string; config?: Record<string, unknown> }>;
    on_turn_start?: Array<{ type: string; config?: Record<string, unknown> }>;
    on_turn_complete?: Array<{ type: string; config?: Record<string, unknown> }>;
    on_exit?: Array<{ type: string; config?: Record<string, unknown> }>;
  };
}

interface KanbanCardProps {
  task: Task;
  workspaceId: string | null;
  externalLinkAvailability: KanbanExternalLinkAvailability;
  /** Display labels and hover paths of every repository linked to the task, primary first. */
  repositoryChips?: RepositoryChip[];
  onClick?: (task: Task) => void;
  onEdit?: (task: Task) => void;
  onDelete?: (task: Task, opts?: { cascade?: boolean }) => void;
  onArchive?: (task: Task, opts?: { cascade?: boolean }) => void;
  onOpenFullPage?: (task: Task) => void;
  onMove?: (task: Task, targetStepId: string) => void;
  steps?: WorkflowStep[];
  showMaximizeButton?: boolean;
  isDeleting?: boolean;
  isArchiving?: boolean;
  isSelected?: boolean;
  selectedIds?: Set<string>;
  onToggleSelect?: (taskId: string) => void;
  /** Shift-click range select within this card's column. */
  onRangeSelect?: (taskId: string) => void;
  isMultiSelectMode?: boolean;
}

function useKanbanCardMoveMenuActions({
  task,
  steps,
  isSelected,
  selectedIds,
  onMove,
}: Pick<KanbanCardProps, "task" | "steps" | "isSelected" | "selectedIds" | "onMove">) {
  const moveTargets = useKanbanCardMoveTargets(task.id, steps);
  const moveTasks = useTaskWorkflowMove();
  const { sortByDisplayOrder, getWorkflowIdForTask } = useTaskMultiSelectStore();

  const runMoveTasks = (
    taskIds: string[],
    workflowId: string,
    stepId: string,
    destination: "step" | "workflow",
  ) => {
    void moveTasks(taskIds, workflowId, stepId, destination).catch(() => {
      // useTaskWorkflowMove already shows the failure toast.
    });
  };
  const moveToStepFromDropdown = (stepId: string) => {
    if (onMove) {
      onMove(task, stepId);
      return;
    }
    if (moveTargets.currentWorkflowId) {
      runMoveTasks([task.id], moveTargets.currentWorkflowId, stepId, "step");
    }
  };
  const selectedTaskIds = isSelected && selectedIds?.size ? [...selectedIds] : [task.id];
  const orderedSelectedIds = () => sortByDisplayOrder(selectedTaskIds);
  const isMixedWorkflowSelection =
    selectedTaskIds.length > 1 &&
    new Set(selectedTaskIds.map((id) => getWorkflowIdForTask(id))).size > 1;
  const moveSelectedToStep = (stepId: string) => {
    if (selectedTaskIds.length === 1 && selectedTaskIds[0] === task.id && onMove) {
      onMove(task, stepId);
      return;
    }
    if (!moveTargets.currentWorkflowId) return;
    runMoveTasks(orderedSelectedIds(), moveTargets.currentWorkflowId, stepId, "step");
  };

  return {
    moveTargets,
    moveToStepFromDropdown,
    moveSelectedToStep: isMixedWorkflowSelection ? undefined : moveSelectedToStep,
    sendTaskToWorkflow: (workflowId: string, stepId: string) => {
      runMoveTasks([task.id], workflowId, stepId, "workflow");
    },
    sendSelectionToWorkflow: (workflowId: string, stepId: string) => {
      runMoveTasks(orderedSelectedIds(), workflowId, stepId, "workflow");
    },
  };
}

function externalLinkHandlers(
  availability: KanbanCardProps["externalLinkAvailability"],
  setExternalLinkProvider: (provider: ExternalLinkProvider) => void,
) {
  return {
    onLinkJiraTicket: availability.jira ? () => setExternalLinkProvider("jira") : undefined,
    onLinkLinearIssue: availability.linear ? () => setExternalLinkProvider("linear") : undefined,
    onLinkSentryIssue: availability.sentry ? () => setExternalLinkProvider("sentry") : undefined,
  };
}

function useKanbanCardMenus({
  task,
  steps,
  isDeleting,
  isArchiving,
  isSelected,
  selectedIds,
  onEdit,
  onDelete,
  onArchive,
  onMove,
  externalLinkAvailability,
}: Pick<
  KanbanCardProps,
  | "task"
  | "externalLinkAvailability"
  | "steps"
  | "isDeleting"
  | "isArchiving"
  | "isSelected"
  | "selectedIds"
  | "onEdit"
  | "onDelete"
  | "onArchive"
  | "onMove"
>) {
  const moveMenu = useKanbanCardMoveMenuActions({ task, steps, isSelected, selectedIds, onMove });
  const [showDeleteConfirm, setShowDeleteConfirm] = useState(false);
  const [showArchiveConfirm, setShowArchiveConfirm] = useState(false);
  const [showDetachConfirm, setShowDetachConfirm] = useState(false);
  const [showPRDialog, setShowPRDialog] = useState(false);
  const [showIssueDialog, setShowIssueDialog] = useState(false);
  const [externalLinkProvider, setExternalLinkProvider] = useState<ExternalLinkProvider | null>(
    null,
  );
  const { detachTask, detachingTaskId } = useDetachTask();
  const isDetaching = detachingTaskId === task.id;
  const disabled = Boolean(isDeleting || isArchiving || isDetaching);
  const actingOnMultiSelection = Boolean(isSelected && selectedIds && selectedIds.size > 1);

  const handleDetachConfirm = async () => {
    try {
      await detachTask(task.id);
      setShowDetachConfirm(false);
    } catch (error) {
      console.error("Failed to detach task:", error);
    }
  };

  const menuBase = {
    currentWorkflowId: moveMenu.moveTargets.currentWorkflowId,
    currentStepId: task.workflowStepId,
    workflows: moveMenu.moveTargets.workflowItems,
    stepsByWorkflowId: moveMenu.moveTargets.stepsByWorkflowId,
    disabled,
    isDeleting,
    isArchiving,
    isDetaching,
    parentTaskId: task.parentTaskId,
    onEdit: onEdit ? () => onEdit(task) : undefined,
    onArchive: onArchive ? () => setShowArchiveConfirm(true) : undefined,
    onDelete: onDelete ? () => setShowDeleteConfirm(true) : undefined,
    onDetach:
      task.parentTaskId && !actingOnMultiSelection ? () => setShowDetachConfirm(true) : undefined,
    onLinkPullRequest: () => setShowPRDialog(true),
    onLinkIssue: () => setShowIssueDialog(true),
    ...externalLinkHandlers(externalLinkAvailability, setExternalLinkProvider),
  };

  return {
    dropdownMenuEntries: buildKanbanCardMenuEntries({
      ...menuBase,
      onMoveToStep: moveMenu.moveToStepFromDropdown,
      onSendToWorkflow: moveMenu.sendTaskToWorkflow,
    }),
    contextMenuEntries: buildKanbanCardMenuEntries({
      ...menuBase,
      onMoveToStep: moveMenu.moveSelectedToStep,
      onSendToWorkflow: moveMenu.sendSelectionToWorkflow,
    }),
    showDeleteConfirm,
    setShowDeleteConfirm,
    showArchiveConfirm,
    setShowArchiveConfirm,
    showDetachConfirm,
    setShowDetachConfirm,
    isDetaching,
    handleDetachConfirm,
    showPRDialog,
    setShowPRDialog,
    showIssueDialog,
    setShowIssueDialog,
    externalLinkProvider,
    setExternalLinkProvider,
  };
}

type KanbanCardMenuState = ReturnType<typeof useKanbanCardMenus>;

function KanbanCardDialogs({
  task,
  workspaceId,
  repositories,
  menu,
  isDeleting,
  isArchiving,
  onDelete,
  onArchive,
}: {
  task: Task;
  workspaceId: string | null;
  repositories: Repository[];
  menu: KanbanCardMenuState;
  isDeleting?: boolean;
  isArchiving?: boolean;
  onDelete?: KanbanCardProps["onDelete"];
  onArchive?: KanbanCardProps["onArchive"];
}) {
  return (
    <>
      <TaskDeleteConfirmDialog
        open={menu.showDeleteConfirm}
        onOpenChange={menu.setShowDeleteConfirm}
        taskTitle={task.title}
        taskId={task.id}
        executorType={task.primaryExecutorType}
        isDeleting={isDeleting}
        onConfirm={({ cascade }) => onDelete?.(task, { cascade })}
      />
      <TaskArchiveConfirmDialog
        open={menu.showArchiveConfirm}
        onOpenChange={menu.setShowArchiveConfirm}
        taskTitle={task.title}
        taskId={task.id}
        executorType={task.primaryExecutorType}
        isArchiving={isArchiving}
        onConfirm={({ cascade }) => onArchive?.(task, { cascade })}
      />
      <TaskDetachConfirmDialog
        open={menu.showDetachConfirm}
        onOpenChange={menu.setShowDetachConfirm}
        taskTitle={task.title}
        sharesParentWorkspace={task.workspaceMode === "inherit_parent"}
        isDetaching={menu.isDetaching}
        onConfirm={menu.handleDetachConfirm}
      />
      <TaskGitHubPRDialog
        open={menu.showPRDialog}
        onOpenChange={menu.setShowPRDialog}
        task={task}
        repositories={repositories}
      />
      <TaskGitHubIssueDialog
        open={menu.showIssueDialog}
        onOpenChange={menu.setShowIssueDialog}
        task={task}
        repositories={repositories}
      />
      {menu.externalLinkProvider && workspaceId && (
        <TaskExternalLinkDialog
          open={true}
          onOpenChange={(open) => {
            if (!open) menu.setExternalLinkProvider(null);
          }}
          provider={menu.externalLinkProvider}
          task={task}
          workspaceId={workspaceId}
        />
      )}
    </>
  );
}

/**
 * Cmd/Ctrl-click toggles a single card; Shift-click range-selects within the
 * column; either modifier enters multi-select mode without the toggle button.
 * A plain click toggles while in multi-select mode, otherwise previews/opens.
 */
/** @internal Exported for unit testing the four-branch click dispatch. */
export function dispatchKanbanCardClick(
  e: React.MouseEvent,
  taskId: string,
  task: Task,
  handlers: {
    onToggleSelect?: (taskId: string) => void;
    onRangeSelect?: (taskId: string) => void;
    onClick?: (task: Task) => void;
    isMultiSelectMode?: boolean;
  },
): void {
  // Only intercept a modifier click when the matching handler is wired, so a
  // card rendered without selection handlers still opens on Cmd/Shift click.
  if ((e.metaKey || e.ctrlKey) && handlers.onToggleSelect) {
    e.preventDefault();
    handlers.onToggleSelect(taskId);
    return;
  }
  if (e.shiftKey && handlers.onRangeSelect) {
    e.preventDefault();
    handlers.onRangeSelect(taskId);
    return;
  }
  if (handlers.isMultiSelectMode && handlers.onToggleSelect) {
    handlers.onToggleSelect(taskId);
    return;
  }
  handlers.onClick?.(task);
}

function useActiveWorkspaceRepositories() {
  const activeWorkspaceId = useAppStore((state) => state.workspaces.activeId);
  return useAppStore((state) =>
    activeWorkspaceId
      ? (state.repositories.itemsByWorkspaceId[activeWorkspaceId] ?? EMPTY_REPOSITORIES)
      : EMPTY_REPOSITORIES,
  );
}

function KanbanCardFrame({
  task,
  repositoryChips,
  draggable,
  menu,
  isPreviewed,
  isSelected,
  isMultiSelectMode,
  showMaximizeButton,
  isDeleting,
  isArchiving,
  onClick,
  onToggleSelect,
  onOpenFullPage,
}: Pick<
  KanbanCardProps,
  | "task"
  | "repositoryChips"
  | "isSelected"
  | "isMultiSelectMode"
  | "showMaximizeButton"
  | "isDeleting"
  | "isArchiving"
  | "onToggleSelect"
  | "onOpenFullPage"
> & {
  draggable: ReturnType<typeof useDraggable>;
  menu: KanbanCardMenuState;
  isPreviewed: boolean;
  onClick: (e: React.MouseEvent) => void;
}) {
  return (
    <KanbanCardContextMenu entries={menu.contextMenuEntries}>
      <KanbanCardShell
        task={task}
        repositoryChips={repositoryChips}
        attributes={draggable.attributes}
        listeners={draggable.listeners}
        setNodeRef={draggable.setNodeRef}
        transform={draggable.transform}
        isDragging={draggable.isDragging}
        isPreviewed={isPreviewed}
        isSelected={isSelected}
        isMultiSelectMode={isMultiSelectMode}
        showMaximizeButton={showMaximizeButton}
        isDeleting={isDeleting}
        isArchiving={isArchiving}
        menuEntries={menu.dropdownMenuEntries}
        onClick={onClick}
        onCheckboxClick={(e) => {
          e.stopPropagation();
          onToggleSelect?.(task.id);
        }}
        onOpenFullPage={onOpenFullPage}
      />
    </KanbanCardContextMenu>
  );
}

export function KanbanCard({
  task,
  workspaceId,
  externalLinkAvailability,
  repositoryChips,
  onClick,
  onEdit,
  onDelete,
  onArchive,
  onOpenFullPage,
  onMove,
  steps,
  showMaximizeButton = false,
  isDeleting,
  isArchiving,
  isSelected,
  selectedIds,
  onToggleSelect,
  onRangeSelect,
  isMultiSelectMode,
}: KanbanCardProps) {
  const draggable = useDraggable({
    id: task.id,
    disabled: isMultiSelectMode,
  });
  const isPreviewed = useAppStore((state) => state.kanbanPreviewedTaskId === task.id);
  const repositories = useActiveWorkspaceRepositories();
  const menu = useKanbanCardMenus({
    task,
    externalLinkAvailability,
    steps,
    isDeleting,
    isArchiving,
    isSelected,
    selectedIds,
    onEdit,
    onDelete,
    onArchive,
    onMove,
  });

  const handleClick = (e: React.MouseEvent) =>
    dispatchKanbanCardClick(e, task.id, task, {
      onToggleSelect,
      onRangeSelect,
      onClick,
      isMultiSelectMode,
    });

  return (
    <>
      <KanbanCardFrame
        task={task}
        repositoryChips={repositoryChips}
        draggable={draggable}
        menu={menu}
        isPreviewed={isPreviewed}
        isSelected={isSelected}
        isMultiSelectMode={isMultiSelectMode}
        showMaximizeButton={showMaximizeButton}
        isDeleting={isDeleting}
        isArchiving={isArchiving}
        onClick={handleClick}
        onToggleSelect={onToggleSelect}
        onOpenFullPage={onOpenFullPage}
      />
      <KanbanCardDialogs
        task={task}
        workspaceId={workspaceId}
        repositories={repositories}
        menu={menu}
        isDeleting={isDeleting}
        isArchiving={isArchiving}
        onDelete={onDelete}
        onArchive={onArchive}
      />
    </>
  );
}

/**
 * Resolves a task's linked repositories to card chip data. Primary first
 * (`task.repositoryId`), then any others ordered by `task.repositories[].position`.
 * Skips unresolved IDs (repo deleted / not yet hydrated).
 */
export function resolveTaskRepositoryChips(
  task: Task,
  repositories: Repository[],
): RepositoryChip[] {
  const byId = new Map(repositories.map((repo) => [repo.id, repo]));
  const seen = new Set<string>();
  const chips: RepositoryChip[] = [];

  const push = (id: string | undefined) => {
    if (!id || seen.has(id)) return;
    const repo = byId.get(toRepositoryId(id));
    if (!repo) return;
    seen.add(id);
    const label = repositorySlug(repo);
    if (!label) return;
    chips.push({
      label,
      ...(repo.local_path ? { path: formatUserHomePath(repo.local_path) } : {}),
    });
  };

  push(task.repositoryId);
  const ordered = [...(task.repositories ?? [])].sort((a, b) => a.position - b.position);
  for (const link of ordered) push(link.repository_id);
  return chips;
}

export function resolveTaskRepositoryNames(task: Task, repositories: Repository[]): string[] {
  return resolveTaskRepositoryChips(task, repositories).map((chip) => chip.label);
}
