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
import type { ReorderBand, Repository } from "@/lib/types/http";
import type { WorkflowStep } from "../kanban-column";
import type { KanbanExternalLinkAvailability } from "../kanban-external-link-availability";

/** A card picked up for a keyboard reorder gesture (REQ-TASKS-KANBAN-TASK-REORDERING-001.12). */
export type KeyboardReorderDraft = {
  taskId: string;
  stepId: string;
  band: ReorderBand;
  order: string[];
};

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
  /** The card currently picked up via the keyboard, if any (AC.12). */
  keyboardDraft?: KeyboardReorderDraft | null;
  onCardKeyDown?: (event: React.KeyboardEvent, task: Task) => void;
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
  forceShowIndicator,
  children,
}: {
  taskId: string;
  index: number;
  top: number;
  measureElement: (node: HTMLDivElement | null) => void;
  insertionEdge: "top" | "bottom" | null;
  /** True while a keyboard reorder (no pointer drag, so no `isOver`) targets this row. */
  forceShowIndicator: boolean;
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
  const showIndicator = (isOver || forceShowIndicator) && insertionEdge !== null;

  return (
    <div
      ref={mergedRef}
      data-index={index}
      className={cn(
        "absolute left-0 w-full",
        showIndicator && insertionEdge === "top" && "border-t-2 border-primary",
        showIndicator && insertionEdge === "bottom" && "border-b-2 border-primary",
      )}
      style={{ top: `${top}px` }}
      data-testid={showIndicator ? `kanban-insertion-indicator-${insertionEdge}` : undefined}
    >
      {children}
    </div>
  );
}

/**
 * AC.7: while a card is dragged over its own band, show which edge of the
 * hovered card the drop would insert next to. `null` for a different band
 * (AC.11's cross-band reject) or when nothing is being dragged. Takes the
 * dragged/picked-up card's index directly rather than resolving it from an
 * id, so the same math serves both the pointer path (its static starting
 * index, gated by that row's own `isOver`) and the keyboard path (the
 * draft's current virtual index, gated by strict adjacency below).
 */
export function computeInsertionEdge(
  queuedStartIndex: number,
  activeIndex: number | null,
  index: number,
): "top" | "bottom" | null {
  if (activeIndex === null || activeIndex === index) return null;
  if (activeIndex < queuedStartIndex !== index < queuedStartIndex) return null;
  return activeIndex < index ? "bottom" : "top";
}

export function findTaskIndex(
  orderedTasks: Task[],
  taskId: string | null | undefined,
): number | null {
  if (!taskId) return null;
  const index = orderedTasks.findIndex((task) => task.id === taskId);
  return index === -1 ? null : index;
}

/**
 * The insertion edge to render on a rendered row for an in-progress keyboard
 * reorder (REQ-TASKS-KANBAN-TASK-REORDERING-001.12), or `null` if that row is
 * not adjacent to the draft's current position. Unlike the pointer path, this
 * cannot compare numeric indices: `draft.order` is the band-local proposed
 * order and evolves with every arrow press, while keyboard reorder never
 * live-reorders the DOM, so a rendered row's index stays fixed to the
 * unmoved order - after more than one move, the two index spaces name
 * different tasks at the same offset. Row identity is compared by task id
 * against the draft's immediate neighbors instead.
 */
export function computeKeyboardInsertionEdge(
  draft: KeyboardReorderDraft | null | undefined,
  stepId: string,
  rowTaskId: string,
): "top" | "bottom" | null {
  if (!draft || draft.stepId !== stepId) return null;
  const draftIndex = draft.order.indexOf(draft.taskId);
  if (draftIndex === -1) return null;
  const rowIndex = draft.order.indexOf(rowTaskId);
  if (rowIndex === -1) return null;
  if (rowIndex === draftIndex - 1) return "bottom";
  if (rowIndex === draftIndex + 1) return "top";
  return null;
}

type VirtualizedTaskRowProps = Pick<
  VirtualizedColumnTaskListProps,
  | "step"
  | "steps"
  | "presentation"
  | "workspaceId"
  | "repositories"
  | "externalLinkAvailability"
  | "showMaximizeButton"
  | "deletingTaskId"
  | "archivingTaskId"
  | "selectedIds"
  | "keyboardDraft"
  | "onCardKeyDown"
  | "onPreviewTask"
  | "onOpenTask"
  | "onEditTask"
  | "onDeleteTask"
  | "onArchiveTask"
  | "onMoveTask"
  | "onToggleSelect"
  | "onSelectRange"
  | "isMultiSelectMode"
> & {
  task: Task;
  queuedCount: number;
  queuedStartIndex: number;
  virtualIndex: number;
  top: number;
  measureElement: (node: HTMLDivElement | null) => void;
  insertionEdge: "top" | "bottom" | null;
  forceShowIndicator: boolean;
  columnTaskIds: string[];
};

/** One rendered card row, including its optional "Queued" section header. */
function VirtualizedTaskRow({
  task,
  queuedCount,
  queuedStartIndex,
  virtualIndex,
  top,
  measureElement,
  insertionEdge,
  forceShowIndicator,
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
  keyboardDraft,
  onCardKeyDown,
  onPreviewTask,
  onOpenTask,
  onEditTask,
  onDeleteTask,
  onArchiveTask,
  onMoveTask,
  onToggleSelect,
  onSelectRange,
  isMultiSelectMode,
}: VirtualizedTaskRowProps) {
  const { t } = useTranslation();
  return (
    <DroppableTaskRow
      taskId={task.id}
      index={virtualIndex}
      top={top}
      measureElement={measureElement}
      insertionEdge={insertionEdge}
      forceShowIndicator={forceShowIndicator}
    >
      {queuedCount > 0 && virtualIndex === queuedStartIndex && (
        <div
          className="mb-2 flex items-center gap-2 border-t border-dashed border-border/60 pt-3 text-xs font-medium text-muted-foreground"
          data-testid="kanban-queued-section"
        >
          <span>{t("kanban:queuedSection")}</span>
          <span className="tabular-nums">{queuedCount}</span>
        </div>
      )}
      <KanbanCard
        task={queuedTaskWithTitle(task, steps, step)}
        workspaceId={workspaceId}
        presentation={presentation}
        externalLinkAvailability={externalLinkAvailability}
        repositoryChips={resolveTaskRepositoryChips(task, repositories)}
        onClick={onPreviewTask}
        onOpenFullPage={onOpenTask}
        onEdit={onEditTask}
        onDelete={onDeleteTask}
        onCardKeyDown={onCardKeyDown}
        isPickedUpForReorder={keyboardDraft?.taskId === task.id}
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
    </DroppableTaskRow>
  );
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
  keyboardDraft,
  onCardKeyDown,
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
  const pointerActiveIndex = findTaskIndex(orderedTasks, activeTaskId);

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

          const keyboardInsertionEdge = computeKeyboardInsertionEdge(
            keyboardDraft,
            step.id,
            task.id,
          );
          const isKeyboardAdjacent = keyboardInsertionEdge !== null;
          const insertionEdge = isKeyboardAdjacent
            ? keyboardInsertionEdge
            : computeInsertionEdge(queuedStartIndex, pointerActiveIndex, virtualItem.index);

          return (
            <VirtualizedTaskRow
              key={task.id}
              task={task}
              queuedCount={queuedCount}
              queuedStartIndex={queuedStartIndex}
              virtualIndex={virtualItem.index}
              top={virtualItem.start}
              measureElement={virtualizer.measureElement}
              insertionEdge={insertionEdge}
              forceShowIndicator={isKeyboardAdjacent}
              columnTaskIds={columnTaskIds}
              step={step}
              steps={steps}
              presentation={presentation}
              workspaceId={workspaceId}
              repositories={repositories}
              externalLinkAvailability={externalLinkAvailability}
              showMaximizeButton={showMaximizeButton}
              deletingTaskId={deletingTaskId}
              archivingTaskId={archivingTaskId}
              selectedIds={selectedIds}
              keyboardDraft={keyboardDraft}
              onCardKeyDown={onCardKeyDown}
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
