"use client";

import { useMemo } from "react";
import { useDroppable } from "@dnd-kit/core";
import { SortableContext, verticalListSortingStrategy } from "@dnd-kit/sortable";
import { useTranslation } from "react-i18next";
import { KanbanCard, resolveTaskRepositoryChips, Task } from "./kanban-card";
import { Badge } from "@kandev/ui/badge";
import { cn } from "@/lib/utils";
import { useAppStore } from "@/components/state-provider";
import type { KanbanExternalLinkAvailability } from "./kanban-external-link-availability";
import type { Repository } from "@/lib/types/http";
import { partitionKanbanColumnTasks } from "@/lib/kanban/task-order";
import { countAdmittedTasks, formatWipCount, isOverWipLimit } from "@/lib/kanban/wip-limit";

export interface WorkflowStep {
  id: string;
  title: string;
  color: string;
  events?: {
    on_enter?: Array<{ type: string; config?: Record<string, unknown> }>;
    on_turn_complete?: Array<{ type: string; config?: Record<string, unknown> }>;
  };
  wip_limit?: number;
  pull_from_step_id?: string | null;
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
  steps?: WorkflowStep[];
  showMaximizeButton?: boolean;
  deletingTaskId?: string | null;
  archivingTaskId?: string | null;
  hideHeader?: boolean;
  selectedIds?: Set<string>;
  onToggleSelect?: (taskId: string) => void;
  /** Shift-click range select; receives the clicked id + this column's ordered ids. */
  onSelectRange?: (taskId: string, orderedIds: string[]) => void;
  isMultiSelectMode?: boolean;
  externalLinkAvailability: KanbanExternalLinkAvailability;
}

function KanbanColumnHeader({
  step,
  overWipLimit,
  wipCountLabel,
}: {
  step: WorkflowStep;
  overWipLimit: boolean;
  wipCountLabel: string;
}) {
  const { t } = useTranslation();
  return (
    <div className="flex items-center justify-between pb-2 mb-3 px-1">
      <div className="flex items-center gap-2">
        <div className={cn("w-2 h-2 rounded-full", step.color)} />
        <h2 className="font-semibold text-sm">{step.title}</h2>
        <Badge
          variant="secondary"
          className={cn(
            "text-xs tabular-nums",
            overWipLimit &&
              "border-amber-500/50 bg-amber-500/15 text-amber-700 dark:text-amber-300",
          )}
          aria-label={
            overWipLimit ? t("common:tasksOverWipLimit", { countLabel: wipCountLabel }) : undefined
          }
          title={overWipLimit ? t("common:overWipLimit") : undefined}
        >
          {wipCountLabel}
        </Badge>
      </div>
    </div>
  );
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
  steps,
  showMaximizeButton,
  deletingTaskId,
  archivingTaskId,
  hideHeader = false,
  selectedIds,
  onToggleSelect,
  onSelectRange,
  isMultiSelectMode,
  externalLinkAvailability,
}: KanbanColumnProps) {
  const { setNodeRef, isOver } = useDroppable({ id: step.id });
  const activeWorkspaceId = useAppStore((state) => state.workspaces.activeId);
  const repositoriesByWorkspace = useAppStore((state) => state.repositories.itemsByWorkspaceId);
  const repositories = useMemo(
    () => Object.values(repositoriesByWorkspace).flat() as Repository[],
    [repositoriesByWorkspace],
  );

  const { admitted, queued } = useMemo(() => partitionKanbanColumnTasks(tasks), [tasks]);
  const columnTaskIds = useMemo(
    () => [...admitted, ...queued].map((t) => t.id),
    [admitted, queued],
  );
  const admittedIds = useMemo(() => admitted.map((t) => t.id), [admitted]);
  const admittedTaskCount = countAdmittedTasks(tasks);
  const overWipLimit = isOverWipLimit(admittedTaskCount, step.wip_limit);
  const wipCountLabel = formatWipCount(admittedTaskCount, step.wip_limit);

  const renderCard = (task: Task) => (
    <KanbanCard
      key={task.id}
      task={queuedTaskWithTitle(task, steps, step)}
      workspaceId={activeWorkspaceId}
      externalLinkAvailability={externalLinkAvailability}
      repositoryChips={resolveTaskRepositoryChips(task, repositories)}
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
      isSelected={selectedIds?.has(task.id)}
      selectedIds={selectedIds}
      onToggleSelect={onToggleSelect}
      onRangeSelect={onSelectRange ? (taskId) => onSelectRange(taskId, columnTaskIds) : undefined}
      isMultiSelectMode={isMultiSelectMode}
    />
  );

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
      {!hideHeader && (
        <KanbanColumnHeader step={step} overWipLimit={overWipLimit} wipCountLabel={wipCountLabel} />
      )}
      <div className="flex-1 min-h-0 overflow-y-auto overflow-x-hidden space-y-2 px-1 pt-1">
        <SortableContext items={admittedIds} strategy={verticalListSortingStrategy}>
          {admitted.map((task) => renderCard(task))}
        </SortableContext>
        {queued.map((task) => renderCard(task))}
      </div>
    </div>
  );
}

function queuedTaskWithTitle(
  task: Task,
  steps: WorkflowStep[] | undefined,
  step: WorkflowStep,
): Task {
  if (!task.queuedForStepId) return task;
  return {
    ...task,
    queuedForStepTitle:
      steps?.find((candidate) => candidate.id === task.queuedForStepId)?.title ??
      (task.queuedForStepId === step.id ? step.title : undefined),
  };
}
