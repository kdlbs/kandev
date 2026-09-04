"use client";

import { memo, useCallback, useMemo, useRef } from "react";
import { useDroppable } from "@dnd-kit/core";
import { useVirtualizer } from "@tanstack/react-virtual";
import { useTranslation } from "react-i18next";
import { cn } from "@kandev/ui/lib/utils";
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
  /** The task currently being dragged anywhere on the board, if any (AC.7's insertion indicator). */
  activeTaskId?: string | null;
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
  "orderedTasks" | "queuedStartIndex" | "queuedCount" | "deletingTaskId" | "archivingTaskId"
> & {
  task: Task;
  columnTaskIds: string[];
  isDeleting: boolean;
  isArchiving: boolean;
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
  isDeleting,
  isArchiving,
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
      isDeleting={isDeleting}
      isArchiving={isArchiving}
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

/**
 * Wraps one rendered card so it is also a dnd-kit drop target (`over.id`
 * resolves to a task id, enabling within-band reorder classification) while
 * staying measured by the virtualizer. Renders the AC.7 insertion-point
 * indicator when this card is the current drop target for a same-band drag.
 */
function DroppableTaskRow({
  taskId,
  index,
  top,
  measureElement,
  insertionEdge,
  children,
}: {
  taskId: string;
  index: number;
  top: number;
  measureElement: (node: HTMLDivElement | null) => void;
  insertionEdge: "top" | "bottom" | null;
  children: React.ReactNode;
}) {
  const { setNodeRef, isOver } = useDroppable({ id: taskId });
  const mergedRef = useCallback(
    (node: HTMLDivElement | null) => {
      measureElement(node);
      setNodeRef(node);
    },
    [measureElement, setNodeRef],
  );
  const showIndicator = isOver && insertionEdge !== null;

  return (
    <div
      ref={mergedRef}
      data-index={index}
      className={cn(
        "absolute left-0 top-0 w-full",
        showIndicator && insertionEdge === "top" && "border-t-2 border-primary",
        showIndicator && insertionEdge === "bottom" && "border-b-2 border-primary",
      )}
      style={{ transform: `translateY(${top}px)` }}
      data-testid={showIndicator ? `kanban-insertion-indicator-${insertionEdge}` : undefined}
    >
      {children}
    </div>
  );
}

/**
 * AC.7: while a card is dragged over its own band, show which edge of the
 * hovered card the drop would insert next to. `null` for a different band
 * (AC.11's cross-band reject) or when nothing is being dragged.
 */
export function computeInsertionEdge(
  orderedTasks: Task[],
  queuedStartIndex: number,
  activeTaskId: string | null | undefined,
  index: number,
): "top" | "bottom" | null {
  if (!activeTaskId) return null;
  const activeIndex = orderedTasks.findIndex((task) => task.id === activeTaskId);
  if (activeIndex === -1 || activeIndex === index) return null;
  if (activeIndex < queuedStartIndex !== index < queuedStartIndex) return null;
  return activeIndex < index ? "bottom" : "top";
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
  activeTaskId,
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
            <DroppableTaskRow
              key={task.id}
              taskId={task.id}
              index={virtualItem.index}
              top={virtualItem.start}
              measureElement={virtualizer.measureElement}
              insertionEdge={computeInsertionEdge(
                orderedTasks,
                queuedStartIndex,
                activeTaskId,
                virtualItem.index,
              )}
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
                isDeleting={deletingTaskId === task.id}
                isArchiving={archivingTaskId === task.id}
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
            </DroppableTaskRow>
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
