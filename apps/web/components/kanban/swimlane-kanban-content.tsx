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
import { type Task } from "@/components/kanban-card";
import { KanbanCardPreview } from "@/components/kanban-card-preview";
import type { WorkflowStep } from "@/components/kanban-column";
import { useAppStore } from "@/components/state-provider";
import { useSwimlaneMove } from "@/hooks/domains/kanban/use-swimlane-move";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import { MobileColumnTabs } from "./mobile-column-tabs";
import { SwipeableColumns } from "./swipeable-columns";
import { MobileDropTargets } from "./mobile-drop-targets";
import { getKanbanColumnGridTemplate } from "./kanban-grid-template";
import type { KanbanState } from "@/lib/state/slices/kanban/types";
import type { MobileWorkflowNavigation } from "@/lib/kanban/view-registry";
import type { MoveTaskError } from "@/lib/kanban/move-task-error";
import { compareTasksByCreatedDesc } from "@/lib/kanban/task-order";
import {
  type KanbanExternalLinkAvailability,
  useKanbanExternalLinkAvailability,
} from "@/components/kanban-external-link-availability";

export type SwimlaneKanbanContentProps = {
  workflowId: string;
  steps: WorkflowStep[];
  tasks: Task[];
  onPreviewTask: (task: Task) => void;
  onOpenTask: (task: Task) => void;
  onEditTask: (task: Task) => void;
  onDeleteTask: (task: Task) => void;
  onArchiveTask?: (task: Task) => void;
  onMoveError?: (error: MoveTaskError) => void;
  deletingTaskId?: string | null;
  archivingTaskId?: string | null;
  showMaximizeButton?: boolean;
  selectedIds?: Set<string>;
  onToggleSelect?: (taskId: string) => void;
  onSelectRange?: (taskId: string, orderedIds: string[]) => void;
  isMultiSelectMode?: boolean;
  mobileWorkflowNavigation?: MobileWorkflowNavigation;
};

type SwimlaneKanbanDndOptions = {
  tasks: Task[];
  workflowId: string;
  onMoveError?: (error: MoveTaskError) => void;
};

function useSwimlaneKanbanDnd({ tasks, workflowId, onMoveError }: SwimlaneKanbanDndOptions) {
  const { moveTask } = useSwimlaneMove(workflowId, { onMoveError });
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
      await moveTask(task, targetStepId);
    },
    [tasks, moveTask],
  );

  const handleDragCancel = useCallback(() => {
    setActiveTaskId(null);
  }, []);

  const moveTaskToStep = useCallback(
    async (task: Task, targetStepId: string) => {
      if (task.workflowStepId === targetStepId) return;
      await moveTask(task, targetStepId);
    },
    [moveTask],
  );

  const activeTask = useMemo(
    () => tasks.find((t) => t.id === activeTaskId) ?? null,
    [tasks, activeTaskId],
  );

  return {
    sensors,
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

function useMobileColumnIndex(workflowId: string, steps: WorkflowStep[], tasks: Task[]) {
  const [selection, setSelection] = useState(() => ({
    workflowId,
    index: getInitialColumnIndex(steps, tasks),
  }));

  // Derive clamped index — avoids calling setState in an effect
  const activeIndex = useMemo(() => {
    if (steps.length === 0) return 0;
    if (selection.workflowId !== workflowId || selection.index >= steps.length) {
      return getInitialColumnIndex(steps, tasks);
    }
    return selection.index;
  }, [steps, tasks, selection, workflowId]);
  const setActiveIndex = useCallback(
    (index: number) => setSelection({ workflowId, index }),
    [workflowId],
  );

  return { activeIndex, setActiveIndex };
}

function useTasksByStep(tasks: Task[]) {
  return useCallback(
    (stepId: string) =>
      tasks.filter((t) => t.workflowStepId === stepId).sort(compareTasksByCreatedDesc),
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
  onArchiveTask,
  moveTaskToStep,
  activeTask,
  showMaximizeButton,
  deletingTaskId,
  archivingTaskId,
  selectedIds,
  onToggleSelect,
  onSelectRange,
  isMultiSelectMode,
  externalLinkAvailability,
  mobileWorkflowNavigation,
}: {
  steps: WorkflowStep[];
  tasks: Task[];
  activeIndex: number;
  onIndexChange: (index: number) => void;
  onPreviewTask: (task: Task) => void;
  onOpenTask: (task: Task) => void;
  onEditTask: (task: Task) => void;
  onDeleteTask: (task: Task) => void;
  onArchiveTask?: (task: Task) => void;
  moveTaskToStep: (task: Task, targetStepId: string) => Promise<void>;
  activeTask: Task | null;
  showMaximizeButton?: boolean;
  deletingTaskId?: string | null;
  archivingTaskId?: string | null;
  selectedIds?: Set<string>;
  onToggleSelect?: (taskId: string) => void;
  onSelectRange?: (taskId: string, orderedIds: string[]) => void;
  isMultiSelectMode?: boolean;
  externalLinkAvailability: KanbanExternalLinkAvailability;
  mobileWorkflowNavigation?: MobileWorkflowNavigation;
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
    <div
      className="flex h-full min-h-0 flex-col overflow-hidden"
      data-testid="mobile-kanban-layout"
    >
      {mobileWorkflowNavigation && (
        <MobileColumnTabs
          steps={steps}
          activeIndex={activeIndex}
          taskCounts={taskCounts}
          onColumnChange={onIndexChange}
          workflowNavigation={mobileWorkflowNavigation}
        />
      )}
      {steps.length === 0 ? (
        <div
          className="mx-4 my-3 flex flex-1 items-center justify-center rounded-xl border border-dashed border-border/70 px-6 text-center text-sm text-muted-foreground"
          data-testid="mobile-kanban-no-steps"
        >
          No steps configured. Choose another workflow or add steps in Settings.
        </div>
      ) : (
        <SwipeableColumns
          steps={steps}
          tasks={tasks}
          activeIndex={activeIndex}
          onIndexChange={onIndexChange}
          onPreviewTask={onPreviewTask}
          onOpenTask={onOpenTask}
          onEditTask={onEditTask}
          onDeleteTask={onDeleteTask}
          onArchiveTask={onArchiveTask}
          onMoveTask={moveTaskToStep}
          showMaximizeButton={showMaximizeButton}
          deletingTaskId={deletingTaskId}
          archivingTaskId={archivingTaskId}
          selectedIds={selectedIds}
          onToggleSelect={onToggleSelect}
          onSelectRange={onSelectRange}
          isMultiSelectMode={isMultiSelectMode}
          externalLinkAvailability={externalLinkAvailability}
        />
      )}
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
  onArchiveTask,
  moveTaskToStep,
  showMaximizeButton,
  deletingTaskId,
  archivingTaskId,
  selectedIds,
  onToggleSelect,
  onSelectRange,
  isMultiSelectMode,
  externalLinkAvailability,
}: {
  steps: WorkflowStep[];
  tasks: Task[];
  onPreviewTask: (task: Task) => void;
  onOpenTask: (task: Task) => void;
  onEditTask: (task: Task) => void;
  onDeleteTask: (task: Task) => void;
  onArchiveTask?: (task: Task) => void;
  moveTaskToStep: (task: Task, targetStepId: string) => Promise<void>;
  showMaximizeButton?: boolean;
  deletingTaskId?: string | null;
  archivingTaskId?: string | null;
  selectedIds?: Set<string>;
  onToggleSelect?: (taskId: string) => void;
  onSelectRange?: (taskId: string, orderedIds: string[]) => void;
  isMultiSelectMode?: boolean;
  externalLinkAvailability: KanbanExternalLinkAvailability;
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
            onArchiveTask={onArchiveTask}
            onMoveTask={moveTaskToStep}
            steps={steps}
            showMaximizeButton={showMaximizeButton}
            deletingTaskId={deletingTaskId}
            archivingTaskId={archivingTaskId}
            selectedIds={selectedIds}
            onToggleSelect={onToggleSelect}
            onSelectRange={onSelectRange}
            isMultiSelectMode={isMultiSelectMode}
            externalLinkAvailability={externalLinkAvailability}
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
  onArchiveTask,
  moveTaskToStep,
  showMaximizeButton,
  deletingTaskId,
  archivingTaskId,
  selectedIds,
  onToggleSelect,
  onSelectRange,
  isMultiSelectMode,
  isCompactDesktop,
  externalLinkAvailability,
}: {
  steps: WorkflowStep[];
  tasks: Task[];
  onPreviewTask: (task: Task) => void;
  onOpenTask: (task: Task) => void;
  onEditTask: (task: Task) => void;
  onDeleteTask: (task: Task) => void;
  onArchiveTask?: (task: Task) => void;
  moveTaskToStep: (task: Task, targetStepId: string) => Promise<void>;
  showMaximizeButton?: boolean;
  deletingTaskId?: string | null;
  archivingTaskId?: string | null;
  selectedIds?: Set<string>;
  onToggleSelect?: (taskId: string) => void;
  onSelectRange?: (taskId: string, orderedIds: string[]) => void;
  isMultiSelectMode?: boolean;
  isCompactDesktop: boolean;
  externalLinkAvailability: KanbanExternalLinkAvailability;
}) {
  const getTasksForStep = useTasksByStep(tasks);

  return (
    <div
      data-testid="desktop-kanban-layout"
      className="grid min-w-full gap-0"
      style={{
        gridTemplateColumns: getKanbanColumnGridTemplate(steps.length, isCompactDesktop),
      }}
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
          onArchiveTask={onArchiveTask}
          onMoveTask={moveTaskToStep}
          steps={steps}
          deletingTaskId={deletingTaskId}
          archivingTaskId={archivingTaskId}
          showMaximizeButton={showMaximizeButton}
          selectedIds={selectedIds}
          onToggleSelect={onToggleSelect}
          onSelectRange={onSelectRange}
          isMultiSelectMode={isMultiSelectMode}
          externalLinkAvailability={externalLinkAvailability}
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
  onArchiveTask,
  onMoveError,
  deletingTaskId,
  archivingTaskId,
  showMaximizeButton,
  selectedIds,
  onToggleSelect,
  onSelectRange,
  isMultiSelectMode,
  mobileWorkflowNavigation,
}: SwimlaneKanbanContentProps) {
  const { isMobile, isTablet, isCompactDesktop } = useResponsiveBreakpoint();
  const activeWorkspaceId = useAppStore((state) => state.workspaces.activeId);
  const externalLinkAvailability = useKanbanExternalLinkAvailability(activeWorkspaceId);
  const { activeIndex, setActiveIndex } = useMobileColumnIndex(workflowId, steps, tasks);
  const { sensors, handleDragStart, handleDragEnd, handleDragCancel, moveTaskToStep, activeTask } =
    useSwimlaneKanbanDnd({ tasks, workflowId, onMoveError });

  // Memoized so the layout components don't re-render from a fresh props object
  // on every parent render. Declared before the early return to keep hook order
  // stable.
  const sharedProps = useMemo(
    () => ({
      steps,
      tasks,
      onPreviewTask,
      onOpenTask,
      onEditTask,
      onDeleteTask,
      onArchiveTask,
      moveTaskToStep,
      showMaximizeButton,
      deletingTaskId,
      archivingTaskId,
      selectedIds,
      onToggleSelect,
      onSelectRange,
      isMultiSelectMode,
      externalLinkAvailability,
    }),
    [
      steps,
      tasks,
      onPreviewTask,
      onOpenTask,
      onEditTask,
      onDeleteTask,
      onArchiveTask,
      moveTaskToStep,
      showMaximizeButton,
      deletingTaskId,
      archivingTaskId,
      selectedIds,
      onToggleSelect,
      onSelectRange,
      isMultiSelectMode,
      externalLinkAvailability,
    ],
  );

  if (steps.length === 0 && !isMobile) return null;

  let layoutContent: React.ReactNode;
  if (isMobile) {
    layoutContent = (
      <MobileKanbanLayout
        {...sharedProps}
        activeIndex={activeIndex}
        onIndexChange={setActiveIndex}
        activeTask={activeTask}
        mobileWorkflowNavigation={mobileWorkflowNavigation}
      />
    );
  } else if (isTablet) {
    layoutContent = <TabletKanbanLayout {...sharedProps} />;
  } else {
    layoutContent = (
      <div className="h-full overflow-x-auto">
        <DesktopKanbanLayout {...sharedProps} isCompactDesktop={isCompactDesktop} />
      </div>
    );
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
