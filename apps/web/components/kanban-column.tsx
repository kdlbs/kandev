"use client";

import { useMemo } from "react";
import { useDroppable } from "@dnd-kit/core";
import { Task, type KanbanPresentation } from "./kanban-card";
import { Badge } from "@kandev/ui/badge";
import { cn } from "@/lib/utils";
import { useAppStore } from "@/components/state-provider";
import type { KanbanExternalLinkAvailability } from "./kanban-external-link-availability";
import type { Repository } from "@/lib/types/http";
import { countAdmittedTasks, formatWipCount, isOverWipLimit } from "@/lib/kanban/wip-limit";
import { partitionWipTasks } from "@/lib/kanban/wip-queue";
import { useTranslation } from "react-i18next";
import { VirtualizedColumnTaskList } from "./kanban/virtualized-column-task-list";

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
  presentation?: KanbanPresentation;
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

function ColumnHeader({ step, tasks }: { step: WorkflowStep; tasks: Task[] }) {
  const { t } = useTranslation();
  const admittedTaskCount = countAdmittedTasks(tasks);
  const overWipLimit = isOverWipLimit(admittedTaskCount, step.wip_limit);
  const wipCountLabel = formatWipCount(admittedTaskCount, step.wip_limit);
  const queuedCount = partitionWipTasks(tasks, step.id).queued.length;

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
            overWipLimit ? t("kanban:tasksOverWipLimit", { label: wipCountLabel }) : undefined
          }
          title={overWipLimit ? t("kanban:overWipLimit") : undefined}
        >
          {wipCountLabel}
        </Badge>
        {queuedCount > 0 && (
          <span className="text-xs text-muted-foreground" data-testid="kanban-queued-count">
            {t("kanban:queuedCount", { count: queuedCount })}
          </span>
        )}
      </div>
    </div>
  );
}

export function KanbanColumn({
  step,
  tasks,
  presentation = "desktop",
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
  const { setNodeRef, isOver } = useDroppable({
    id: step.id,
  });
  const activeWorkspaceId = useAppStore((state) => state.workspaces.activeId);

  // Access repositories from store to pass repository names to cards
  const repositoriesByWorkspace = useAppStore((state) => state.repositories.itemsByWorkspaceId);
  const repositories = useMemo(
    () => Object.values(repositoriesByWorkspace).flat() as Repository[],
    [repositoriesByWorkspace],
  );

  // Ordered ids of the cards rendered in this column — the source of truth for
  // shift-click range selection (matches exactly what the user sees).
  const { admitted, queued } = useMemo(() => partitionWipTasks(tasks, step.id), [tasks, step.id]);
  const orderedTasks = useMemo(() => [...admitted, ...queued], [admitted, queued]);

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
      {!hideHeader && <ColumnHeader step={step} tasks={tasks} />}

      <VirtualizedColumnTaskList
        orderedTasks={orderedTasks}
        queuedStartIndex={admitted.length}
        queuedCount={queued.length}
        step={step}
        steps={steps}
        presentation={presentation}
        workspaceId={activeWorkspaceId}
        repositories={repositories}
        externalLinkAvailability={externalLinkAvailability}
        showMaximizeButton={showMaximizeButton}
        deletingTaskId={deletingTaskId}
        archivingTaskId={archivingTaskId}
        selectedIds={selectedIds}
        onPreviewTask={onPreviewTask}
        onOpenTask={onOpenTask}
        onEditTask={onEditTask}
        onDeleteTask={onDeleteTask}
        onArchiveTask={onArchiveTask}
        onMoveTask={onMoveTask}
        onToggleSelect={onToggleSelect}
        onSelectRange={onSelectRange}
        isMultiSelectMode={isMultiSelectMode}
      />
    </div>
  );
}
