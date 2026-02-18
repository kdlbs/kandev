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
  steps: WorkflowStep[];
  tasks: Task[];
  onPreviewTask: (task: Task) => void;
  onOpenTask: (task: Task) => void;
  onEditTask: (task: Task) => void;
  onDeleteTask: (task: Task) => void;
  onMoveTask?: (task: Task, targetStepId: string) => void;
  onDragStart: (event: DragStartEvent) => void;
  onDragEnd: (event: DragEndEvent) => void;
  onDragCancel: () => void;
  activeTask: Task | null;
  showMaximizeButton?: boolean;
  deletingTaskId?: string | null;
  onCreateTask?: () => void;
  isLoading?: boolean;
};

type ColumnGridProps = Pick<
  KanbanBoardGridProps,
  'steps' | 'tasks' | 'onPreviewTask' | 'onOpenTask' | 'onEditTask' | 'onDeleteTask' | 'onMoveTask' | 'showMaximizeButton' | 'deletingTaskId'
>;

function getTasksForStep(tasks: Task[], stepId: string) {
  return tasks
    .filter((task) => task.workflowStepId === stepId)
    .map((task) => ({ ...task, position: task.position ?? 0 }))
    .sort((a, b) => (a.position ?? 0) - (b.position ?? 0));
}

function EmptyState({ showLoading }: { showLoading: boolean }) {
  return (
    <div className="h-full rounded-lg border border-dashed border-border/60 flex items-center justify-center text-sm text-muted-foreground mx-4">
      {showLoading ? 'Loading...' : 'No workflows available yet.'}
    </div>
  );
}

function MobileLayout({
  steps,
  tasks,
  onPreviewTask,
  onOpenTask,
  onEditTask,
  onDeleteTask,
  onMoveTask,
  showMaximizeButton,
  deletingTaskId,
  showLoading,
  activeTask,
  onCreateTask,
  taskCounts,
  activeColumnIndex,
  setActiveColumnIndex,
  currentStepId,
}: ColumnGridProps & {
  showLoading: boolean;
  activeTask: Task | null;
  onCreateTask?: () => void;
  taskCounts: Record<string, number>;
  activeColumnIndex: number;
  setActiveColumnIndex: (index: number) => void;
  currentStepId: string | null;
}) {
  return (
    <>
      <div className="flex-1 min-h-0 flex flex-col">
        {showLoading || steps.length === 0 ? (
          <EmptyState showLoading={showLoading} />
        ) : (
          <>
            <MobileColumnTabs
              steps={steps}
              activeIndex={activeColumnIndex}
              taskCounts={taskCounts}
              onColumnChange={setActiveColumnIndex}
            />
            <SwipeableColumns
              steps={steps}
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
              steps={steps}
              currentStepId={currentStepId}
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
    </>
  );
}

function TabletLayout({
  steps,
  tasks,
  onPreviewTask,
  onOpenTask,
  onEditTask,
  onDeleteTask,
  onMoveTask,
  showMaximizeButton,
  deletingTaskId,
  showLoading,
  activeTask,
}: ColumnGridProps & { showLoading: boolean; activeTask: Task | null }) {
  return (
    <>
      <div className="flex-1 min-h-0 px-4 pb-4">
        {showLoading || steps.length === 0 ? (
          <EmptyState showLoading={showLoading} />
        ) : (
          <div className="flex overflow-x-auto snap-x snap-mandatory gap-2 h-full scrollbar-hide">
            {steps.map((step) => (
              <div
                key={step.id}
                className="flex-shrink-0 w-[calc(50%-4px)] snap-start h-full"
              >
                <KanbanColumn
                  step={step}
                  tasks={getTasksForStep(tasks, step.id)}
                  onPreviewTask={onPreviewTask}
                  onOpenTask={onOpenTask}
                  onEditTask={onEditTask}
                  onDeleteTask={onDeleteTask}
                  onMoveTask={onMoveTask}
                  steps={steps}
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
    </>
  );
}

function DesktopLayout({
  steps,
  tasks,
  onPreviewTask,
  onOpenTask,
  onEditTask,
  onDeleteTask,
  onMoveTask,
  showMaximizeButton,
  deletingTaskId,
  showLoading,
  activeTask,
}: ColumnGridProps & { showLoading: boolean; activeTask: Task | null }) {
  return (
    <>
      <div className="flex-1 min-h-0 px-4 pb-4">
        {showLoading || steps.length === 0 ? (
          <EmptyState showLoading={showLoading} />
        ) : (
          <div
            className="grid gap-2 rounded-lg h-full"
            style={{ gridTemplateColumns: `repeat(${steps.length}, minmax(0, 1fr))` }}
          >
            {steps.map((step) => (
              <KanbanColumn
                key={step.id}
                step={step}
                tasks={getTasksForStep(tasks, step.id)}
                onPreviewTask={onPreviewTask}
                onOpenTask={onOpenTask}
                onEditTask={onEditTask}
                onDeleteTask={onDeleteTask}
                onMoveTask={onMoveTask}
                steps={steps}
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
    </>
  );
}

function useShowLoading(isLoading: boolean | undefined, stepsLength: number) {
  const workflowsActiveId = useAppStore((state) => state.workflows.activeId);
  const kanbanWorkflowId = useAppStore((state) => state.kanban.workflowId);

  return (
    isLoading === true ||
    (workflowsActiveId && !kanbanWorkflowId) ||
    (isLoading === undefined && stepsLength === 0 && !workflowsActiveId)
  );
}

export function KanbanBoardGrid({
  steps,
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

  // Calculate task counts per step for tabs
  const taskCounts = useMemo(() => {
    const counts: Record<string, number> = {};
    for (const step of steps) {
      counts[step.id] = tasks.filter((task) => task.workflowStepId === step.id).length;
    }
    return counts;
  }, [steps, tasks]);

  // On mobile, select the first step with tasks on initial load
  const hasInitializedRef = useRef(false);
  useEffect(() => {
    if (!isMobile || hasInitializedRef.current || steps.length === 0) return;

    const firstStepWithTasks = steps.findIndex(
      (step) => taskCounts[step.id] > 0
    );

    if (firstStepWithTasks !== -1 && firstStepWithTasks !== activeColumnIndex) {
      setActiveColumnIndex(firstStepWithTasks);
    }
    hasInitializedRef.current = true;
  }, [isMobile, steps, taskCounts, activeColumnIndex, setActiveColumnIndex]);

  // Get current step ID for mobile drop targets
  const currentStepId = steps[activeColumnIndex]?.id ?? null;

  const showLoading = useShowLoading(isLoading, steps.length);

  const columnProps: ColumnGridProps = {
    steps, tasks, onPreviewTask, onOpenTask, onEditTask, onDeleteTask, onMoveTask, showMaximizeButton, deletingTaskId,
  };

  let layoutContent: React.ReactNode;
  if (isMobile) {
    layoutContent = (
      <MobileLayout
        {...columnProps}
        showLoading={!!showLoading}
        activeTask={activeTask}
        onCreateTask={onCreateTask}
        taskCounts={taskCounts}
        activeColumnIndex={activeColumnIndex}
        setActiveColumnIndex={setActiveColumnIndex}
        currentStepId={currentStepId}
      />
    );
  } else if (isTablet) {
    layoutContent = <TabletLayout {...columnProps} showLoading={!!showLoading} activeTask={activeTask} />;
  } else {
    layoutContent = <DesktopLayout {...columnProps} showLoading={!!showLoading} activeTask={activeTask} />;
  }

  return (
    <DndContext
      sensors={sensors}
      onDragStart={onDragStart}
      onDragEnd={onDragEnd}
      onDragCancel={onDragCancel}
    >
      {layoutContent}
    </DndContext>
  );
}
