"use client";

import { useDraggable } from "@dnd-kit/core";
import { KanbanCardContextMenu } from "@/components/kanban-card-context-menu";
import { KanbanCardShell } from "@/components/kanban-card-content";
import { useActiveWorkspaceRepositories } from "@/components/kanban-card-repositories";
export { resolveTaskRepositoryChips } from "@/components/kanban-card-repositories";
import {
  KanbanCardDialogs,
  useKanbanCardMenus,
  type KanbanCardMenuState,
} from "@/components/kanban-card-menu";
export { buildPluginMenuContext } from "@/components/kanban-card-menu";
import { useAppStore } from "@/components/state-provider";
import { TaskArchiveConfirmation } from "@/components/task/task-archive-confirmation";
import { TaskDetachConfirmationSurface } from "@/components/task/task-detach-confirm-dialog";
import type { KanbanExternalLinkAvailability } from "./kanban-external-link-availability";
import type { TaskDependencyRef } from "@/lib/state/slices/kanban/types";
import {
  type ForegroundActivity,
  type TaskPendingAction,
  type TaskPriority,
  type TaskState,
} from "@/lib/types/http";
import type { PluginTaskMenuContext } from "@/lib/plugins/types";

export interface Task {
  id: string;
  title: string;
  workflowStepId: string;
  state?: TaskState;
  priority?: TaskPriority;
  description?: string;
  position?: number;
  repositoryId?: string;
  /** All repositories linked to the task; used to render a "+N" chip for multi-repo. */
  repositories?: Array<{
    id: string;
    repository_id: string;
    base_branch?: string;
    checkout_branch?: string;
    branch_policy_id?: string;
    branch_policy_name?: string;
    branch_policy_base_branch?: string;
    branch_policy_branch_template?: string;
    branch_policy_pull_request_target?: string;
    position: number;
  }>;
  sessionCount?: number | null;
  primarySessionId?: string | null;
  /**
   * Primary session's runtime state. Decoupled from `state` (the workflow
   * column). Used to suppress the running-spinner when the agent has already
   * finished — the workflow may leave the task in IN_PROGRESS for review.
   */
  primarySessionState?: string | null;
  primarySessionPendingAction?: TaskPendingAction | null;
  taskPendingAction?: TaskPendingAction | null;
  /**
   * Task-level MOST-ACTIVE-WINS activity aggregate;
   * undefined/null when no session is running. Drives the background-running
   * affordance on the card status icon.
   */
  foregroundActivity?: ForegroundActivity | null;
  /** True when the task's session was mid-turn when the backend died. */
  interrupted?: boolean;
  /** True when a workflow step's auto_start_agent on_enter action failed to
   *  launch a run for this task. */
  autoStartFailed?: boolean;
  /** Live subagents summed across this task's sessions; drives the count chip. */
  activeSubagentCount?: number;
  reviewStatus?: "pending" | "approved" | "changes_requested" | "rejected" | null;
  primaryExecutorId?: string | null;
  primaryExecutorType?: string | null;
  primaryExecutorName?: string | null;
  isRemoteExecutor?: boolean;
  /** Human assignee (user id); the card renders their name read-only. */
  assigneeUserId?: string;
  parentTaskId?: string | null;
  workspaceMode?: "inherit_parent" | "new_workspace" | "shared_group";
  updatedAt?: string;
  createdAt?: string;
  wipAdmitted?: boolean;
  queuedForStepId?: string;
  queuedForStepTitle?: string;
  /** Derived dependency state — see TaskDependencyRef in the kanban slice. */
  blocked?: boolean;
  blockedReason?: string;
  dependsOn?: TaskDependencyRef[];
  blocks?: TaskDependencyRef[];
  startWhenUnblocked?: boolean;
  queuedAt?: string;
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

export type KanbanPresentation = PluginTaskMenuContext["presentation"];

interface KanbanCardProps {
  task: Task;
  workspaceId: string | null;
  presentation?: KanbanPresentation;
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
  onArchive,
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
  | "onArchive"
  | "onToggleSelect"
  | "onOpenFullPage"
> & {
  draggable: ReturnType<typeof useDraggable>;
  menu: KanbanCardMenuState;
  isPreviewed: boolean;
  onClick: (e: React.MouseEvent) => void;
}) {
  return (
    <>
      <div ref={menu.detachAnchorRef} className="w-full">
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
            menuTriggerRef={menu.detachFocusReturnRef}
            onClick={onClick}
            onCheckboxClick={(e) => {
              e.stopPropagation();
              onToggleSelect?.(task.id);
            }}
            onOpenFullPage={onOpenFullPage}
          />
        </KanbanCardContextMenu>
      </div>
      <TaskDetachConfirmationSurface
        open={menu.showDetachConfirm}
        anchorRef={menu.detachAnchorRef}
        focusReturnRef={menu.detachFocusReturnRef}
        taskTitle={task.title}
        sharesParentWorkspace={task.workspaceMode === "inherit_parent"}
        onOpenChange={menu.setShowDetachConfirm}
        onConfirm={menu.handleDetachConfirm}
      />
      <TaskArchiveConfirmation
        open={menu.showArchiveConfirm}
        anchorRef={menu.archiveAnchorRef}
        focusReturnRef={menu.archiveFocusReturnRef}
        taskTitle={task.title}
        taskId={task.id}
        executorType={task.primaryExecutorType}
        isArchiving={isArchiving}
        onOpenChange={menu.setShowArchiveConfirm}
        onConfirm={({ cascade }) => onArchive?.(task, { cascade })}
      />
    </>
  );
}

export function KanbanCard({
  task,
  workspaceId,
  presentation = "desktop",
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
    workspaceId,
    presentation,
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
        onArchive={onArchive}
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
        onDelete={onDelete}
      />
    </>
  );
}
