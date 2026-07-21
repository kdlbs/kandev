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
import type { MoveTaskError } from "@/hooks/use-drag-and-drop";
import { useTaskActions } from "@/hooks/use-task-actions";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import { MobileColumnTabs } from "./mobile-column-tabs";
import { SwipeableColumns } from "./swipeable-columns";
import { MobileDropTargets } from "./mobile-drop-targets";
import { getKanbanColumnGridTemplate } from "./kanban-grid-template";
import type { KanbanState } from "@/lib/state/slices/kanban/types";
import type { MobileWorkflowNavigation } from "@/lib/kanban/view-registry";
import { compareTasksByCreatedDesc } from "@/lib/kanban/task-order";
import {
  type KanbanExternalLinkAvailability,
  useKanbanExternalLinkAvailability,
} from "@/components/kanban-external-link-availability";

/**
 * Sentinel step ID used to collect tasks whose workflow_step_id no longer
 * matches any rendered column.  Tasks remapped here are visible as a
 * "Needs Reassignment" fallback column so they are never silently hidden.
 */
export const ORPHAN_STEP_ID = "__kandev_orphan__";

export const ORPHAN_STEP: WorkflowStep = {
  id: ORPHAN_STEP_ID,
  title: "Needs Reassignment",
  color: "#f59e0b",
};

/**
 * The "Needs Reassignment" column is a display-only fallback, not a real
 * workflow step — it must never be offered as a manual move destination
 * (drag-and-drop, "Move to" menus, or Pipeline navigation).
 */
export function isOrphanMoveTarget(targetStepId: string): boolean {
  return targetStepId === ORPHAN_STEP_ID;
}

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
      if (!task || task.workflowStepId === targetStepId || isOrphanMoveTarget(targetStepId)) return;

      const state = store.getState();
      const snapshot = state.kanbanMulti.snapshots[workflowId];
      if (!snapshot) return;

      const targetTasks = snapshot.tasks.filter(
        (t: KanbanState["tasks"][number]) => t.workflowStepId === targetStepId && t.id !== taskId,
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

/**
 * remapOrphanTasks re-keys any task whose workflowStepId matches no step in
 * `stepIds` to `orphanStepId`.  Returns the remapped task list and whether
 * any orphans were found.  Pure function — safe to call outside React.
 */
export function remapOrphanTasks(
  tasks: Task[],
  stepIds: Set<string>,
  orphanStepId: string,
): { tasks: Task[]; hasOrphans: boolean } {
  let hasOrphans = false;
  const remapped = tasks.map((t) => {
    if (!t.workflowStepId || stepIds.has(t.workflowStepId)) return t;
    hasOrphans = true;
    return { ...t, workflowStepId: orphanStepId };
  });
  return { tasks: remapped, hasOrphans };
}

/**
 * useOrphanDisplay remaps tasks with an unknown workflowStepId to the sentinel
 * ORPHAN_STEP so they appear in a visible fallback column instead of being
 * silently dropped from the board.
 *
 * Returns:
 *   displayTasks – all tasks, with orphaned ones keyed to ORPHAN_STEP_ID
 *   displaySteps – original steps plus ORPHAN_STEP when orphans are present
 */
function useOrphanDisplay(
  tasks: Task[],
  steps: WorkflowStep[],
): { displayTasks: Task[]; displaySteps: WorkflowStep[] } {
  return useMemo(() => {
    const stepIds = new Set(steps.map((s) => s.id));
    const { tasks: displayTasks, hasOrphans } = remapOrphanTasks(tasks, stepIds, ORPHAN_STEP_ID);
    const displaySteps = hasOrphans ? [...steps, ORPHAN_STEP] : steps;
    return { displayTasks, displaySteps };
  }, [tasks, steps]);
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
  moveTargetSteps,
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
}: SharedKanbanLayoutProps & {
  activeIndex: number;
  onIndexChange: (index: number) => void;
  activeTask: Task | null;
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
          moveTargetSteps={moveTargetSteps}
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
      <MobileDropTargets
        steps={moveTargetSteps}
        currentStepId={currentStepId}
        isDragging={!!activeTask}
      />
    </div>
  );
}

type SharedKanbanLayoutProps = {
  steps: WorkflowStep[];
  // Real workflow steps only (excludes the synthetic "Needs Reassignment"
  // sentinel) — used wherever a step is offered as a move destination
  // (move menus, drop targets), since that sentinel is display-only.
  moveTargetSteps: WorkflowStep[];
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
};

function TabletKanbanLayout({
  steps,
  moveTargetSteps,
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
}: SharedKanbanLayoutProps) {
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
            steps={moveTargetSteps}
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
  moveTargetSteps,
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
}: SharedKanbanLayoutProps & {
  isCompactDesktop: boolean;
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
          steps={moveTargetSteps}
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

/**
 * Picks the responsive layout to render for the swimlane. Extracted so
 * `SwimlaneKanbanContent` stays under the max-lines-per-function limit.
 */
function renderKanbanLayout({
  isMobile,
  isTablet,
  isCompactDesktop,
  sharedProps,
  activeIndex,
  setActiveIndex,
  activeTask,
  mobileWorkflowNavigation,
}: {
  isMobile: boolean;
  isTablet: boolean;
  isCompactDesktop: boolean;
  sharedProps: SharedKanbanLayoutProps;
  activeIndex: number;
  setActiveIndex: (index: number) => void;
  activeTask: Task | null;
  mobileWorkflowNavigation?: MobileWorkflowNavigation;
}): React.ReactNode {
  if (isMobile) {
    return (
      <MobileKanbanLayout
        {...sharedProps}
        activeIndex={activeIndex}
        onIndexChange={setActiveIndex}
        activeTask={activeTask}
        mobileWorkflowNavigation={mobileWorkflowNavigation}
      />
    );
  }
  if (isTablet) {
    return <TabletKanbanLayout {...sharedProps} />;
  }
  return (
    <div className="h-full overflow-x-auto">
      <DesktopKanbanLayout {...sharedProps} isCompactDesktop={isCompactDesktop} />
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

  // Remap tasks with a dead workflowStepId to the orphan sentinel so they
  // always appear in a visible column rather than being silently dropped.
  const { displayTasks, displaySteps } = useOrphanDisplay(tasks, steps);

  const { activeIndex, setActiveIndex } = useMobileColumnIndex(
    workflowId,
    displaySteps,
    displayTasks,
  );
  const { sensors, handleDragStart, handleDragEnd, handleDragCancel, moveTaskToStep, activeTask } =
    useSwimlaneKanbanDnd({ tasks: displayTasks, workflowId, onMoveError });

  // Memoized so the layout components don't re-render from a fresh props object
  // on every parent render. Declared before the early return to keep hook order
  // stable.
  const sharedProps = useMemo(
    () => ({
      steps: displaySteps,
      moveTargetSteps: steps,
      tasks: displayTasks,
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
      displaySteps,
      steps,
      displayTasks,
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

  if (displaySteps.length === 0 && !isMobile) return null;

  const layoutContent = renderKanbanLayout({
    isMobile,
    isTablet,
    isCompactDesktop,
    sharedProps,
    activeIndex,
    setActiveIndex,
    activeTask,
    mobileWorkflowNavigation,
  });

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
