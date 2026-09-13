"use client";

import { useState, type Ref, type RefObject } from "react";
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
import type { KanbanCardMenuState } from "@/components/kanban-card-menu";
import { TaskCardIndicators } from "@/components/kanban-card-plugin-slots";
import { KanbanCardBadges, RepoChipRow } from "@/components/kanban-card-status-strip";
import { CardTitle } from "@/components/kanban-card-title";
import { renderSubagentCountChip } from "@/components/kanban-card-content";
import { resolveTaskRepositoryChips } from "@/components/kanban-card-repositories";
import { RemoteCloudTooltip } from "@/components/task/remote-cloud-tooltip";
import { taskPRInfoFromSummary } from "@/lib/task-pr-info";
import { formatRelativeTime } from "@/lib/utils";
import { needsAction } from "@/lib/utils/needs-action";
import { usePipelineOverflowStage } from "@/hooks/use-pipeline-overflow-stage";
import { dispatchKanbanCardClick, type Task } from "@/components/kanban-card";
import type { WorkflowStep } from "@/components/kanban-column";
import { PipelineStepNodes } from "./graph2-task-pipeline-step-run";
import { useTranslation } from "react-i18next";

function RowMenuTrigger({
  taskId,
  entries,
  triggerRef,
  isProcessing,
}: {
  taskId: string;
  entries: KanbanCardMenuEntry[];
  triggerRef: RefObject<HTMLButtonElement | null>;
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
          onContextMenu={(e) => e.stopPropagation()}
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

/**
 * The row's information column: title, repository, relative time and session
 * count, stacked in a column of one fixed width for every row.
 *
 * Fixed, never flexible. A column that sizes to its own row's content makes
 * the step run start at a different x position on every row, so the runs stop
 * reading as one shared track down the board. Holding all rows at 200px is
 * what keeps them aligned; each line truncates rather than widening the
 * column, and the title's full text stays reachable through the hover card.
 */
function RowInfoColumn({
  task,
  repositoryChips,
}: {
  task: Task;
  repositoryChips: ReturnType<typeof resolveTaskRepositoryChips>;
}) {
  const { t } = useTranslation();
  const sessionCount = task.sessionCount ?? 0;

  return (
    <div className="w-[200px] min-w-0 shrink-0" data-testid="pipeline-row-info">
      <div data-testid="pipeline-row-title">
        <CardTitle task={task} enableTitleHover />
      </div>
      {/* Height is reserved whether or not the task has a repository, so a
          row with no repository is the same height as one with, and the
          board keeps a constant row pitch. */}
      <div className="h-4" data-testid="pipeline-row-repo-line">
        <RepoChipRow chips={repositoryChips} />
      </div>
      <div className="flex items-center gap-1.5">
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
    </div>
  );
}

/**
 * The row's status strip: the task's state indicators, seated after the step
 * run so that a row's own indicator count cannot displace its run. The step
 * run flexes, so the strip settles against the actions cluster on the right.
 */
function RowInlineStatus({ task, innerRef }: { task: Task; innerRef: Ref<HTMLDivElement> }) {
  const { t } = useTranslation("common");
  return (
    <div
      ref={innerRef}
      className="flex shrink-0 items-center gap-1.5"
      data-testid="pipeline-row-status-strip"
    >
      <PRTaskIcon taskId={task.id} prInfo={taskPRInfoFromSummary(task.statusSummary)} />
      <MRTaskIcon taskId={task.id} />
      <RegisteredChangeRequestTaskIcon taskId={task.id} />
      <TaskCardIndicators task={task} />
      <KanbanCardBadges task={task} hideSessionCount className="mt-0 flex-nowrap" />
      {renderSubagentCountChip(
        task,
        t("common:activeSubagents", { count: task.activeSubagentCount ?? 0 }),
      )}
      {task.isRemoteExecutor && (
        <RemoteCloudTooltip
          taskId={task.id}
          sessionId={task.primarySessionId ?? null}
          executorId={task.primaryExecutorId}
          executorType={task.primaryExecutorType}
          fallbackName={task.primaryExecutorName ?? task.primaryExecutorType}
        />
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

export type PipelineRowProps = {
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

/** The row's clickable body: information column, step run, inline status, and menu trigger. */
export function PipelineRow({
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
      <RowInfoColumn task={task} repositoryChips={repositoryChips} />
      <div
        ref={overflowStage.outerRef}
        data-testid="pipeline-row-overflow-region"
        className={cn(
          "flex min-w-0 items-center gap-1.5",
          overflowStage.atTerminus && "overflow-x-auto scrollbar-hide",
        )}
        style={{ flex: "1 1 0%" }}
      >
        <PipelineStepNodes
          steps={steps}
          moveTargetSteps={moveTargetSteps}
          currentStepIndex={currentStepIndex}
          task={task}
          onMoveTask={onMoveTask}
          isMoving={isMoving}
          atTerminus={overflowStage.atTerminus}
        />
        <RowInlineStatus task={task} innerRef={overflowStage.stripRef} />
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
