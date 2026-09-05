"use client";

import { memo, useCallback, useMemo, useRef } from "react";
import { useVirtualizer } from "@tanstack/react-virtual";
import { useTranslation } from "react-i18next";
import {
  KanbanCard,
  resolveTaskRepositoryChips,
  Task,
  type KanbanPresentation,
} from "../kanban-card";
import type { Repository } from "@/lib/types/http";
import type { WorkflowStep } from "../kanban-column";
import type { KanbanExternalLinkAvailability } from "../kanban-external-link-availability";

type VirtualizedColumnTaskListProps = {
  orderedTasks: Task[];
  queuedStartIndex: number;
  queuedCount: number;
  step: WorkflowStep;
  steps?: WorkflowStep[];
  presentation: KanbanPresentation;
  workspaceId: string | null;
  repositories: Repository[];
  externalLinkAvailability: KanbanExternalLinkAvailability;
  showMaximizeButton?: boolean;
  deletingTaskId?: string | null;
  archivingTaskId?: string | null;
  selectedIds?: Set<string>;
  onPreviewTask: (task: Task) => void;
  onOpenTask: (task: Task) => void;
  onEditTask: (task: Task) => void;
  onDeleteTask: (task: Task) => void;
  onArchiveTask?: (task: Task) => void;
  onMoveTask?: (task: Task, targetStepId: string) => void;
  onToggleSelect?: (taskId: string) => void;
  onSelectRange?: (taskId: string, orderedIds: string[]) => void;
  isMultiSelectMode?: boolean;
};

type VirtualizedKanbanCardProps = Omit<
  VirtualizedColumnTaskListProps,
  "orderedTasks" | "queuedStartIndex" | "queuedCount"
> & {
  task: Task;
  columnTaskIds: string[];
};

const VirtualizedKanbanCard = memo(function VirtualizedKanbanCard({
  task,
  columnTaskIds,
  step,
  steps,
  presentation,
  workspaceId,
  repositories,
  externalLinkAvailability,
  showMaximizeButton,
  deletingTaskId,
  archivingTaskId,
  selectedIds,
  onPreviewTask,
  onOpenTask,
  onEditTask,
  onDeleteTask,
  onArchiveTask,
  onMoveTask,
  onToggleSelect,
  onSelectRange,
  isMultiSelectMode,
}: VirtualizedKanbanCardProps) {
  const displayTask = useMemo(() => queuedTaskWithTitle(task, steps, step), [step, steps, task]);
  const repositoryChips = useMemo(
    () => resolveTaskRepositoryChips(task, repositories),
    [repositories, task],
  );
  const handleRangeSelect = useCallback(
    (taskId: string) => onSelectRange?.(taskId, columnTaskIds),
    [columnTaskIds, onSelectRange],
  );

  return (
    <KanbanCard
      task={displayTask}
      workspaceId={workspaceId}
      presentation={presentation}
      externalLinkAvailability={externalLinkAvailability}
      repositoryChips={repositoryChips}
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
      onRangeSelect={onSelectRange ? handleRangeSelect : undefined}
      isMultiSelectMode={isMultiSelectMode}
    />
  );
});

function useStableTaskIds(tasks: Task[]): string[] {
  const previousRef = useRef<string[]>([]);
  const next = tasks.map((task) => task.id);
  const previous = previousRef.current;
  const isUnchanged =
    previous.length === next.length && previous.every((taskId, index) => taskId === next[index]);
  if (!isUnchanged) previousRef.current = next;
  return isUnchanged ? previous : next;
}

function useStableExternalLinkAvailability(
  availability: KanbanExternalLinkAvailability,
): KanbanExternalLinkAvailability {
  const previousRef = useRef(availability);
  const previous = previousRef.current;
  const isUnchanged =
    previous.gitlab === availability.gitlab &&
    previous.jira === availability.jira &&
    previous.linear === availability.linear &&
    previous.sentry === availability.sentry;
  if (!isUnchanged) previousRef.current = availability;
  return isUnchanged ? previous : availability;
}

export function VirtualizedColumnTaskList({
  orderedTasks,
  queuedStartIndex,
  queuedCount,
  step,
  steps,
  presentation,
  workspaceId,
  repositories,
  externalLinkAvailability,
  showMaximizeButton,
  deletingTaskId,
  archivingTaskId,
  selectedIds,
  onPreviewTask,
  onOpenTask,
  onEditTask,
  onDeleteTask,
  onArchiveTask,
  onMoveTask,
  onToggleSelect,
  onSelectRange,
  isMultiSelectMode,
}: VirtualizedColumnTaskListProps) {
  const { t } = useTranslation();
  const scrollRef = useRef<HTMLDivElement>(null);
  const columnTaskIds = useStableTaskIds(orderedTasks);
  const stableExternalLinkAvailability =
    useStableExternalLinkAvailability(externalLinkAvailability);
  const virtualizer = useVirtualizer<HTMLDivElement, HTMLDivElement>({
    count: orderedTasks.length,
    getScrollElement: () => scrollRef.current,
    estimateSize: (index) => (queuedCount > 0 && index === queuedStartIndex ? 136 : 96),
    getItemKey: (index) => orderedTasks[index]?.id ?? index,
    overscan: 5,
  });

  return (
    <div
      ref={scrollRef}
      className="flex-1 min-h-0 overflow-y-auto overflow-x-hidden px-1 pt-1"
      data-testid="kanban-column-scroll"
    >
      <div className="relative w-full" style={{ height: `${virtualizer.getTotalSize()}px` }}>
        {virtualizer.getVirtualItems().map((virtualItem) => {
          const task = orderedTasks[virtualItem.index];
          if (!task) return null;

          return (
            <div
              key={task.id}
              ref={virtualizer.measureElement}
              data-index={virtualItem.index}
              className="absolute left-0 top-0 w-full"
              style={{ transform: `translateY(${virtualItem.start}px)` }}
            >
              {queuedCount > 0 && virtualItem.index === queuedStartIndex && (
                <div
                  className="mb-2 flex items-center gap-2 border-t border-dashed border-border/60 pt-3 text-xs font-medium text-muted-foreground"
                  data-testid="kanban-queued-section"
                >
                  <span>{t("kanban:queuedSection")}</span>
                  <span className="tabular-nums">{queuedCount}</span>
                </div>
              )}
              <VirtualizedKanbanCard
                task={task}
                columnTaskIds={columnTaskIds}
                step={step}
                steps={steps}
                presentation={presentation}
                workspaceId={workspaceId}
                repositories={repositories}
                externalLinkAvailability={stableExternalLinkAvailability}
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
        })}
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
