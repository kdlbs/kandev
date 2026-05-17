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
import { useTaskWorkflowMove } from "@/hooks/use-task-workflow-move";
import { getRepositoryDisplayName } from "@/lib/utils";
import { repositoryId as toRepositoryId, type Repository, type TaskState } from "@/lib/types/http";

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
  reviewStatus?: "pending" | "approved" | "changes_requested" | "rejected" | null;
  primaryExecutorId?: string | null;
  primaryExecutorType?: string | null;
  primaryExecutorName?: string | null;
  isRemoteExecutor?: boolean;
  parentTaskId?: string | null;
  updatedAt?: string;
  createdAt?: string;
}

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
  /** Display names of every repository linked to the task, primary first. */
  repositoryNames?: string[];
  onClick?: (task: Task) => void;
  onEdit?: (task: Task) => void;
  onDelete?: (task: Task) => void;
  onArchive?: (task: Task) => void;
  onOpenFullPage?: (task: Task) => void;
  onMove?: (task: Task, targetStepId: string) => void;
  steps?: WorkflowStep[];
  showMaximizeButton?: boolean;
  isDeleting?: boolean;
  isArchiving?: boolean;
  isSelected?: boolean;
  selectedIds?: Set<string>;
  onToggleSelect?: (taskId: string) => void;
  isMultiSelectMode?: boolean;
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
}: Pick<
  KanbanCardProps,
  | "task"
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
  const moveTargets = useKanbanCardMoveTargets(task.id, steps);
  const moveTasks = useTaskWorkflowMove();
  const [showDeleteConfirm, setShowDeleteConfirm] = useState(false);
  const [showArchiveConfirm, setShowArchiveConfirm] = useState(false);
  const disabled = Boolean(isDeleting || isArchiving);

  const runMoveTasks = (taskIds: string[], workflowId: string, stepId: string) => {
    void moveTasks(taskIds, workflowId, stepId).catch(() => {
      // useTaskWorkflowMove already shows the failure toast.
    });
  };

  const moveToStepFromDropdown = (stepId: string) => {
    if (onMove) {
      onMove(task, stepId);
      return;
    }
    if (moveTargets.currentWorkflowId) {
      runMoveTasks([task.id], moveTargets.currentWorkflowId, stepId);
    }
  };

  const selectedTaskIds = isSelected && selectedIds?.size ? [...selectedIds] : [task.id];
  const moveSelectedToStep = (stepId: string) => {
    if (selectedTaskIds.length === 1 && selectedTaskIds[0] === task.id && onMove) {
      onMove(task, stepId);
      return;
    }
    if (!moveTargets.currentWorkflowId) return;
    runMoveTasks(selectedTaskIds, moveTargets.currentWorkflowId, stepId);
  };

  const menuBase = {
    currentWorkflowId: moveTargets.currentWorkflowId,
    currentStepId: task.workflowStepId,
    workflows: moveTargets.workflowItems,
    stepsByWorkflowId: moveTargets.stepsByWorkflowId,
    disabled,
    isDeleting,
    isArchiving,
    onEdit: onEdit ? () => onEdit(task) : undefined,
    onArchive: onArchive ? () => setShowArchiveConfirm(true) : undefined,
    onDelete: onDelete ? () => setShowDeleteConfirm(true) : undefined,
  };

  return {
    dropdownMenuEntries: buildKanbanCardMenuEntries({
      ...menuBase,
      onMoveToStep: moveToStepFromDropdown,
      onSendToWorkflow: (workflowId, stepId) => {
        runMoveTasks([task.id], workflowId, stepId);
      },
    }),
    contextMenuEntries: buildKanbanCardMenuEntries({
      ...menuBase,
      onMoveToStep: moveSelectedToStep,
      onSendToWorkflow: (workflowId, stepId) => {
        runMoveTasks(selectedTaskIds, workflowId, stepId);
      },
    }),
    showDeleteConfirm,
    setShowDeleteConfirm,
    showArchiveConfirm,
    setShowArchiveConfirm,
  };
}

export function KanbanCard({
  task,
  repositoryNames,
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
  isMultiSelectMode,
}: KanbanCardProps) {
  const { attributes, listeners, setNodeRef, transform, isDragging } = useDraggable({
    id: task.id,
    disabled: isMultiSelectMode,
  });
  const isPreviewed = useAppStore((state) => state.kanbanPreviewedTaskId === task.id);
  const {
    dropdownMenuEntries,
    contextMenuEntries,
    showDeleteConfirm,
    setShowDeleteConfirm,
    showArchiveConfirm,
    setShowArchiveConfirm,
  } = useKanbanCardMenus({
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
  });

  const handleClick = () => {
    if (isMultiSelectMode) {
      onToggleSelect?.(task.id);
      return;
    }
    onClick?.(task);
  };

  const handleCheckboxClick = (e: React.MouseEvent) => {
    e.stopPropagation();
    onToggleSelect?.(task.id);
  };

  return (
    <>
      <KanbanCardContextMenu entries={contextMenuEntries}>
        <KanbanCardShell
          task={task}
          repositoryNames={repositoryNames}
          attributes={attributes}
          listeners={listeners}
          setNodeRef={setNodeRef}
          transform={transform}
          isDragging={isDragging}
          isPreviewed={isPreviewed}
          isSelected={isSelected}
          isMultiSelectMode={isMultiSelectMode}
          showMaximizeButton={showMaximizeButton}
          isDeleting={isDeleting}
          isArchiving={isArchiving}
          menuEntries={dropdownMenuEntries}
          onClick={handleClick}
          onCheckboxClick={handleCheckboxClick}
          onOpenFullPage={onOpenFullPage}
        />
      </KanbanCardContextMenu>
      <TaskDeleteConfirmDialog
        open={showDeleteConfirm}
        onOpenChange={setShowDeleteConfirm}
        taskTitle={task.title}
        isDeleting={isDeleting}
        onConfirm={() => onDelete?.(task)}
      />
      <TaskArchiveConfirmDialog
        open={showArchiveConfirm}
        onOpenChange={setShowArchiveConfirm}
        taskTitle={task.title}
        isArchiving={isArchiving}
        onConfirm={() => onArchive?.(task)}
      />
    </>
  );
}

/**
 * Resolves a task's linked repositories to display names. Primary first
 * (`task.repositoryId`), then any others ordered by `task.repositories[].position`.
 * Skips unresolved IDs (repo deleted / not yet hydrated).
 */
export function resolveTaskRepositoryNames(task: Task, repositories: Repository[]): string[] {
  const byId = new Map(repositories.map((repo) => [repo.id, repo]));
  const seen = new Set<string>();
  const names: string[] = [];

  const push = (id: string | undefined) => {
    if (!id || seen.has(id)) return;
    const repo = byId.get(toRepositoryId(id));
    if (!repo) return;
    seen.add(id);
    const display = getRepositoryDisplayName(repo.local_path);
    if (display) names.push(display);
  };

  push(task.repositoryId);
  const ordered = [...(task.repositories ?? [])].sort((a, b) => a.position - b.position);
  for (const link of ordered) push(link.repository_id);
  return names;
}
