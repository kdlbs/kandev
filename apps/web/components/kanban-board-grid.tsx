'use client';

import { useEffect, useMemo, useRef } from 'react';
import {
  DndContext,
  DragEndEvent,
  DragOverlay,
  DragStartEvent,
  PointerSensor,
  TouchSensor,
  useSensor,
  useSensors,
} from '@dnd-kit/core';
import { KanbanColumn, WorkflowStep } from './kanban-column';
import { KanbanCardPreview, Task } from './kanban-card';
import { MobileColumnTabs } from './kanban/mobile-column-tabs';
import { SwipeableColumns } from './kanban/swipeable-columns';
import { MobileDropTargets } from './kanban/mobile-drop-targets';
import { MobileFab } from './kanban/mobile-fab';
import { useResponsiveBreakpoint } from '@/hooks/use-responsive-breakpoint';
import { useAppStore } from '@/components/state-provider';

export type KanbanBoardGridProps = {
  columns: WorkflowStep[];
  tasks: Task[];
  onPreviewTask: (task: Task) => void;
  onOpenTask: (task: Task) => void;
  onEditTask: (task: Task) => void;
  onDeleteTask: (task: Task) => void;
  onMoveTask?: (task: Task, targetColumnId: string) => void;
  onDragStart: (event: DragStartEvent) => void;
  onDragEnd: (event: DragEndEvent) => void;
  onDragCancel: () => void;
  activeTask: Task | null;
  showMaximizeButton?: boolean;
  deletingTaskId?: string | null;
  onCreateTask?: () => void;
  isLoading?: boolean;
};

export function KanbanBoardGrid({
  columns,
  tasks,
  onPreviewTask,
  onOpenTask,
  onEditTask,
  onDeleteTask,
  onMoveTask,
  onDragStart,
  onDragEnd,
  onDragCancel,
  activeTask,
  showMaximizeButton,
  deletingTaskId,
  onCreateTask,
  isLoading,
}: KanbanBoardGridProps) {
  const { isMobile, isTablet } = useResponsiveBreakpoint();
  const activeColumnIndex = useAppStore((state) => state.mobileKanban.activeColumnIndex);
  const setActiveColumnIndex = useAppStore((state) => state.setMobileKanbanColumnIndex);

  // Use TouchSensor with delay for mobile (long-press to drag)
  const sensors = useSensors(
    useSensor(PointerSensor, {
      activationConstraint: {
        distance: 8,
      },
    }),
    useSensor(TouchSensor, {
      activationConstraint: {
        delay: 250,
        tolerance: 5,
      },
    })
  );

  const getTasksForColumn = (columnId: string) => {
    return tasks
      .filter((task) => task.workflowStepId === columnId)
      .map((task) => ({ ...task, position: task.position ?? 0 }))
      .sort((a, b) => (a.position ?? 0) - (b.position ?? 0));
  };

  // Calculate task counts per column for tabs
  const taskCounts = useMemo(() => {
    const counts: Record<string, number> = {};
    for (const column of columns) {
      counts[column.id] = tasks.filter((task) => task.workflowStepId === column.id).length;
    }
    return counts;
  }, [columns, tasks]);

  // On mobile, select the first column with tasks on initial load
  const hasInitializedRef = useRef(false);
  useEffect(() => {
    if (!isMobile || hasInitializedRef.current || columns.length === 0) return;

    const firstColumnWithTasks = columns.findIndex(
      (column) => taskCounts[column.id] > 0
    );

    if (firstColumnWithTasks !== -1 && firstColumnWithTasks !== activeColumnIndex) {
      setActiveColumnIndex(firstColumnWithTasks);
    }
    hasInitializedRef.current = true;
  }, [isMobile, columns, taskCounts, activeColumnIndex, setActiveColumnIndex]);

  // Get current column ID for mobile drop targets
  const currentColumnId = columns[activeColumnIndex]?.id ?? null;

  // Check if we have a board selected (from SSR or client)
  const boardsActiveId = useAppStore((state) => state.boards.activeId);
  const kanbanBoardId = useAppStore((state) => state.kanban.boardId);

  // Show loading state when:
  // 1. isLoading is explicitly true, or
  // 2. We have an active board selected but the kanban data hasn't been hydrated yet
  //    (boards.activeId is set but kanban.boardId is null - waiting for snapshot)
  // 3. isLoading is undefined AND no columns AND no board selected
  //    (still initializing before any SSR data)
  const showLoading =
    isLoading === true ||
    (boardsActiveId && !kanbanBoardId) ||
    (isLoading === undefined && columns.length === 0 && !boardsActiveId);

  const renderEmptyState = () => (
    <div className="h-full rounded-lg border border-dashed border-border/60 flex items-center justify-center text-sm text-muted-foreground mx-4">
      {showLoading ? 'Loading...' : 'No boards available yet.'}
    </div>
  );

  // Mobile layout: Single column with swipeable tabs
  if (isMobile) {
    return (
      <DndContext
        sensors={sensors}
        onDragStart={onDragStart}
        onDragEnd={onDragEnd}
        onDragCancel={onDragCancel}
      >
        <div className="flex-1 min-h-0 flex flex-col">
          {showLoading || columns.length === 0 ? (
            renderEmptyState()
          ) : (
            <>
              <MobileColumnTabs
                columns={columns}
                activeIndex={activeColumnIndex}
                taskCounts={taskCounts}
                onColumnChange={setActiveColumnIndex}
              />
              <SwipeableColumns
                columns={columns}
                tasks={tasks}
                activeIndex={activeColumnIndex}
                onIndexChange={setActiveColumnIndex}
                onPreviewTask={onPreviewTask}
                onOpenTask={onOpenTask}
                onEditTask={onEditTask}
                onDeleteTask={onDeleteTask}
                onMoveTask={onMoveTask}
                showMaximizeButton={showMaximizeButton}
                deletingTaskId={deletingTaskId}
              />
              <MobileDropTargets
                columns={columns}
                currentColumnId={currentColumnId}
                isDragging={!!activeTask}
              />
            </>
          )}
          {/* Safe area spacer for iOS bottom bar */}
          <div className="flex-shrink-0 h-safe" />
        </div>
        {onCreateTask && (
          <MobileFab onClick={onCreateTask} isDragging={!!activeTask} />
        )}
        <DragOverlay dropAnimation={null}>
          {activeTask ? <KanbanCardPreview task={activeTask} /> : null}
        </DragOverlay>
      </DndContext>
    );
  }

  // Tablet layout: Two columns with horizontal scroll
  if (isTablet) {
    return (
      <DndContext
        sensors={sensors}
        onDragStart={onDragStart}
        onDragEnd={onDragEnd}
        onDragCancel={onDragCancel}
      >
        <div className="flex-1 min-h-0 px-4 pb-4">
          {showLoading || columns.length === 0 ? (
            renderEmptyState()
          ) : (
            <div className="flex overflow-x-auto snap-x snap-mandatory gap-2 h-full scrollbar-hide">
              {columns.map((column) => (
                <div
                  key={column.id}
                  className="flex-shrink-0 w-[calc(50%-4px)] snap-start h-full"
                >
                  <KanbanColumn
                    column={column}
                    tasks={getTasksForColumn(column.id)}
                    onPreviewTask={onPreviewTask}
                    onOpenTask={onOpenTask}
                    onEditTask={onEditTask}
                    onDeleteTask={onDeleteTask}
                    onMoveTask={onMoveTask}
                    columns={columns}
                    showMaximizeButton={showMaximizeButton}
                    deletingTaskId={deletingTaskId}
                  />
                </div>
              ))}
            </div>
          )}
        </div>
        <DragOverlay dropAnimation={null}>
          {activeTask ? <KanbanCardPreview task={activeTask} /> : null}
        </DragOverlay>
      </DndContext>
    );
  }

  // Desktop layout: Original grid
  return (
    <DndContext
      sensors={sensors}
      onDragStart={onDragStart}
      onDragEnd={onDragEnd}
      onDragCancel={onDragCancel}
    >
      <div className="flex-1 min-h-0 px-4 pb-4">
        {showLoading || columns.length === 0 ? (
          renderEmptyState()
        ) : (
          <div
            className="grid gap-2 rounded-lg h-full"
            style={{ gridTemplateColumns: `repeat(${columns.length}, minmax(0, 1fr))` }}
          >
            {columns.map((column) => (
              <KanbanColumn
                key={column.id}
                column={column}
                tasks={getTasksForColumn(column.id)}
                onPreviewTask={onPreviewTask}
                onOpenTask={onOpenTask}
                onEditTask={onEditTask}
                onDeleteTask={onDeleteTask}
                onMoveTask={onMoveTask}
                columns={columns}
                showMaximizeButton={showMaximizeButton}
                deletingTaskId={deletingTaskId}
              />
            ))}
          </div>
        )}
      </div>
      <DragOverlay dropAnimation={null}>
        {activeTask ? <KanbanCardPreview task={activeTask} /> : null}
      </DragOverlay>
    </DndContext>
  );
}
