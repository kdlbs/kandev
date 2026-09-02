"use client";

import { useMemo, useState } from "react";
import { IconDots } from "@tabler/icons-react";
import { DropdownMenu, DropdownMenuContent, DropdownMenuTrigger } from "@kandev/ui/dropdown-menu";
import { Checkbox } from "@kandev/ui/checkbox";
import { cn } from "@kandev/ui/lib/utils";
import { PRTaskIcon } from "@/components/github/pr-task-icon";
import { MRTaskIcon } from "@/components/gitlab/mr-task-icon";
import { RegisteredChangeRequestTaskIcon } from "@/components/integrations/registered-change-request-task-icon";
import {
  KanbanCardDropdownMenuItems,
  type KanbanCardMenuEntry,
} from "@/components/kanban-card-menu-items";
import { KanbanCardContextMenu } from "@/components/kanban-card-context-menu";
import {
  useKanbanCardMenus,
  KanbanCardDialogs,
  type KanbanCardMenuState,
} from "@/components/kanban-card-menu";
import { TaskCardIndicators, TaskCardTags } from "@/components/kanban-card-plugin-slots";
import { KanbanCardBadges, RepoChipRow } from "@/components/kanban-card-status-strip";
import { CardTitle } from "@/components/kanban-card-title";
import { renderSubagentCountChip } from "@/components/kanban-card-content";
import { resolveTaskRepositoryChips } from "@/components/kanban-card-repositories";
import { TaskArchiveConfirmation } from "@/components/task/task-archive-confirmation";
import { TaskDetachConfirmationSurface } from "@/components/task/task-detach-confirm-dialog";
import { RemoteCloudTooltip } from "@/components/task/remote-cloud-tooltip";
import { formatRelativeTime } from "@/lib/utils";
import { needsAction } from "@/lib/utils/needs-action";
import { usePipelineOverflowStage } from "@/hooks/use-pipeline-overflow-stage";
import { Graph2StepNode, Graph2UnassignedStepMarker } from "./graph2-step-node";
import { Graph2Connector } from "./graph2-connector";
import { isOrphanMoveTarget } from "./swimlane-kanban-content";
import { dispatchKanbanCardClick, type Task } from "@/components/kanban-card";
import type { WorkflowStep } from "@/components/kanban-column";
import type { KanbanExternalLinkAvailability } from "@/components/kanban-external-link-availability";
import type { Repository } from "@/lib/types/http";
import { useTranslation } from "react-i18next";

type ConnectorType = "past" | "transition" | "future";

export type Graph2TaskPipelineProps = {
  task: Task;
  steps: WorkflowStep[];
  moveTargetSteps: WorkflowStep[];
  workspaceId: string | null;
  externalLinkAvailability: KanbanExternalLinkAvailability;
  repositories: Repository[];
  onMoveTask: (task: Task, targetStepId: string) => void;
  onPreviewTask: (task: Task) => void;
  onOpenTask: (task: Task) => void;
  onEditTask?: (task: Task) => void;
  onDeleteTask: (task: Task, opts?: { cascade?: boolean }) => void;
  onArchiveTask?: (task: Task, opts?: { cascade?: boolean }) => void;
  isMoving?: boolean;
  isDeleting?: boolean;
  isArchiving?: boolean;
  isSelected?: boolean;
  selectedIds?: Set<string>;
  onToggleSelect?: (taskId: string) => void;
  onRangeSelect?: (taskId: string) => void;
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
  nextStepTitle?: string;
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
  moveTargetSteps: WorkflowStep[],
  stepId: string,
): StepMoveTargets {
  const adjacency = getStepAdjacencyForStep(moveTargetSteps, stepId);
  const prevStep = moveTargetSteps.find((step) => step.id === adjacency.prevStepId);
  const nextStep = moveTargetSteps.find((step) => step.id === adjacency.nextStepId);
  return {
    ...adjacency,
    prevStepTitle: prevStep?.title,
    nextStepTitle: nextStep?.title,
  };
}

function PipelineStepNodes({
  steps,
  moveTargetSteps,
  currentStepIndex,
  task,
  onMoveTask,
  isMoving,
  atTerminus,
}: {
  steps: WorkflowStep[];
  moveTargetSteps: WorkflowStep[];
  currentStepIndex: number;
  task: Task;
  onMoveTask: (task: Task, targetStepId: string) => void;
  isMoving?: boolean;
  atTerminus: boolean;
}) {
  // A task whose `workflowStepId` matches no displayed step
  // (currentStepIndex === -1) gets one synthetic labelled marker before the
  // run, so the row keeps its single-label invariant. An empty `steps` list
  // is a distinct case and renders no run at all.
  const showUnassignedMarker = currentStepIndex === -1 && steps.length > 0;

  return (
    <div
      className={cn(
        "flex items-center gap-0",
        atTerminus ? "shrink-0" : "min-w-0 flex-1 overflow-x-auto scrollbar-hide",
      )}
      data-testid="pipeline-step-run-scroll"
    >
      {showUnassignedMarker && (
        <div className="flex items-center">
          <Graph2UnassignedStepMarker />
          <Graph2Connector type="future" />
        </div>
      )}
      {steps.map((step, index) => {
        const phase = getStepPhase(index, currentStepIndex);
        const hasConnector = index < steps.length - 1;
        const connectorType = hasConnector
          ? getConnectorType(phase, getStepPhase(index + 1, currentStepIndex))
          : null;

        const moveTargets = getStepMoveTargets(moveTargetSteps, step.id);

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
              onMoveTask={onMoveTask}
              isMoving={isMoving}
            />

            {connectorType && <Graph2Connector type={connectorType} />}
          </div>
        );
      })}
    </div>
  );
}

/** The row's task-menu trigger, sourced from the shared menu module (AC-UI-PIPELINE-ROW-002.1/002.4). */
function RowMenuTrigger({
  taskId,
  entries,
  triggerRef,
  isProcessing,
}: {
  taskId: string;
  entries: KanbanCardMenuEntry[];
  triggerRef: React.RefObject<HTMLButtonElement | null>;
  isProcessing?: boolean;
}) {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);

  return (
    <DropdownMenu
      open={open}
      onOpenChange={(next) => {
        if (!next && isProcessing) return;
        setOpen(next);
      }}
    >
      <DropdownMenuTrigger asChild>
        <button
          ref={triggerRef}
          type="button"
          data-testid={`pipeline-row-menu-trigger-${taskId}`}
          onClick={(e) => e.stopPropagation()}
          onPointerDown={(e) => e.stopPropagation()}
          className="shrink-0 h-7 w-7 flex items-center justify-center rounded-md text-muted-foreground/60 hover:text-foreground hover:bg-accent/60 transition-colors cursor-pointer [@media(pointer:coarse)]:h-11 [@media(pointer:coarse)]:w-11"
          aria-label={t("kanban:moreOptions")}
        >
          <IconDots className="h-3.5 w-3.5" />
        </button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" className="w-56">
        <KanbanCardDropdownMenuItems entries={entries} />
      </DropdownMenuContent>
    </DropdownMenu>
  );
}

/** The row's status strip: everything ordered after the title and before the step run. */
function RowInlineStatus({ task, innerRef }: { task: Task; innerRef: React.Ref<HTMLDivElement> }) {
  const { t } = useTranslation("common");
  return (
    <div
      ref={innerRef}
      className="flex shrink-0 items-center gap-1.5"
      data-testid="pipeline-row-status-strip"
    >
      <PRTaskIcon taskId={task.id} />
      <MRTaskIcon taskId={task.id} />
      <RegisteredChangeRequestTaskIcon taskId={task.id} />
      <TaskCardIndicators task={task} />
      <TaskCardTags task={task} />
      <KanbanCardBadges task={task} />
      {renderSubagentCountChip(
        task,
        t("common:activeSubagents", { count: task.activeSubagentCount ?? 0 }),
      )}
      {task.isRemoteExecutor && (
        <RemoteCloudTooltip
          taskId={task.id}
          sessionId={task.primarySessionId ?? null}
          executorType={task.primaryExecutorType}
          fallbackName={task.primaryExecutorName ?? task.primaryExecutorType}
        />
      )}
      {task.updatedAt && (
        <span className="text-[10px] text-muted-foreground/60">
          {formatRelativeTime(task.updatedAt)}
        </span>
      )}
    </div>
  );
}

/** The row's accessible position summary: names the current step and its ordinal, or the unassigned state when there is none. */
function RowPositionSummary({
  steps,
  currentStepIndex,
}: {
  steps: WorkflowStep[];
  currentStepIndex: number;
}) {
  const { t } = useTranslation();
  if (steps.length === 0) return null;
  const text =
    currentStepIndex === -1
      ? t("kanban:pipelineUnassignedStep")
      : t("kanban:pipelineRowPosition", {
          title: steps[currentStepIndex].title,
          position: currentStepIndex + 1,
          total: steps.length,
        });
  return (
    <span className="sr-only" data-testid="pipeline-row-position-summary">
      {text}
    </span>
  );
}

type PipelineRowProps = {
  task: Task;
  steps: WorkflowStep[];
  moveTargetSteps: WorkflowStep[];
  repositoryChips: ReturnType<typeof resolveTaskRepositoryChips>;
  currentStepIndex: number;
  menu: KanbanCardMenuState;
  onMoveTask: (task: Task, targetStepId: string) => void;
  onPreviewTask: (task: Task) => void;
  onToggleSelect?: (taskId: string) => void;
  onRangeSelect?: (taskId: string) => void;
  isMoving?: boolean;
  isDeleting?: boolean;
  isArchiving?: boolean;
  isSelected?: boolean;
  isMultiSelectMode?: boolean;
};

/** The row's clickable body: repo chips, title, inline status, step run, and menu trigger. */
function PipelineRow({
  task,
  steps,
  moveTargetSteps,
  repositoryChips,
  currentStepIndex,
  menu,
  onMoveTask,
  onPreviewTask,
  onToggleSelect,
  onRangeSelect,
  isMoving,
  isDeleting,
  isArchiving,
  isSelected,
  isMultiSelectMode,
}: PipelineRowProps) {
  const { t } = useTranslation();
  const showCheckbox = isMultiSelectMode || !!isSelected;
  const overflowStage = usePipelineOverflowStage<HTMLDivElement, HTMLDivElement>();

  // Preview-aware: matches the Kanban card's own body-click wiring
  // (onClick={onPreviewTask} in virtualized-column-task-list.tsx). Falls back
  // to full-page navigation when "Open preview on click" is off — see
  // useKanbanNavigation's handleCardClick.
  const handleClick = (e: React.MouseEvent) =>
    dispatchKanbanCardClick(e, task.id, task, {
      onToggleSelect,
      onRangeSelect,
      onClick: onPreviewTask,
      isMultiSelectMode,
    });

  const handleCheckboxClick = (e: React.MouseEvent) => {
    e.stopPropagation();
    onToggleSelect?.(task.id);
  };

  return (
    <div
      data-testid={`pipeline-task-${task.id}`}
      className={cn(
        "flex w-full min-w-0 items-center gap-2 rounded-lg px-3 py-2 transition-colors hover:bg-muted/30 cursor-pointer",
        needsAction(task) && !isSelected && "border-l-2 !border-l-amber-500",
        isSelected && "ring-1 ring-primary/60",
      )}
      onClick={handleClick}
    >
      <RowPositionSummary steps={steps} currentStepIndex={currentStepIndex} />
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
      <RepoChipRow chips={repositoryChips} />
      <div
        className="min-w-0"
        data-testid="pipeline-row-title"
        style={{ flex: "1 1 auto", minWidth: "96px", maxWidth: "200px" }}
      >
        <CardTitle task={task} enableTitleHover />
      </div>
      <div
        ref={overflowStage.outerRef}
        data-testid="pipeline-row-overflow-region"
        className={cn(
          "flex min-w-0 items-center gap-1.5",
          overflowStage.atTerminus && "overflow-x-auto scrollbar-hide",
        )}
        style={{ flex: "1 1 0%" }}
      >
        <RowInlineStatus task={task} innerRef={overflowStage.stripRef} />
        <PipelineStepNodes
          steps={steps}
          moveTargetSteps={moveTargetSteps}
          currentStepIndex={currentStepIndex}
          task={task}
          onMoveTask={onMoveTask}
          isMoving={isMoving}
          atTerminus={overflowStage.atTerminus}
        />
      </div>
      {!isMultiSelectMode && (
        <div className="ml-auto shrink-0">
          <RowMenuTrigger
            taskId={task.id}
            entries={menu.dropdownMenuEntries}
            triggerRef={menu.detachFocusReturnRef}
            isProcessing={isDeleting || isArchiving}
          />
        </div>
      )}
    </div>
  );
}

/** The row's dialogs/confirmations, sourced from the shared menu module (AC-UI-PIPELINE-ROW-002.4). */
function PipelineDialogs({
  task,
  workspaceId,
  repositories,
  menu,
  isDeleting,
  isArchiving,
  onDeleteTask,
  onArchiveTask,
}: {
  task: Task;
  workspaceId: string | null;
  repositories: Repository[];
  menu: KanbanCardMenuState;
  isDeleting?: boolean;
  isArchiving?: boolean;
  onDeleteTask: (task: Task, opts?: { cascade?: boolean }) => void;
  onArchiveTask?: (task: Task, opts?: { cascade?: boolean }) => void;
}) {
  return (
    <>
      <KanbanCardDialogs
        task={task}
        workspaceId={workspaceId}
        repositories={repositories}
        menu={menu}
        isDeleting={isDeleting}
        onDelete={onDeleteTask}
      />
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
        onConfirm={({ cascade }) => onArchiveTask?.(task, { cascade })}
      />
    </>
  );
}

export function Graph2TaskPipeline({
  task,
  steps,
  moveTargetSteps,
  workspaceId,
  externalLinkAvailability,
  repositories,
  onMoveTask,
  onPreviewTask,
  onEditTask,
  onDeleteTask,
  onArchiveTask,
  isMoving,
  isDeleting,
  isArchiving,
  isSelected,
  selectedIds,
  onToggleSelect,
  onRangeSelect,
  isMultiSelectMode,
}: Graph2TaskPipelineProps) {
  const currentStepIndex = useMemo(
    () => steps.findIndex((s) => s.id === task.workflowStepId),
    [steps, task.workflowStepId],
  );
  const repositoryChips = useMemo(
    () => resolveTaskRepositoryChips(task, repositories),
    [task, repositories],
  );

  const menu = useKanbanCardMenus({
    task,
    workspaceId,
    externalLinkAvailability,
    steps: moveTargetSteps,
    isDeleting,
    isArchiving,
    isMoving,
    isSelected,
    selectedIds,
    onEdit: onEditTask,
    onDelete: onDeleteTask,
    onArchive: onArchiveTask,
    onMove: onMoveTask,
  });

  const row = (
    <PipelineRow
      task={task}
      steps={steps}
      moveTargetSteps={moveTargetSteps}
      repositoryChips={repositoryChips}
      currentStepIndex={currentStepIndex}
      menu={menu}
      onMoveTask={onMoveTask}
      onPreviewTask={onPreviewTask}
      onToggleSelect={onToggleSelect}
      onRangeSelect={onRangeSelect}
      isMoving={isMoving}
      isDeleting={isDeleting}
      isArchiving={isArchiving}
      isSelected={isSelected}
      isMultiSelectMode={isMultiSelectMode}
    />
  );

  return (
    <>
      <div ref={menu.detachAnchorRef} className="w-full">
        {isMultiSelectMode ? (
          row
        ) : (
          <KanbanCardContextMenu entries={menu.contextMenuEntries}>{row}</KanbanCardContextMenu>
        )}
      </div>
      <PipelineDialogs
        task={task}
        workspaceId={workspaceId}
        repositories={repositories}
        menu={menu}
        isDeleting={isDeleting}
        isArchiving={isArchiving}
        onDeleteTask={onDeleteTask}
        onArchiveTask={onArchiveTask}
      />
    </>
  );
}
