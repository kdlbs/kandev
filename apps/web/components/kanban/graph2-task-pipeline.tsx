"use client";

import { useMemo, useRef, useState } from "react";
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
import { TaskArchiveConfirmation } from "@/components/task/task-archive-confirmation";
import { formatRelativeTime } from "@/lib/utils";
import { needsAction } from "@/lib/utils/needs-action";
import { useAppStore } from "@/components/state-provider";
import { Graph2StepNode } from "./graph2-step-node";
import { Graph2Connector } from "./graph2-connector";
import { isOrphanMoveTarget } from "./swimlane-kanban-content";
import type { Task } from "@/components/kanban-card";
import type { WorkflowStep } from "@/components/kanban-column";
import { useTranslation } from "react-i18next";

type ConnectorType = "past" | "transition" | "future";

export type Graph2TaskPipelineProps = {
  task: Task;
  steps: WorkflowStep[];
  moveTargetSteps: WorkflowStep[];
  onMoveTask: (task: Task, targetStepId: string) => void;
  onPreviewTask: (task: Task) => void;
  onOpenTask: (task: Task) => void;
  onDeleteTask: (task: Task, opts?: { cascade?: boolean }) => void;
  onArchiveTask?: (task: Task, opts?: { cascade?: boolean }) => void;
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

export type StepAdjacency = {
  hasPrev: boolean;
  prevStepId?: string;
  hasNext: boolean;
  nextStepId?: string;
};

export type StepMoveTargets = StepAdjacency & {
  prevStepTitle?: string;
  prevStepHidden: boolean;
  nextStepTitle?: string;
  nextStepHidden: boolean;
};

/**
 * Computes the prev/next move targets for the node at `index`. The synthetic
 * "Needs Reassignment" node is display-only: it marks where an orphaned task
 * currently sits, but is never itself a valid move destination (there is no
 * backing workflow step to move into), so it is excluded as an adjacency
 * target in either direction.
 */
export function getStepAdjacency(steps: WorkflowStep[], index: number): StepAdjacency {
  const hasPrev = index > 0 && !isOrphanMoveTarget(steps[index - 1].id);
  const hasNext = index < steps.length - 1 && !isOrphanMoveTarget(steps[index + 1].id);
  return {
    hasPrev,
    prevStepId: hasPrev ? steps[index - 1].id : undefined,
    hasNext,
    nextStepId: hasNext ? steps[index + 1].id : undefined,
  };
}

export function getStepAdjacencyForStep(
  moveTargetSteps: WorkflowStep[],
  stepId: string,
): StepAdjacency {
  const index = moveTargetSteps.findIndex((step) => step.id === stepId);
  if (index < 0) return { hasPrev: false, hasNext: false };
  return getStepAdjacency(moveTargetSteps, index);
}

export function getStepMoveTargets(
  visibleSteps: WorkflowStep[],
  moveTargetSteps: WorkflowStep[],
  stepId: string,
): StepMoveTargets {
  const adjacency = getStepAdjacencyForStep(moveTargetSteps, stepId);
  const visibleStepIds = new Set(visibleSteps.map((step) => step.id));
  const prevStep = moveTargetSteps.find((step) => step.id === adjacency.prevStepId);
  const nextStep = moveTargetSteps.find((step) => step.id === adjacency.nextStepId);
  return {
    ...adjacency,
    prevStepTitle: prevStep?.title,
    prevStepHidden: !!adjacency.prevStepId && !visibleStepIds.has(adjacency.prevStepId),
    nextStepTitle: nextStep?.title,
    nextStepHidden: !!adjacency.nextStepId && !visibleStepIds.has(adjacency.nextStepId),
  };
}

function PipelineStepNodes({
  steps,
  moveTargetSteps,
  currentStepIndex,
  task,
  onMoveTask,
  onPreviewTask,
  isMoving,
}: {
  steps: WorkflowStep[];
  moveTargetSteps: WorkflowStep[];
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

        const moveTargets = getStepMoveTargets(steps, moveTargetSteps, step.id);

        return (
          <div key={step.id} className="flex items-center">
            <Graph2StepNode
              step={step}
              phase={phase}
              task={task}
              hasPrev={moveTargets.hasPrev}
              hasNext={moveTargets.hasNext}
              prevStepId={moveTargets.prevStepId}
              nextStepId={moveTargets.nextStepId}
              prevStepTitle={moveTargets.prevStepTitle}
              nextStepTitle={moveTargets.nextStepTitle}
              prevStepHidden={moveTargets.prevStepHidden}
              nextStepHidden={moveTargets.nextStepHidden}
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
  const { t } = useTranslation();
  const [showDeleteConfirm, setShowDeleteConfirm] = useState(false);
  const [showArchiveConfirm, setShowArchiveConfirm] = useState(false);
  const archiveAnchorRef = useRef<HTMLButtonElement>(null);

  return (
    <div className="flex min-w-0 flex-wrap items-center justify-end gap-1">
      <DropdownMenu>
        <DropdownMenuTrigger asChild>
          <button
            ref={archiveAnchorRef}
            type="button"
            className="shrink-0 h-7 w-7 flex items-center justify-center rounded-md text-muted-foreground/40 hover:text-foreground hover:bg-accent/60 transition-colors cursor-pointer"
          >
            <IconDots className="h-3.5 w-3.5" />
          </button>
        </DropdownMenuTrigger>
        <DropdownMenuContent align="end" className="w-[160px]">
          {onArchiveTask && (
            <DropdownMenuItem
              onClick={() => window.setTimeout(() => setShowArchiveConfirm(true), 300)}
              disabled={isArchiving}
              className="cursor-pointer"
            >
              <IconArchive className="h-3.5 w-3.5 mr-2" />
              {t("kanban:archiveTask")}
            </DropdownMenuItem>
          )}
          <DropdownMenuItem
            onClick={() => setShowDeleteConfirm(true)}
            disabled={isDeleting}
            className="text-destructive focus:text-destructive cursor-pointer"
          >
            <IconTrash className="h-3.5 w-3.5 mr-2" />
            {t("kanban:deleteTask")}
          </DropdownMenuItem>
        </DropdownMenuContent>
      </DropdownMenu>
      <TaskDeleteConfirmDialog
        open={showDeleteConfirm}
        onOpenChange={setShowDeleteConfirm}
        taskTitle={task.title}
        taskId={task.id}
        executorType={task.primaryExecutorType}
        isDeleting={isDeleting}
        onConfirm={({ cascade }) => onDeleteTask(task, { cascade })}
      />
      <TaskArchiveConfirmation
        open={showArchiveConfirm}
        anchorRef={archiveAnchorRef}
        onOpenChange={setShowArchiveConfirm}
        taskTitle={task.title}
        taskId={task.id}
        executorType={task.primaryExecutorType}
        isArchiving={isArchiving}
        onConfirm={({ cascade }) => onArchiveTask?.(task, { cascade })}
      />
    </div>
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
  const { t } = useTranslation();
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
            {t("kanban:sessionCount", { count: sessionCount })}
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
  moveTargetSteps,
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
  const { t } = useTranslation();
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
      className="flex min-w-max items-center justify-start rounded-lg hover:bg-muted/30 transition-colors px-3 py-2"
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
              aria-label={t("kanban:selectTask", { title: task.title })}
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
          moveTargetSteps={moveTargetSteps}
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
