"use client";

import { useCallback, useMemo, useState } from "react";
import {
  DndContext,
  DragEndEvent,
  DragOverlay,
  DragStartEvent,
  PointerSensor,
  TouchSensor,
  useSensor,
  useSensors,
} from "@dnd-kit/core";
import { KanbanColumn } from "@/components/kanban-column";
import { KanbanCardPreview, type Task } from "@/components/kanban-card";
import type { WorkflowStep } from "@/components/kanban-column";
import type { MoveTaskError } from "@/hooks/use-drag-and-drop";
import { useTaskActions } from "@/hooks/use-task-actions";
import { useAppStoreApi } from "@/components/state-provider";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import { MobileColumnTabs } from "./mobile-column-tabs";
import { SwipeableColumns } from "./swipeable-columns";
import { MobileDropTargets } from "./mobile-drop-targets";
import type { KanbanState } from "@/lib/state/slices/kanban/types";

export type SwimlaneKanbanContentProps = {
  workflowId: string;
  steps: WorkflowStep[];
  tasks: Task[];
  onPreviewTask: (task: Task) => void;
  onOpenTask: (task: Task) => void;
  onEditTask: (task: Task) => void;
  onDeleteTask: (task: Task) => void;
  onMoveError?: (error: MoveTaskError) => void;
  deletingTaskId?: string | null;
  showMaximizeButton?: boolean;
};

type SwimlaneKanbanDndOptions = {
  tasks: Task[];
  workflowId: string;
  useTouchSensors: boolean;
  onMoveError?: (error: MoveTaskError) => void;
};

function useSwimlaneKanbanDnd({
  tasks,
  workflowId,
  useTouchSensors,
  onMoveError,
}: SwimlaneKanbanDndOptions) {
  const store = useAppStoreApi();
  const { moveTaskById } = useTaskActions();
  const [activeTaskId, setActiveTaskId] = useState<string | null>(null);

  const sensors = useSensors(
    useSensor(PointerSensor, { activationConstraint: { distance: 8 } }),
    useSensor(TouchSensor, {
      activationConstraint: { delay: 250, tolerance: 5 },
    }),
  );

  const handleDragStart = useCallback((event: DragStartEvent) => {
    setActiveTaskId(event.active.id as string);
  }, []);

  const handleDragEnd = useCallback(
    async (event: DragEndEvent) => {
      const { active, over } = event;
      setActiveTaskId(null);
      if (!over) return;

      const taskId = active.id as string;
      const targetStepId = over.id as string;
      const task = tasks.find((t) => t.id === taskId);
      if (!task || task.workflowStepId === targetStepId) return;

      const state = store.getState();
      const snapshot = state.kanbanMulti.snapshots[workflowId];
      if (!snapshot) return;

      const targetTasks = snapshot.tasks
        .filter(
          (t: KanbanState["tasks"][number]) => t.workflowStepId === targetStepId && t.id !== taskId,
        )
        .sort(
          (a: KanbanState["tasks"][number], b: KanbanState["tasks"][number]) =>
            a.position - b.position,
        );
      const nextPosition = targetTasks.length;
      const originalTasks = snapshot.tasks;

      state.setWorkflowSnapshot(workflowId, {
        ...snapshot,
        tasks: snapshot.tasks.map((t: KanbanState["tasks"][number]) =>
          t.id === taskId ? { ...t, workflowStepId: targetStepId, position: nextPosition } : t,
        ),
      });

      try {
        await moveTaskById(taskId, {
          workflow_id: workflowId,
          workflow_step_id: targetStepId,
          position: nextPosition,
        });
      } catch (error) {
        const currentSnapshot = store.getState().kanbanMulti.snapshots[workflowId];
        if (currentSnapshot) {
          store
            .getState()
            .setWorkflowSnapshot(workflowId, { ...currentSnapshot, tasks: originalTasks });
        }
        const message = error instanceof Error ? error.message : "Failed to move task";
        onMoveError?.({ message, taskId, sessionId: task.primarySessionId ?? null });
      }
    },
    [tasks, workflowId, store, moveTaskById, onMoveError],
  );

  const handleDragCancel = useCallback(() => {
    setActiveTaskId(null);
  }, []);

  const moveTaskToStep = useCallback(
    async (task: Task, targetStepId: string) => {
      if (task.workflowStepId === targetStepId) return;
      await handleDragEnd({ active: { id: task.id }, over: { id: targetStepId } } as DragEndEvent);
    },
    [handleDragEnd],
  );

  const activeTask = useMemo(
    () => tasks.find((t) => t.id === activeTaskId) ?? null,
    [tasks, activeTaskId],
  );

  // Touch-only sensors for mobile/tablet (PointerSensor conflicts with touch scroll)
  const touchSensors = useSensors(
    useSensor(TouchSensor, {
      activationConstraint: { delay: 250, tolerance: 5 },
    }),
  );

  return {
    sensors: useTouchSensors ? touchSensors : sensors,
    handleDragStart,
    handleDragEnd,
    handleDragCancel,
    moveTaskToStep,
    activeTask,
  };
}

function getInitialColumnIndex(steps: WorkflowStep[], tasks: Task[]): number {
  if (steps.length === 0) return 0;
  const idx = steps.findIndex((step) => tasks.some((t) => t.workflowStepId === step.id));
  return idx !== -1 ? idx : 0;
}

function useMobileColumnIndex(steps: WorkflowStep[], tasks: Task[]) {
  const [rawIndex, setActiveIndex] = useState(() => getInitialColumnIndex(steps, tasks));

  // Derive clamped index — avoids calling setState in an effect
  const activeIndex = useMemo(() => {
    if (steps.length === 0) return 0;
    if (rawIndex >= steps.length) return getInitialColumnIndex(steps, tasks);
    return rawIndex;
  }, [steps, tasks, rawIndex]);

  return { activeIndex, setActiveIndex };
}

function useTasksByStep(tasks: Task[]) {
  return useCallback(
    (stepId: string) =>
      tasks
        .filter((t) => t.workflowStepId === stepId)
        .sort((a, b) => (a.position ?? 0) - (b.position ?? 0)),
    [tasks],
  );
}

function MobileKanbanLayout({
  steps,
  tasks,
  activeIndex,
  onIndexChange,
  onPreviewTask,
  onOpenTask,
  onEditTask,
  onDeleteTask,
  moveTaskToStep,
  activeTask,
  showMaximizeButton,
  deletingTaskId,
}: {
  steps: WorkflowStep[];
  tasks: Task[];
  activeIndex: number;
  onIndexChange: (index: number) => void;
  onPreviewTask: (task: Task) => void;
  onOpenTask: (task: Task) => void;
  onEditTask: (task: Task) => void;
  onDeleteTask: (task: Task) => void;
  moveTaskToStep: (task: Task, targetStepId: string) => Promise<void>;
  activeTask: Task | null;
  showMaximizeButton?: boolean;
  deletingTaskId?: string | null;
}) {
  const taskCounts = useMemo(() => {
    const counts: Record<string, number> = {};
    for (const step of steps) {
      counts[step.id] = tasks.filter((t) => t.workflowStepId === step.id).length;
    }
    return counts;
  }, [steps, tasks]);

  const currentStepId = steps[activeIndex]?.id ?? null;

  return (
    <div className="flex flex-col min-h-0" data-testid="mobile-kanban-layout">
      <MobileColumnTabs
        steps={steps}
        activeIndex={activeIndex}
        taskCounts={taskCounts}
        onColumnChange={onIndexChange}
      />
      <SwipeableColumns
        steps={steps}
        tasks={tasks}
        activeIndex={activeIndex}
        onIndexChange={onIndexChange}
        onPreviewTask={onPreviewTask}
        onOpenTask={onOpenTask}
        onEditTask={onEditTask}
        onDeleteTask={onDeleteTask}
        onMoveTask={moveTaskToStep}
        showMaximizeButton={showMaximizeButton}
        deletingTaskId={deletingTaskId}
      />
      <MobileDropTargets steps={steps} currentStepId={currentStepId} isDragging={!!activeTask} />
    </div>
  );
}

function TabletKanbanLayout({
  steps,
  tasks,
  onPreviewTask,
  onOpenTask,
  onEditTask,
  onDeleteTask,
  moveTaskToStep,
  showMaximizeButton,
  deletingTaskId,
}: {
  steps: WorkflowStep[];
  tasks: Task[];
  onPreviewTask: (task: Task) => void;
  onOpenTask: (task: Task) => void;
  onEditTask: (task: Task) => void;
  onDeleteTask: (task: Task) => void;
  moveTaskToStep: (task: Task, targetStepId: string) => Promise<void>;
  showMaximizeButton?: boolean;
  deletingTaskId?: string | null;
}) {
  const getTasksForStep = useTasksByStep(tasks);

  return (
    <div
      className="flex overflow-x-auto snap-x snap-mandatory gap-2 h-full scrollbar-hide"
      data-testid="tablet-kanban-layout"
    >
      {steps.map((step) => (
        <div key={step.id} className="flex-shrink-0 w-[calc(50%-4px)] snap-start h-full">
          <KanbanColumn
            step={step}
            tasks={getTasksForStep(step.id)}
            onPreviewTask={onPreviewTask}
            onOpenTask={onOpenTask}
            onEditTask={onEditTask}
            onDeleteTask={onDeleteTask}
            onMoveTask={moveTaskToStep}
            steps={steps}
            showMaximizeButton={showMaximizeButton}
            deletingTaskId={deletingTaskId}
          />
        </div>
      ))}
    </div>
  );
}

function DesktopKanbanLayout({
  steps,
  tasks,
  onPreviewTask,
  onOpenTask,
  onEditTask,
  onDeleteTask,
  moveTaskToStep,
  showMaximizeButton,
  deletingTaskId,
}: {
  steps: WorkflowStep[];
  tasks: Task[];
  onPreviewTask: (task: Task) => void;
  onOpenTask: (task: Task) => void;
  onEditTask: (task: Task) => void;
  onDeleteTask: (task: Task) => void;
  moveTaskToStep: (task: Task, targetStepId: string) => Promise<void>;
  showMaximizeButton?: boolean;
  deletingTaskId?: string | null;
}) {
  const getTasksForStep = useTasksByStep(tasks);

  return (
    <div
      className="grid gap-0"
      style={{ gridTemplateColumns: `repeat(${steps.length}, minmax(0, 1fr))` }}
    >
      {steps.map((step) => (
        <KanbanColumn
          key={step.id}
          step={step}
          tasks={getTasksForStep(step.id)}
          onPreviewTask={onPreviewTask}
          onOpenTask={onOpenTask}
          onEditTask={onEditTask}
          onDeleteTask={onDeleteTask}
          onMoveTask={moveTaskToStep}
          steps={steps}
          deletingTaskId={deletingTaskId}
          showMaximizeButton={showMaximizeButton}
        />
      ))}
    </div>
  );
}

export function SwimlaneKanbanContent({
  workflowId,
  steps,
  tasks,
  onPreviewTask,
  onOpenTask,
  onEditTask,
  onDeleteTask,
  onMoveError,
  deletingTaskId,
  showMaximizeButton,
}: SwimlaneKanbanContentProps) {
  const { isMobile, isTablet } = useResponsiveBreakpoint();
  const { activeIndex, setActiveIndex } = useMobileColumnIndex(steps, tasks);
  const { sensors, handleDragStart, handleDragEnd, handleDragCancel, moveTaskToStep, activeTask } =
    useSwimlaneKanbanDnd({ tasks, workflowId, useTouchSensors: isMobile || isTablet, onMoveError });

  if (steps.length === 0) return null;

  const sharedProps = {
    steps,
    tasks,
    onPreviewTask,
    onOpenTask,
    onEditTask,
    onDeleteTask,
    moveTaskToStep,
    showMaximizeButton,
    deletingTaskId,
  };

  let layoutContent: React.ReactNode;
  if (isMobile) {
    layoutContent = (
      <MobileKanbanLayout
        {...sharedProps}
        activeIndex={activeIndex}
        onIndexChange={setActiveIndex}
        activeTask={activeTask}
      />
    );
  } else if (isTablet) {
    layoutContent = <TabletKanbanLayout {...sharedProps} />;
  } else {
    layoutContent = <DesktopKanbanLayout {...sharedProps} />;
  }

  return (
    <DndContext
      sensors={sensors}
      onDragStart={handleDragStart}
      onDragEnd={handleDragEnd}
      onDragCancel={handleDragCancel}
    >
      {layoutContent}
      <DragOverlay dropAnimation={null}>
        {activeTask ? <KanbanCardPreview task={activeTask} /> : null}
      </DragOverlay>
    </DndContext>
  );
}
