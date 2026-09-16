"use client";

import { useCallback, useEffect, useMemo, useRef } from "react";
import { KanbanColumn } from "@/components/kanban-column";
import { type Task } from "@/components/kanban-card";
import type { WorkflowStep } from "@/components/kanban-column";
import type { MoveTaskError } from "@/hooks/use-drag-and-drop";
import { useAppStore } from "@/components/state-provider";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import { MobileColumnTabs } from "./mobile-column-tabs";
import { SwipeableColumns } from "./swipeable-columns";
import { KanbanDragSurface } from "./kanban-drag-surface";
import { getDesktopEmptyState } from "./desktop-auto-hidden-empty-state";
import { useOrphanDisplay } from "./swimlane-orphan-display";
export {
  ORPHAN_STEP,
  ORPHAN_STEP_ID,
  isOrphanMoveTarget,
  remapOrphanTasks,
} from "./swimlane-orphan-display";
import { AdaptiveDesktopKanban } from "./adaptive-desktop-kanban";
import type { MobileWorkflowNavigation } from "@/lib/kanban/view-registry";
import { resolveMobileColumnIndex } from "@/lib/kanban/mobile-column-index";
import { pickKanbanColumnComparator } from "@/lib/kanban/task-order";
import { useSwimlaneKanbanPresentationDnd } from "@/hooks/domains/kanban/use-swimlane-kanban-dnd";
import { useKeyboardReorder } from "@/hooks/domains/kanban/use-keyboard-reorder";
import { countAdmittedTasks } from "@/lib/kanban/wip-limit";
import { areAllEmptyStepsAutoHidden } from "@/lib/kanban/auto-hide-empty-columns";
import {
  type SharedKanbanLayoutProps,
  useSharedKanbanLayoutProps,
} from "@/hooks/domains/kanban/use-shared-kanban-layout-props";
import { cn } from "@kandev/ui/lib/utils";
import { useKanbanExternalLinkAvailability } from "@/components/kanban-external-link-availability";
import { useTranslation } from "react-i18next";
import { useCompactSwimlaneHeight } from "@/hooks/domains/kanban/use-compact-swimlane-height";
import { useKanbanOverflow } from "@/hooks/domains/kanban/use-kanban-overflow";
import { KanbanOverflowFades } from "./kanban-overflow-fades";

export type SwimlaneKanbanContentProps = {
  compactHeight?: boolean;
  workflowId: string;
  steps: WorkflowStep[];
  moveTargetSteps: WorkflowStep[];
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

function useMobileColumnIndex(workflowId: string, steps: WorkflowStep[], tasks: Task[]) {
  const storedStepId = useAppStore(
    (state) => state.mobileKanban.activeStepIdByWorkflowId[workflowId],
  );
  const setMobileKanbanActiveStep = useAppStore((state) => state.setMobileKanbanActiveStep);

  const activeIndex = useMemo(
    () => resolveMobileColumnIndex(steps, tasks, storedStepId),
    [steps, tasks, storedStepId],
  );
  const activeStepId = steps[activeIndex]?.id;

  useEffect(() => {
    if (!activeStepId || activeStepId === storedStepId) return;
    setMobileKanbanActiveStep(workflowId, activeStepId);
  }, [activeStepId, storedStepId, workflowId, setMobileKanbanActiveStep]);

  const setActiveIndex = useCallback(
    (index: number) => {
      const stepId = steps[index]?.id;
      if (!stepId) return;
      setMobileKanbanActiveStep(workflowId, stepId);
    },
    [steps, workflowId, setMobileKanbanActiveStep],
  );

  return { activeIndex, setActiveIndex };
}

function useTasksByStep(tasks: Task[]) {
  const kanbanSort = useAppStore((state) => state.userSettings.kanbanSort);
  const comparator = pickKanbanColumnComparator(kanbanSort);
  return useCallback(
    (stepId: string) => tasks.filter((t) => t.workflowStepId === stepId).sort(comparator),
    [tasks, comparator],
  );
}

function MobileKanbanEmptyState({ allStepsAutoHidden }: { allStepsAutoHidden: boolean }) {
  const { t } = useTranslation();
  return (
    <div
      className="mx-4 my-3 flex flex-1 items-center justify-center rounded-xl border border-dashed border-border/70 px-6 text-center text-sm text-muted-foreground"
      data-testid={allStepsAutoHidden ? "kanban-auto-hidden-empty-state" : "mobile-kanban-no-steps"}
    >
      {allStepsAutoHidden
        ? t("kanban:allEmptyStepsAutoHidden")
        : t("kanban:noStepsConfiguredChooseAnotherWorkflow")}
    </div>
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
  mobileWorkflowNavigation?: MobileWorkflowNavigation;
}) {
  const taskCounts = useMemo(() => {
    const counts: Record<string, number> = {};
    for (const step of steps) {
      counts[step.id] = countAdmittedTasks(tasks.filter((task) => task.workflowStepId === step.id));
    }
    return counts;
  }, [steps, tasks]);

  const allStepsAutoHidden = areAllEmptyStepsAutoHidden(steps, moveTargetSteps);

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
          allStepsAutoHidden={allStepsAutoHidden}
        />
      )}
      {steps.length === 0 ? (
        <MobileKanbanEmptyState allStepsAutoHidden={allStepsAutoHidden} />
      ) : (
        <SwipeableColumns
          steps={steps}
          presentation="mobile"
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
    </div>
  );
}

function TabletKanbanLayout({
  columnHeight,
  onNaturalHeightChange,
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
  temporaryStepIds,
  activeTaskId,
  keyboardDraft,
  onCardKeyDown,
}: SharedKanbanLayoutProps) {
  const getTasksForStep = useTasksByStep(tasks);
  const scrollRef = useRef<HTMLDivElement>(null);
  const { t } = useTranslation();
  const overflow = useKanbanOverflow(scrollRef, {
    axis: "horizontal",
    revision: steps.length,
  });

  return (
    <div
      className="relative h-full min-h-0"
      data-testid="tablet-kanban-layout"
      style={{ height: columnHeight }}
    >
      <div
        ref={scrollRef}
        aria-label={t("kanban:columns")}
        className="kanban-scroll-region flex h-full min-h-0 gap-2 overflow-x-auto overscroll-y-auto snap-x snap-mandatory"
        data-kanban-scroll-axis="horizontal"
        data-kanban-scroll-active={overflow.isScrolling}
        data-kanban-scroll-left={overflow.canScrollLeft}
        data-kanban-scroll-right={overflow.canScrollRight}
        data-testid="tablet-kanban-scroll-window"
        tabIndex={0}
      >
        {steps.map((step) => (
          <div
            key={step.id}
            data-kanban-step-id={step.id}
            className={cn(
              "h-full min-h-0 w-[calc(50%-4px)] flex-shrink-0 snap-start",
              temporaryStepIds.has(step.id) && "opacity-70",
            )}
          >
            <KanbanColumn
              onNaturalHeightChange={onNaturalHeightChange}
              step={step}
              tasks={getTasksForStep(step.id)}
              presentation="desktop"
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
              activeTaskId={activeTaskId}
              keyboardDraft={keyboardDraft}
              onCardKeyDown={onCardKeyDown}
            />
          </div>
        ))}
      </div>
      <KanbanOverflowFades axis="horizontal" state={overflow} />
    </div>
  );
}

function DesktopKanbanLayout({
  columnHeight,
  onNaturalHeightChange,
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
  temporaryStepIds,
  isDragging,
  activeTaskId,
  keyboardDraft,
  onCardKeyDown,
}: SharedKanbanLayoutProps) {
  const getTasksForStep = useTasksByStep(tasks);

  return (
    <AdaptiveDesktopKanban
      columnHeight={columnHeight}
      steps={steps}
      isDragging={isDragging}
      renderColumn={(step) => (
        <div className={cn("h-full", temporaryStepIds.has(step.id) && "opacity-70")}>
          <KanbanColumn
            onNaturalHeightChange={onNaturalHeightChange}
            step={step}
            tasks={getTasksForStep(step.id)}
            presentation="desktop"
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
            activeTaskId={activeTaskId}
            keyboardDraft={keyboardDraft}
            onCardKeyDown={onCardKeyDown}
          />
        </div>
      )}
    />
  );
}

/**
 * Picks the responsive layout to render for the swimlane. Extracted so
 * `SwimlaneKanbanContent` stays under the max-lines-per-function limit.
 */
function renderKanbanLayout({
  isMobile,
  isTablet,
  sharedProps,
  activeIndex,
  setActiveIndex,
  mobileWorkflowNavigation,
}: {
  isMobile: boolean;
  isTablet: boolean;
  sharedProps: SharedKanbanLayoutProps;
  activeIndex: number;
  setActiveIndex: (index: number) => void;
  mobileWorkflowNavigation?: MobileWorkflowNavigation;
}): React.ReactNode {
  if (isMobile) {
    return (
      <MobileKanbanLayout
        {...sharedProps}
        activeIndex={activeIndex}
        onIndexChange={setActiveIndex}
        mobileWorkflowNavigation={mobileWorkflowNavigation}
      />
    );
  }
  if (isTablet) {
    return <TabletKanbanLayout {...sharedProps} />;
  }
  return <DesktopKanbanLayout {...sharedProps} />;
}

// eslint-disable-next-line max-lines-per-function -- composes compact sizing, keyboard reorder, and responsive layouts.
export function SwimlaneKanbanContent({
  compactHeight = false,
  workflowId,
  steps,
  moveTargetSteps,
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
  const { isMobile, isTablet } = useResponsiveBreakpoint();
  const activeWorkspaceId = useAppStore((state) => state.workspaces.activeId);
  const externalLinkAvailability = useKanbanExternalLinkAvailability(activeWorkspaceId);

  const { displayTasks, displaySteps } = useOrphanDisplay(tasks, steps);

  const { activeIndex, setActiveIndex } = useMobileColumnIndex(
    workflowId,
    displaySteps,
    displayTasks,
  );
  const drag = useSwimlaneKanbanPresentationDnd({
    tasks: displayTasks,
    workflowId,
    onMoveError,
    displaySteps,
    moveTargetSteps,
    isMobile,
  });
  const keyboard = useKeyboardReorder(workflowId, displayTasks);
  const sizing = useCompactSwimlaneHeight(
    compactHeight && !isMobile,
    displaySteps,
    !!drag.activeTask,
  );
  const sharedProps = useSharedKanbanLayoutProps({
    ...sizing,
    drag,
    moveTargetSteps,
    displayTasks,
    onPreviewTask,
    onOpenTask,
    onEditTask,
    onDeleteTask,
    onArchiveTask,
    showMaximizeButton,
    deletingTaskId,
    archivingTaskId,
    selectedIds,
    onToggleSelect,
    onSelectRange,
    isMultiSelectMode,
    externalLinkAvailability,
    keyboardDraft:
      keyboard.pickedUpTaskId && keyboard.draftStepId && keyboard.draftBand && keyboard.draftOrder
        ? {
            taskId: keyboard.pickedUpTaskId,
            stepId: keyboard.draftStepId,
            band: keyboard.draftBand,
            order: keyboard.draftOrder,
          }
        : null,
    onCardKeyDown: keyboard.handleKeyDown,
  });

  const desktopEmptyState = getDesktopEmptyState(isMobile, displaySteps, moveTargetSteps);
  if (desktopEmptyState !== undefined) return desktopEmptyState;

  const layoutContent = renderKanbanLayout({
    isMobile,
    isTablet,
    sharedProps,
    activeIndex,
    setActiveIndex,
    mobileWorkflowNavigation,
  });

  return (
    <>
      <div
        role="status"
        aria-live="polite"
        className="sr-only"
        data-testid="kanban-reorder-announcement"
      >
        {keyboard.announcement}
      </div>
      <KanbanDragSurface
        sensors={drag.sensors}
        onDragStart={drag.handleDragStart}
        onDragEnd={drag.handleDragEnd}
        onDragCancel={drag.handleDragCancel}
        layoutContent={layoutContent}
        activeTask={drag.activeTask}
        boardRef={drag.boardRef}
      />
    </>
  );
}
