'use client';

import { useEffect, useCallback } from 'react';
import useEmblaCarousel from 'embla-carousel-react';
import { KanbanColumn, WorkflowStep } from '../kanban-column';
import { Task } from '../kanban-card';

type SwipeableColumnsProps = {
  columns: WorkflowStep[];
  tasks: Task[];
  activeIndex: number;
  onIndexChange: (index: number) => void;
  onPreviewTask: (task: Task) => void;
  onOpenTask: (task: Task) => void;
  onEditTask: (task: Task) => void;
  onDeleteTask: (task: Task) => void;
  onMoveTask?: (task: Task, targetColumnId: string) => void;
  showMaximizeButton?: boolean;
  deletingTaskId?: string | null;
};

export function SwipeableColumns({
  columns,
  tasks,
  activeIndex,
  onIndexChange,
  onPreviewTask,
  onOpenTask,
  onEditTask,
  onDeleteTask,
  onMoveTask,
  showMaximizeButton,
  deletingTaskId,
}: SwipeableColumnsProps) {
  const [emblaRef, emblaApi] = useEmblaCarousel({
    align: 'start',
    containScroll: 'trimSnaps',
    watchDrag: true,
  });

  const getTasksForColumn = useCallback(
    (columnId: string) => {
      return tasks
        .filter((task) => task.workflowStepId === columnId)
        .map((task) => ({ ...task, position: task.position ?? 0 }))
        .sort((a, b) => (a.position ?? 0) - (b.position ?? 0));
    },
    [tasks]
  );

  // Sync carousel position with external activeIndex
  useEffect(() => {
    if (emblaApi && emblaApi.selectedScrollSnap() !== activeIndex) {
      emblaApi.scrollTo(activeIndex);
    }
  }, [emblaApi, activeIndex]);

  // Update external activeIndex when user swipes
  useEffect(() => {
    if (!emblaApi) return;

    const onSelect = () => {
      const selectedIndex = emblaApi.selectedScrollSnap();
      if (selectedIndex !== activeIndex) {
        onIndexChange(selectedIndex);
      }
    };

    emblaApi.on('select', onSelect);
    return () => {
      emblaApi.off('select', onSelect);
    };
  }, [emblaApi, activeIndex, onIndexChange]);

  // Use explicit height calculation for mobile since flex height inheritance doesn't work reliably
  // 100dvh - header (~56px) - tabs (~44px) - safe area
  return (
    <div
      className="flex-1 min-h-0 overflow-hidden"
      ref={emblaRef}
      style={{ height: 'calc(100dvh - 100px - env(safe-area-inset-bottom, 0px))' }}
    >
      <div className="flex h-full touch-pan-y">
        {columns.map((column) => (
          <div
            key={column.id}
            className="flex-shrink-0 w-full h-full min-w-0 px-4 py-2 flex flex-col"
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
              hideHeader
            />
          </div>
        ))}
      </div>
    </div>
  );
}
