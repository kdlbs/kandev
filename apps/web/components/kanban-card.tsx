"use client";

import { useMemo, useState } from "react";
import { useDraggable } from "@dnd-kit/core";
import { CSS } from "@dnd-kit/utilities";
import {
  IconDots,
  IconArrowsMaximize,
  IconLoader,
  IconAlertCircle,
  IconSubtask,
} from "@tabler/icons-react";
import { Checkbox } from "@kandev/ui/checkbox";
import { Card, CardContent } from "@kandev/ui/card";
import { Badge } from "@kandev/ui/badge";
import { TaskDeleteConfirmDialog } from "@/components/task/task-delete-confirm-dialog";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
  DropdownMenuSub,
  DropdownMenuSubTrigger,
  DropdownMenuSubContent,
  DropdownMenuPortal,
  DropdownMenuSeparator,
} from "@kandev/ui/dropdown-menu";
import type { TaskState, Repository } from "@/lib/types/http";
import { cn, getRepositoryDisplayName } from "@/lib/utils";
import { getTaskStateIcon } from "@/lib/ui/state-icons";
import { needsAction } from "@/lib/utils/needs-action";
import { useAppStore } from "@/components/state-provider";
import { PRTaskIcon } from "@/components/github/pr-task-icon";
import { RemoteCloudTooltip } from "@/components/task/remote-cloud-tooltip";
import { TaskArchiveConfirmDialog } from "@/components/task/task-archive-confirm-dialog";

export interface Task {
  id: string;
  title: string;
  workflowStepId: string;
  state?: TaskState;
  description?: string;
  position?: number;
  repositoryId?: string;
  // Workflow fields
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
}

interface KanbanCardProps {
  task: Task;
  repositoryName?: string | null;
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
  onToggleSelect?: (taskId: string) => void;
  isMultiSelectMode?: boolean;
}

function KanbanCardBody({
  task,
  repoName,
  actions,
}: {
  task: Task;
  repoName: string | null;
  actions?: React.ReactNode;
}) {
  return (
    <>
      <div className="flex items-start justify-between gap-2">
        <div className="min-w-0 flex-1">
          {repoName && (
            <p className="text-[10px] mb-1 text-muted-foreground leading-tight truncate">
              {repoName}
            </p>
          )}
          <div className="flex items-center gap-1 min-w-0">
            <p
              data-testid="task-card-title"
              className="text-sm font-medium leading-tight line-clamp-1 min-w-0"
            >
              {task.title}
            </p>
            <PRTaskIcon taskId={task.id} />
          </div>
        </div>
        {task.isRemoteExecutor && (
          <RemoteCloudTooltip
            taskId={task.id}
            sessionId={task.primarySessionId ?? null}
            fallbackName={task.primaryExecutorName ?? task.primaryExecutorType}
          />
        )}
        {actions}
      </div>
      {task.description && (
        <p className="text-xs text-muted-foreground mt-1 leading-tight line-clamp-1">
          {task.description}
        </p>
      )}
      <KanbanCardBadges task={task} />
    </>
  );
}

function KanbanCardBadges({ task }: { task: Task }) {
  const parentTitle = useAppStore((s) => {
    if (!task.parentTaskId) return null;
    return s.kanban.tasks.find((t) => t.id === task.parentTaskId)?.title ?? null;
  });

  const showRow =
    (task.sessionCount && task.sessionCount > 1) ||
    task.reviewStatus === "changes_requested" ||
    task.reviewStatus === "pending" ||
    task.parentTaskId;

  if (!showRow) return null;

  return (
    <div className="flex flex-wrap items-center justify-end gap-2 mt-1 min-w-0">
      {task.parentTaskId && (
        <Badge variant="outline" className="text-xs h-5 gap-1 max-w-[160px] min-w-0">
          <IconSubtask className="h-3 w-3 shrink-0" />
          <span className="truncate">{parentTitle ?? "Subtask"}</span>
        </Badge>
      )}
      {task.sessionCount && task.sessionCount > 1 && (
        <Badge variant="secondary" className="text-xs h-5">
          {task.sessionCount} sessions
        </Badge>
      )}
      {task.reviewStatus === "pending" && task.state !== "IN_PROGRESS" && (
        <div className="flex items-center gap-1 text-amber-700 dark:text-amber-600">
          <IconAlertCircle className="h-3.5 w-3.5" />
          <span className="text-[10px] font-medium">Approval Required</span>
        </div>
      )}
      {task.reviewStatus === "changes_requested" && (
        <Badge
          variant="outline"
          className="border-amber-500 text-amber-600 bg-amber-50 dark:bg-amber-950/50 text-xs h-5"
        >
          Changes Requested
        </Badge>
      )}
    </div>
  );
}

function KanbanCardLayout({
  task,
  repositoryName,
  className,
}: KanbanCardProps & { className?: string }) {
  return (
    <Card size="sm" className={cn("w-full py-0", className)}>
      <CardContent className="px-2 py-1">
        <KanbanCardBody task={task} repoName={repositoryName ?? null} />
      </CardContent>
    </Card>
  );
}

function KanbanCardActions({
  task,
  showMaximizeButton,
  onOpenFullPage,
  onEdit,
  onDelete,
  onArchive,
  onMove,
  steps,
  isDeleting,
  isArchiving,
}: Pick<
  KanbanCardProps,
  | "task"
  | "showMaximizeButton"
  | "onOpenFullPage"
  | "onEdit"
  | "onDelete"
  | "onArchive"
  | "onMove"
  | "steps"
  | "isDeleting"
  | "isArchiving"
>) {
  const [menuOpen, setMenuOpen] = useState(false);
  const effectiveMenuOpen = menuOpen || Boolean(isDeleting) || Boolean(isArchiving);
  const statusIcon = getTaskStateIcon(task.state, "h-4 w-4");
  const hasKnownSession =
    Boolean(task.primarySessionId) || Boolean(task.sessionCount && task.sessionCount > 0);

  return (
    <div className="flex items-center gap-2">
      {(task.state === "IN_PROGRESS" || task.state === "SCHEDULING") && statusIcon}
      {showMaximizeButton && onOpenFullPage && hasKnownSession && (
        <button
          type="button"
          className="text-muted-foreground hover:text-foreground hover:bg-accent rounded-sm p-1 -m-1 transition-colors cursor-pointer"
          onClick={(event) => {
            event.stopPropagation();
            onOpenFullPage(task);
          }}
          onPointerDown={(event) => event.stopPropagation()}
          aria-label="Open full page"
          title="Open full page"
        >
          <IconArrowsMaximize className="h-4 w-4" />
        </button>
      )}
      <KanbanCardMenu
        task={task}
        effectiveMenuOpen={effectiveMenuOpen}
        setMenuOpen={setMenuOpen}
        isDeleting={isDeleting}
        isArchiving={isArchiving}
        onEdit={onEdit}
        onDelete={onDelete}
        onArchive={onArchive}
        onMove={onMove}
        steps={steps}
      />
    </div>
  );
}

function MoveToSubmenu({
  task,
  steps,
  disabled,
  onMove,
}: {
  task: Task;
  steps: WorkflowStep[];
  disabled?: boolean;
  onMove: (task: Task, targetStepId: string) => void;
}) {
  return (
    <DropdownMenuSub>
      <DropdownMenuSubTrigger
        disabled={disabled}
        onClick={(event) => event.stopPropagation()}
        onPointerDown={(event) => event.stopPropagation()}
      >
        Move to
      </DropdownMenuSubTrigger>
      <DropdownMenuPortal>
        <DropdownMenuSubContent>
          {steps
            .filter((col) => col.id !== task.workflowStepId)
            .map((col) => (
              <DropdownMenuItem
                key={col.id}
                onClick={(event) => {
                  event.stopPropagation();
                  onMove(task, col.id);
                }}
              >
                <div className={cn("w-2 h-2 rounded-full mr-2", col.color)} />
                {col.title}
              </DropdownMenuItem>
            ))}
        </DropdownMenuSubContent>
      </DropdownMenuPortal>
    </DropdownMenuSub>
  );
}

type KanbanCardMenuProps = {
  task: Task;
  effectiveMenuOpen: boolean;
  setMenuOpen: (open: boolean) => void;
  isDeleting?: boolean;
  isArchiving?: boolean;
  onEdit?: (task: Task) => void;
  onDelete?: (task: Task) => void;
  onArchive?: (task: Task) => void;
  onMove?: (task: Task, targetStepId: string) => void;
  steps?: WorkflowStep[];
};

function KanbanCardMenu(props: KanbanCardMenuProps) {
  const { task, effectiveMenuOpen, setMenuOpen, isDeleting, isArchiving } = props;
  const { onEdit, onDelete, onArchive, onMove, steps } = props;
  const [showDeleteConfirm, setShowDeleteConfirm] = useState(false);
  const [showArchiveConfirm, setShowArchiveConfirm] = useState(false);
  const isProcessing = isDeleting || isArchiving;

  return (
    <>
      <DropdownMenu
        open={effectiveMenuOpen}
        onOpenChange={(open) => {
          if (!open && isProcessing) return;
          setMenuOpen(open);
        }}
      >
        <DropdownMenuTrigger asChild>
          <button
            type="button"
            className="text-muted-foreground hover:text-foreground hover:bg-muted rounded-sm p-1 -m-1 transition-colors cursor-pointer"
            onClick={(e) => e.stopPropagation()}
            onPointerDown={(e) => e.stopPropagation()}
            aria-label="More options"
          >
            <IconDots className="h-4 w-4" />
          </button>
        </DropdownMenuTrigger>
        <DropdownMenuContent align="end">
          <DropdownMenuItem
            disabled={isProcessing}
            onClick={(e) => {
              e.stopPropagation();
              onEdit?.(task);
            }}
          >
            Edit
          </DropdownMenuItem>
          {steps && steps.length > 1 && onMove && (
            <MoveToSubmenu task={task} steps={steps} disabled={isProcessing} onMove={onMove} />
          )}
          {onArchive && (
            <DropdownMenuItem
              disabled={isProcessing}
              onClick={(e) => {
                e.stopPropagation();
                setShowArchiveConfirm(true);
              }}
            >
              {isArchiving ? <IconLoader className="mr-2 h-4 w-4 animate-spin" /> : null}
              Archive
            </DropdownMenuItem>
          )}
          <DropdownMenuSeparator />
          <DropdownMenuItem
            disabled={isProcessing}
            className="text-destructive focus:text-destructive"
            onClick={(e) => {
              e.stopPropagation();
              setShowDeleteConfirm(true);
            }}
          >
            Delete
          </DropdownMenuItem>
        </DropdownMenuContent>
      </DropdownMenu>
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
        onConfirm={() => {
          onArchive?.(task);
          setShowArchiveConfirm(false);
        }}
      />
    </>
  );
}

function KanbanCardCheckbox({
  taskId,
  taskTitle,
  isSelected,
  onCheckboxClick,
}: {
  taskId: string;
  taskTitle: string;
  isSelected?: boolean;
  onCheckboxClick: (e: React.MouseEvent) => void;
}) {
  return (
    <div
      className="mt-0.5 shrink-0"
      onClick={onCheckboxClick}
      onPointerDown={(e) => e.stopPropagation()}
      data-testid={`task-select-checkbox-${taskId}`}
    >
      <Checkbox
        checked={!!isSelected}
        aria-label={`Select task ${taskTitle}`}
        className="cursor-pointer border-muted-foreground/50"
      />
    </div>
  );
}

export function KanbanCard({
  task,
  repositoryName,
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
  onToggleSelect,
  isMultiSelectMode,
}: KanbanCardProps) {
  const { attributes, listeners, setNodeRef, transform, isDragging } = useDraggable({
    id: task.id,
    disabled: isMultiSelectMode,
  });

  const style = {
    transform: CSS.Translate.toString(transform),
    transition: "none",
    willChange: isDragging ? "transform" : undefined,
  };

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

  const showCheckbox = isMultiSelectMode || !!isSelected;

  return (
    <Card
      size="sm"
      ref={setNodeRef}
      style={style}
      data-testid={`task-card-${task.id}`}
      className={cn(
        "group max-h-48 bg-card rounded-sm data-[size=sm]:py-1 cursor-pointer mb-2 w-full py-0 relative border border-border overflow-visible shadow-none ring-0",
        needsAction(task) && !isSelected && "border-l-2 border-l-amber-500",
        isDragging && "opacity-50 z-50",
        isSelected && "ring-1 ring-primary/60 border-primary/60",
      )}
      onClick={handleClick}
      {...(!isMultiSelectMode ? listeners : {})}
      {...(!isMultiSelectMode ? attributes : {})}
    >
      <CardContent className="px-2 py-1">
        <div className="flex items-start gap-1.5">
          {showCheckbox && (
            <KanbanCardCheckbox
              taskId={task.id}
              taskTitle={task.title}
              isSelected={isSelected}
              onCheckboxClick={handleCheckboxClick}
            />
          )}
          <div className="min-w-0 flex-1">
            <KanbanCardBody
              task={task}
              repoName={repositoryName ?? null}
              actions={
                !isMultiSelectMode ? (
                  <KanbanCardActions
                    task={task}
                    showMaximizeButton={showMaximizeButton}
                    onOpenFullPage={onOpenFullPage}
                    onEdit={onEdit}
                    onDelete={onDelete}
                    onArchive={onArchive}
                    onMove={onMove}
                    steps={steps}
                    isDeleting={isDeleting}
                    isArchiving={isArchiving}
                  />
                ) : undefined
              }
            />
          </div>
        </div>
      </CardContent>
    </Card>
  );
}

export function KanbanCardPreview({ task }: KanbanCardProps) {
  // Access store to get repository name for the drag preview
  const repositoriesByWorkspace = useAppStore((state) => state.repositories.itemsByWorkspaceId);
  const repository = useMemo(
    () =>
      (Object.values(repositoriesByWorkspace).flat() as Repository[]).find(
        (repo) => repo.id === task.repositoryId,
      ) ?? null,
    [repositoriesByWorkspace, task.repositoryId],
  );
  const repositoryName = repository ? getRepositoryDisplayName(repository.local_path) : null;

  return (
    <KanbanCardLayout
      task={task}
      repositoryName={repositoryName}
      className="cursor-grabbing shadow-lg ring-0 pointer-events-none border border-border"
    />
  );
}
