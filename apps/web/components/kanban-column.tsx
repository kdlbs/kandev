"use client";

import { useMemo, useState } from "react";
import { useDroppable } from "@dnd-kit/core";
import { IconDots } from "@tabler/icons-react";
import { KanbanCard, Task } from "./kanban-card";
import { Badge } from "@kandev/ui/badge";
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@kandev/ui/alert-dialog";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuPortal,
  DropdownMenuSeparator,
  DropdownMenuSub,
  DropdownMenuSubContent,
  DropdownMenuSubTrigger,
  DropdownMenuTrigger,
} from "@kandev/ui/dropdown-menu";
import { cn, getRepositoryDisplayName } from "@/lib/utils";
import { useAppStore } from "@/components/state-provider";
import type { Repository } from "@/lib/types/http";

export interface WorkflowStep {
  id: string;
  title: string;
  color: string;
  events?: {
    on_enter?: Array<{ type: string; config?: Record<string, unknown> }>;
    on_turn_complete?: Array<{ type: string; config?: Record<string, unknown> }>;
  };
}

interface KanbanColumnProps {
  step: WorkflowStep;
  tasks: Task[];
  onPreviewTask: (task: Task) => void;
  onOpenTask: (task: Task) => void;
  onEditTask: (task: Task) => void;
  onDeleteTask: (task: Task) => void;
  onArchiveTask?: (task: Task) => void;
  onMoveTask?: (task: Task, targetStepId: string) => void;
  onClearLane?: (tasks: Task[]) => Promise<void>;
  onArchiveLane?: (tasks: Task[]) => Promise<void>;
  onMoveLane?: (tasks: Task[], targetStepId: string) => Promise<void>;
  steps?: WorkflowStep[];
  showMaximizeButton?: boolean;
  deletingTaskId?: string | null;
  archivingTaskId?: string | null;
  hideHeader?: boolean;
}

export function KanbanColumn({
  step,
  tasks,
  onPreviewTask,
  onOpenTask,
  onEditTask,
  onDeleteTask,
  onArchiveTask,
  onMoveTask,
  onClearLane,
  onArchiveLane,
  onMoveLane,
  steps,
  showMaximizeButton,
  deletingTaskId,
  archivingTaskId,
  hideHeader = false,
}: KanbanColumnProps) {
  const { setNodeRef, isOver } = useDroppable({
    id: step.id,
  });

  // Access repositories from store to pass repository names to cards
  const repositoriesByWorkspace = useAppStore((state) => state.repositories.itemsByWorkspaceId);
  const repositories = useMemo(
    () => Object.values(repositoriesByWorkspace).flat() as Repository[],
    [repositoriesByWorkspace],
  );

  // Helper function to get repository name for a task
  const getRepositoryName = (repositoryId?: string): string | null => {
    if (!repositoryId) return null;
    const repository = repositories.find((repo) => repo.id === repositoryId);
    return repository ? getRepositoryDisplayName(repository.local_path) : null;
  };

  return (
    <div
      ref={setNodeRef}
      data-testid={`kanban-column-${step.id}`}
      className={cn(
        "flex flex-col flex-1 h-full min-w-0 px-3 py-2 sm:min-h-[200px]",
        "border-r border-dashed border-border/50 last:border-r-0",
        isOver && "bg-primary/5",
      )}
    >
      {/* Column Header */}
      {!hideHeader && (
        <div className="group flex items-center justify-between pb-2 mb-3 px-1">
          <div className="flex items-center gap-2">
            <div className={cn("w-2 h-2 rounded-full", step.color)} />
            <h2 className="font-semibold text-sm">{step.title}</h2>
            <Badge variant="secondary" className="text-xs">
              {tasks.length}
            </Badge>
          </div>
          {(onClearLane || onArchiveLane || onMoveLane) && tasks.length > 0 && (
            <ColumnMenu
              tasks={tasks}
              stepTitle={step.title}
              currentStepId={step.id}
              steps={steps}
              onClearLane={onClearLane}
              onArchiveLane={onArchiveLane}
              onMoveLane={onMoveLane}
            />
          )}
        </div>
      )}

      {/* Tasks */}
      <div className="flex-1 min-h-0 overflow-y-auto overflow-x-hidden space-y-2 px-1">
        {tasks.map((task) => (
          <KanbanCard
            key={task.id}
            task={task}
            repositoryName={getRepositoryName(task.repositoryId)}
            onClick={onPreviewTask}
            onOpenFullPage={onOpenTask}
            onEdit={onEditTask}
            onDelete={onDeleteTask}
            onArchive={onArchiveTask}
            onMove={onMoveTask}
            steps={steps}
            showMaximizeButton={showMaximizeButton}
            isDeleting={deletingTaskId === task.id}
            isArchiving={archivingTaskId === task.id}
          />
        ))}
      </div>
    </div>
  );
}

function MoveAllSubmenu({
  tasks,
  currentStepId,
  steps,
  onMoveLane,
}: {
  tasks: Task[];
  currentStepId?: string;
  steps: WorkflowStep[];
  onMoveLane: (tasks: Task[], targetStepId: string) => Promise<void>;
}) {
  const otherSteps = steps.filter((s) => s.id !== currentStepId);
  if (otherSteps.length === 0) return null;
  return (
    <DropdownMenuSub>
      <DropdownMenuSubTrigger
        data-testid="lane-menu-move-all"
        className="cursor-pointer"
        onClick={(e) => e.stopPropagation()}
        onPointerDown={(e) => e.stopPropagation()}
      >
        Move all to
      </DropdownMenuSubTrigger>
      <DropdownMenuPortal>
        <DropdownMenuSubContent>
          {otherSteps.map((s) => (
            <DropdownMenuItem
              key={s.id}
              data-testid={`lane-menu-move-to-${s.id}`}
              className="cursor-pointer"
              onSelect={() => onMoveLane(tasks, s.id)}
            >
              <div className={cn("w-2 h-2 rounded-full mr-2", s.color)} />
              {s.title}
            </DropdownMenuItem>
          ))}
        </DropdownMenuSubContent>
      </DropdownMenuPortal>
    </DropdownMenuSub>
  );
}

function ArchiveConfirmDialog({
  open,
  onOpenChange,
  isArchiving,
  onConfirm,
  tasks,
  stepTitle,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  isArchiving: boolean;
  onConfirm: () => void;
  tasks: Task[];
  stepTitle: string;
}) {
  const taskWord = tasks.length !== 1 ? "tasks" : "task";
  return (
    <AlertDialog open={open} onOpenChange={onOpenChange}>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>Archive all</AlertDialogTitle>
          <AlertDialogDescription>
            Archive all {tasks.length} {taskWord} in &quot;{stepTitle}&quot;?
          </AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel className="cursor-pointer">Cancel</AlertDialogCancel>
          <AlertDialogAction
            data-testid="lane-confirm-archive"
            disabled={isArchiving}
            className="cursor-pointer"
            onClick={onConfirm}
          >
            Archive all
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  );
}

function ClearConfirmDialog({
  open,
  onOpenChange,
  isClearing,
  onConfirm,
  tasks,
  stepTitle,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  isClearing: boolean;
  onConfirm: () => void;
  tasks: Task[];
  stepTitle: string;
}) {
  const taskWord = tasks.length !== 1 ? "tasks" : "task";
  return (
    <AlertDialog open={open} onOpenChange={onOpenChange}>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>Clear lane</AlertDialogTitle>
          <AlertDialogDescription>
            Delete all {tasks.length} {taskWord} in &quot;{stepTitle}&quot;? This action cannot be
            undone.
          </AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel className="cursor-pointer">Cancel</AlertDialogCancel>
          <AlertDialogAction
            data-testid="lane-confirm-clear"
            disabled={isClearing}
            className="cursor-pointer bg-destructive text-destructive-foreground hover:bg-destructive/90"
            onClick={onConfirm}
          >
            Delete all
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  );
}

function ColumnMenu({
  tasks,
  stepTitle,
  currentStepId,
  steps,
  onClearLane,
  onArchiveLane,
  onMoveLane,
}: {
  tasks: Task[];
  stepTitle: string;
  currentStepId?: string;
  steps?: WorkflowStep[];
  onClearLane?: (tasks: Task[]) => Promise<void>;
  onArchiveLane?: (tasks: Task[]) => Promise<void>;
  onMoveLane?: (tasks: Task[], targetStepId: string) => Promise<void>;
}) {
  const [menuOpen, setMenuOpen] = useState(false);
  const [clearConfirmOpen, setClearConfirmOpen] = useState(false);
  const [archiveConfirmOpen, setArchiveConfirmOpen] = useState(false);
  const [isClearing, setIsClearing] = useState(false);
  const [isArchiving, setIsArchiving] = useState(false);

  const handleClearConfirm = async () => {
    if (!onClearLane) return;
    setIsClearing(true);
    try {
      await onClearLane(tasks);
    } finally {
      setIsClearing(false);
      setClearConfirmOpen(false);
    }
  };

  const handleArchiveConfirm = async () => {
    if (!onArchiveLane) return;
    setIsArchiving(true);
    try {
      await onArchiveLane(tasks);
    } finally {
      setIsArchiving(false);
      setArchiveConfirmOpen(false);
    }
  };

  return (
    <>
      <DropdownMenu open={menuOpen} onOpenChange={setMenuOpen}>
        <DropdownMenuTrigger asChild>
          <button
            type="button"
            data-testid={`lane-menu-trigger-${currentStepId}`}
            className={cn(
              "transition-opacity text-muted-foreground hover:text-foreground hover:bg-muted rounded-sm p-1 -m-1 cursor-pointer",
              menuOpen ? "opacity-100" : "opacity-0 group-hover:opacity-100",
            )}
            onClick={(e) => e.stopPropagation()}
            onPointerDown={(e) => e.stopPropagation()}
            aria-label="Lane options"
          >
            <IconDots className="h-4 w-4" />
          </button>
        </DropdownMenuTrigger>
        <DropdownMenuContent align="end">
          {onMoveLane && steps && (
            <MoveAllSubmenu
              tasks={tasks}
              currentStepId={currentStepId}
              steps={steps}
              onMoveLane={onMoveLane}
            />
          )}
          {onMoveLane && onArchiveLane && <DropdownMenuSeparator />}
          {onArchiveLane && (
            <DropdownMenuItem
              data-testid="lane-menu-archive-all"
              className="cursor-pointer"
              onSelect={() => setArchiveConfirmOpen(true)}
            >
              Archive all
            </DropdownMenuItem>
          )}
          {onArchiveLane && onClearLane && <DropdownMenuSeparator />}
          {onClearLane && (
            <DropdownMenuItem
              data-testid="lane-menu-clear"
              className="text-destructive focus:text-destructive cursor-pointer"
              onSelect={() => setClearConfirmOpen(true)}
            >
              Clear lane
            </DropdownMenuItem>
          )}
        </DropdownMenuContent>
      </DropdownMenu>

      {onArchiveLane && (
        <ArchiveConfirmDialog
          open={archiveConfirmOpen}
          onOpenChange={setArchiveConfirmOpen}
          isArchiving={isArchiving}
          onConfirm={handleArchiveConfirm}
          tasks={tasks}
          stepTitle={stepTitle}
        />
      )}

      {onClearLane && (
        <ClearConfirmDialog
          open={clearConfirmOpen}
          onOpenChange={setClearConfirmOpen}
          isClearing={isClearing}
          onConfirm={handleClearConfirm}
          tasks={tasks}
          stepTitle={stepTitle}
        />
      )}
    </>
  );
}
