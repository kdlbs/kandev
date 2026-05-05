"use client";

import { useMemo, useState } from "react";
import { IconArchive, IconDots, IconTrash } from "@tabler/icons-react";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@kandev/ui/dropdown-menu";
import { Checkbox } from "@kandev/ui/checkbox";
import { cn } from "@kandev/ui/lib/utils";
import { TaskDeleteConfirmDialog } from "@/components/task/task-delete-confirm-dialog";
import { TaskArchiveConfirmDialog } from "@/components/task/task-archive-confirm-dialog";
import { needsAction } from "@/lib/utils/needs-action";
import { useAppStore } from "@/components/state-provider";
import { Graph2StepNode } from "./graph2-step-node";
import { Graph2Connector } from "./graph2-connector";
import type { Task } from "@/components/kanban-card";
import type { WorkflowStep } from "@/components/kanban-column";

type ConnectorType = "past" | "transition" | "future";

function formatRelativeTime(dateStr: string): string {
  const now = Date.now();
  const then = new Date(dateStr).getTime();
  const diff = now - then;
  const seconds = Math.floor(diff / 1000);
  if (seconds < 60) return "just now";
  const minutes = Math.floor(seconds / 60);
  if (minutes < 60) return `${minutes}m ago`;
  const hours = Math.floor(minutes / 60);
  if (hours < 24) return `${hours}h ago`;
  const days = Math.floor(hours / 24);
  if (days < 30) return `${days}d ago`;
  const months = Math.floor(days / 30);
  return `${months}mo ago`;
}

export type Graph2TaskPipelineProps = {
  task: Task;
  steps: WorkflowStep[];
  onMoveTask: (task: Task, targetStepId: string) => void;
  onPreviewTask: (task: Task) => void;
  onOpenTask: (task: Task) => void;
  onDeleteTask: (task: Task) => void;
  onArchiveTask?: (task: Task) => void;
  isMoving?: boolean;
  isDeleting?: boolean;
  isArchiving?: boolean;
  isSelected?: boolean;
  onToggleSelect?: (taskId: string) => void;
  isMultiSelectMode?: boolean;
};

function getStepPhase(index: number, currentStepIndex: number): "past" | "current" | "future" {
  if (index < currentStepIndex) return "past";
  if (index === currentStepIndex) return "current";
  return "future";
}

function getConnectorType(
  phase: "past" | "current" | "future",
  nextPhase: "past" | "current" | "future",
): ConnectorType {
  if (phase === "past" && nextPhase === "past") return "past";
  if (phase === "future" && nextPhase === "future") return "future";
  return "transition";
}

function PipelineStepNodes({
  steps,
  currentStepIndex,
  task,
  onMoveTask,
  onPreviewTask,
  isMoving,
}: {
  steps: WorkflowStep[];
  currentStepIndex: number;
  task: Task;
  onMoveTask: (task: Task, targetStepId: string) => void;
  onPreviewTask: (task: Task) => void;
  isMoving?: boolean;
}) {
  return (
    <div className="flex items-center gap-0">
      {steps.map((step, index) => {
        const phase = getStepPhase(index, currentStepIndex);
        const hasConnector = index < steps.length - 1;
        const connectorType = hasConnector
          ? getConnectorType(phase, getStepPhase(index + 1, currentStepIndex))
          : null;

        return (
          <div key={step.id} className="flex items-center">
            <Graph2StepNode
              step={step}
              phase={phase}
              task={task}
              hasPrev={index > 0}
              hasNext={index < steps.length - 1}
              prevStepId={index > 0 ? steps[index - 1].id : undefined}
              nextStepId={index < steps.length - 1 ? steps[index + 1].id : undefined}
              onMoveTask={onMoveTask}
              onPreviewTask={onPreviewTask}
              isMoving={isMoving}
            />

            {connectorType && <Graph2Connector type={connectorType} />}
          </div>
        );
      })}
    </div>
  );
}

function TaskActions({
  task,
  onDeleteTask,
  onArchiveTask,
  isDeleting,
  isArchiving,
}: Pick<
  Graph2TaskPipelineProps,
  "task" | "onDeleteTask" | "onArchiveTask" | "isDeleting" | "isArchiving"
>) {
  const [showDeleteConfirm, setShowDeleteConfirm] = useState(false);
  const [showArchiveConfirm, setShowArchiveConfirm] = useState(false);

  return (
    <>
      <DropdownMenu>
        <DropdownMenuTrigger asChild>
          <button
            type="button"
            className="shrink-0 h-7 w-7 flex items-center justify-center rounded-md text-muted-foreground/40 hover:text-foreground hover:bg-accent/60 transition-colors cursor-pointer"
          >
            <IconDots className="h-3.5 w-3.5" />
          </button>
        </DropdownMenuTrigger>
        <DropdownMenuContent align="end" className="w-[160px]">
          {onArchiveTask && (
            <DropdownMenuItem
              onClick={() => setShowArchiveConfirm(true)}
              disabled={isArchiving}
              className="cursor-pointer"
            >
              <IconArchive className="h-3.5 w-3.5 mr-2" />
              Archive task
            </DropdownMenuItem>
          )}
          <DropdownMenuItem
            onClick={() => setShowDeleteConfirm(true)}
            disabled={isDeleting}
            className="text-destructive focus:text-destructive cursor-pointer"
          >
            <IconTrash className="h-3.5 w-3.5 mr-2" />
            Delete task
          </DropdownMenuItem>
        </DropdownMenuContent>
      </DropdownMenu>
      <TaskDeleteConfirmDialog
        open={showDeleteConfirm}
        onOpenChange={setShowDeleteConfirm}
        taskTitle={task.title}
        isDeleting={isDeleting}
        onConfirm={() => onDeleteTask(task)}
      />
      <TaskArchiveConfirmDialog
        open={showArchiveConfirm}
        onOpenChange={setShowArchiveConfirm}
        taskTitle={task.title}
        isArchiving={isArchiving}
        onConfirm={() => onArchiveTask?.(task)}
      />
    </>
  );
}

function TaskButton({
  task,
  repoName,
  isSelected,
  onClick,
}: {
  task: Task;
  repoName: string | undefined;
  isSelected?: boolean;
  onClick: () => void;
}) {
  const hasAction = needsAction(task);
  const sessionCount = task.sessionCount ?? 0;
  return (
    <button
      type="button"
      onClick={onClick}
      className={cn(
        "w-[200px] shrink-0 rounded-md px-2.5 py-1.5 text-left transition-colors cursor-pointer",
        "hover:bg-accent/60 active:bg-accent/80",
        "border border-transparent hover:border-border/50",
        hasAction && !isSelected && "border-l-2 !border-l-amber-500",
        isSelected && "ring-1 ring-primary/60 border-primary/60",
      )}
    >
      <span className="text-xs font-medium truncate block text-foreground/80">{task.title}</span>
      {repoName && (
        <span
          data-testid={`pipeline-task-repo-${task.id}`}
          className="text-xs text-muted-foreground/60 truncate block"
        >
          {repoName}
        </span>
      )}
      <div className="flex items-center gap-1.5 mt-0.5">
        {task.updatedAt && (
          <span className="text-[10px] text-muted-foreground/60">
            {formatRelativeTime(task.updatedAt)}
          </span>
        )}
        {sessionCount > 0 && (
          <span className="text-[10px] text-muted-foreground/60">
            {sessionCount} {sessionCount === 1 ? "session" : "sessions"}
          </span>
        )}
      </div>
    </button>
  );
}

function useTaskRepoName(task: Task): string | undefined {
  const repositoriesByWorkspace = useAppStore((state) => state.repositories.itemsByWorkspaceId);
  return useMemo(() => {
    const primaryRepoId = task.repositories?.[0]?.repository_id;
    if (!primaryRepoId) return undefined;
    for (const repos of Object.values(repositoriesByWorkspace)) {
      const repo = repos.find((r) => r.id === primaryRepoId);
      if (repo) return repo.name;
    }
    return undefined;
  }, [repositoriesByWorkspace, task.repositories]);
}

export function Graph2TaskPipeline({
  task,
  steps,
  onMoveTask,
  onPreviewTask,
  onOpenTask,
  onDeleteTask,
  onArchiveTask,
  isMoving,
  isDeleting,
  isArchiving,
  isSelected,
  onToggleSelect,
  isMultiSelectMode,
}: Graph2TaskPipelineProps) {
  const currentStepIndex = useMemo(
    () => steps.findIndex((s) => s.id === task.workflowStepId),
    [steps, task.workflowStepId],
  );
  const repoName = useTaskRepoName(task);
  const showCheckbox = isMultiSelectMode || !!isSelected;

  const handleTaskClick = () => {
    if (isMultiSelectMode || isSelected) {
      onToggleSelect?.(task.id);
      return;
    }
    onOpenTask(task);
  };

  const handleCheckboxClick = (e: React.MouseEvent) => {
    e.stopPropagation();
    onToggleSelect?.(task.id);
  };

  return (
    <div
      data-testid={`pipeline-task-${task.id}`}
      className="flex items-center justify-center rounded-lg hover:bg-muted/30 transition-colors px-3 py-2"
    >
      <div className="flex items-center gap-3">
        {showCheckbox && (
          <div
            className="shrink-0"
            onClick={handleCheckboxClick}
            data-testid={`task-select-checkbox-${task.id}`}
          >
            <Checkbox
              checked={!!isSelected}
              aria-label={`Select task ${task.title}`}
              className="cursor-pointer border-muted-foreground/50"
            />
          </div>
        )}
        <TaskButton
          task={task}
          repoName={repoName}
          isSelected={isSelected}
          onClick={handleTaskClick}
        />
        <PipelineStepNodes
          steps={steps}
          currentStepIndex={currentStepIndex}
          task={task}
          onMoveTask={onMoveTask}
          onPreviewTask={onPreviewTask}
          isMoving={isMoving}
        />
        {!isMultiSelectMode && (
          <TaskActions
            task={task}
            onDeleteTask={onDeleteTask}
            onArchiveTask={onArchiveTask}
            isDeleting={isDeleting}
            isArchiving={isArchiving}
          />
        )}
      </div>
    </div>
  );
}
