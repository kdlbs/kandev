"use client";

import {
  memo,
  type ComponentType,
  type HTMLAttributes,
  useCallback,
  useEffect,
  useMemo,
} from "react";
import {
  DndContext,
  closestCenter,
  type DragEndEvent,
  PointerSensor,
  useSensor,
  useSensors,
} from "@dnd-kit/core";
import {
  SortableContext,
  verticalListSortingStrategy,
  arrayMove,
  useSortable,
} from "@dnd-kit/sortable";
import { CSS } from "@dnd-kit/utilities";
import { useAppStore } from "@/components/state-provider";
import { useSwimlaneCollapse } from "@/hooks/domains/kanban/use-swimlane-collapse";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import { selectVisibleWorkflows } from "@/lib/kanban/workflow-swimlanes";
import { reorderWorkflows } from "@/lib/api";
import { SwimlaneSection } from "./swimlane-section";
import { ColumnsMenu } from "./columns-menu";
import { deriveAutoHiddenStepIds } from "@/lib/kanban/auto-hide-empty-columns";
import { sortWorkflowStepsByPosition } from "@/lib/kanban/workflow-step-order";
import { useKanbanDisplaySettings } from "@/hooks/use-kanban-display-settings";
import {
  getEffectiveView,
  type MobileWorkflowNavigation,
  type ViewContentProps,
} from "@/lib/kanban/view-registry";
import type { Task } from "@/components/kanban-card";
import type { MoveTaskError } from "@/hooks/use-drag-and-drop";
import type { WorkflowSnapshotData } from "@/lib/state/slices/kanban/types";
import { t } from "@/lib/i18n";
import {
  useStableStringSet,
  useStableWorkflowList,
  useSwimlaneRenderData,
  useWorkflowSwimlaneData,
} from "@/hooks/domains/kanban/use-swimlane-render-data";

export type SwimlaneContainerProps = {
  viewMode: string;
  workflowFilter: string | null;
  onPreviewTask: (task: Task) => void;
  onOpenTask: (task: Task) => void;
  onEditTask: (task: Task) => void;
  onDeleteTask: (task: Task) => void;
  onArchiveTask?: (task: Task) => void;
  onMoveError?: (error: MoveTaskError) => void;
  deletingTaskId?: string | null;
  archivingTaskId?: string | null;
  showMaximizeButton?: boolean;
  searchQuery?: string;
  selectedRepositoryIds?: string[];
  /** Predicate combining every active plugin `registerTaskFilter` selection (AND semantics). */
  matchesPluginTaskFilters?: (taskId: string) => boolean;
  selectedIds?: Set<string>;
  onToggleSelect?: (taskId: string) => void;
  onSelectRange?: (taskId: string, orderedIds: string[]) => void;
  isMultiSelectMode?: boolean;
  onToggleMultiSelect?: () => void;
  onWorkflowChange?: (workflowId: string | null) => void;
};

type EmptyMessageOptions = {
  isLoading: boolean;
  snapshots: Record<string, unknown>;
  orderedWorkflows: { id: string; name: string }[];
  visibleWorkflows: { id: string; name: string }[];
  showEmptyBoard: boolean;
};

function getEmptyMessage({
  isLoading,
  snapshots,
  orderedWorkflows,
  visibleWorkflows,
  showEmptyBoard,
}: EmptyMessageOptions): string | null {
  if (isLoading && Object.keys(snapshots).length === 0) return t("common:loading");
  if (orderedWorkflows.length === 0) return t("kanban:noWorkflowsAvailableYet");
  if (visibleWorkflows.length === 0 && !showEmptyBoard) return t("kanban:noTasksYet");
  return null;
}

function renderEmptyState(emptyMessage: string) {
  return (
    <div className="flex-1 min-h-0 px-4 pt-3 pb-4">
      <div className="h-full rounded-lg border border-dashed border-border/60 flex items-center justify-center text-sm text-muted-foreground">
        {emptyMessage}
      </div>
    </div>
  );
}

const EMPTY_SELECTED_REPOSITORY_IDS: string[] = [];
const EMPTY_WORKFLOW_STEPS: WorkflowSnapshotData["steps"] = [];
const WORKFLOW_POINTER_SENSOR_OPTIONS = { activationConstraint: { distance: 8 } };

type WorkflowItemProps = {
  wf: { id: string; name: string };
  repoFilter: Set<string>;
  searchQuery: string;
  matchesPluginTaskFilters?: (taskId: string) => boolean;
  ViewComponent: ComponentType<ViewContentProps>;
  hideHeader: boolean;
  isSortable: boolean;
  isCollapsed: boolean;
  toggleCollapse: (workflowId: string) => void;
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
  onToggleMultiSelect?: () => void;
  fillHeight?: boolean;
  mobileWorkflowNavigation?: MobileWorkflowNavigation;
  onToggleStepVisibility: (workflowId: string, stepId: string) => void;
  onToggleAutoHideEmpty: (workflowId: string) => void;
};

const SortableWorkflowItem = memo(function SortableWorkflowItem({
  wf,
  hideHeader,
  isSortable,
  fillHeight,
  toggleCollapse,
  ...rest
}: WorkflowItemProps) {
  const { attributes, listeners, setNodeRef, transform, transition, isDragging } = useSortable({
    id: wf.id,
    disabled: !isSortable,
  });
  const style = {
    transform: CSS.Transform.toString(transform),
    transition,
    opacity: isDragging ? 0.5 : 1,
  };
  const dragHandleProps = useMemo(
    () => (isSortable && !hideHeader ? { ...attributes, ...listeners } : undefined),
    [attributes, hideHeader, isSortable, listeners],
  );
  const onToggleCollapse = useCallback(() => toggleCollapse(wf.id), [toggleCollapse, wf.id]);
  return (
    <div
      ref={setNodeRef}
      style={style}
      className={fillHeight ? "h-full min-h-0 flex-1" : undefined}
    >
      <WorkflowItemContent
        wf={wf}
        hideHeader={hideHeader}
        fillHeight={fillHeight}
        dragHandleProps={dragHandleProps}
        onToggleCollapse={onToggleCollapse}
        {...rest}
      />
    </div>
  );
});

type WorkflowItemContentProps = Omit<WorkflowItemProps, "isSortable" | "toggleCollapse"> & {
  dragHandleProps?: HTMLAttributes<HTMLDivElement>;
  onToggleCollapse: () => void;
};

const WorkflowItemContent = memo(function WorkflowItemContent({
  wf,
  repoFilter,
  searchQuery,
  matchesPluginTaskFilters,
  ViewComponent,
  hideHeader,
  isCollapsed,
  onToggleCollapse,
  dragHandleProps,
  onToggleMultiSelect,
  fillHeight,
  onToggleStepVisibility,
  onToggleAutoHideEmpty,
  ...viewProps
}: WorkflowItemContentProps) {
  const { snapshot, tasks, occupancyTasks, hiddenStepIds, hiddenSet, autoHideEmpty } =
    useWorkflowSwimlaneData(wf.id, repoFilter, searchQuery, matchesPluginTaskFilters);
  const snapshotSteps = snapshot?.steps ?? EMPTY_WORKFLOW_STEPS;
  const derivedAutoHiddenSet = useMemo(
    () => deriveAutoHiddenStepIds(snapshotSteps, occupancyTasks, autoHideEmpty, hiddenStepIds),
    [autoHideEmpty, hiddenStepIds, occupancyTasks, snapshotSteps],
  );
  const autoHiddenSet = useStableStringSet(derivedAutoHiddenSet);
  const moveTargetSteps = useMemo(
    () => sortWorkflowStepsByPosition(snapshotSteps).filter((step) => !hiddenSet.has(step.id)),
    [hiddenSet, snapshotSteps],
  );
  const steps = useMemo(
    () => moveTargetSteps.filter((step) => !autoHiddenSet.has(step.id)),
    [moveTargetSteps, autoHiddenSet],
  );
  const content = (
    <ViewComponent
      workflowId={wf.id}
      steps={steps}
      moveTargetSteps={moveTargetSteps}
      tasks={tasks}
      {...viewProps}
    />
  );

  if (!snapshot) return null;

  if (hideHeader) {
    return <div className={fillHeight ? "h-full min-h-0" : undefined}>{content}</div>;
  }

  return (
    <SwimlaneSection
      workflowId={wf.id}
      workflowName={wf.name}
      taskCount={tasks.length}
      isCollapsed={isCollapsed}
      onToggleCollapse={onToggleCollapse}
      dragHandleProps={dragHandleProps}
      onToggleMultiSelect={onToggleMultiSelect}
      isMultiSelectMode={viewProps.isMultiSelectMode}
      fillHeight={fillHeight}
      columnsMenu={
        <ColumnsMenu
          workflowId={wf.id}
          workflowName={wf.name}
          steps={snapshotSteps}
          hiddenStepIds={hiddenStepIds}
          onToggle={onToggleStepVisibility}
          autoHideEmpty={autoHideEmpty}
          onToggleAutoHide={onToggleAutoHideEmpty}
        />
      }
    >
      {content}
    </SwimlaneSection>
  );
});

function useWorkflowReorder(
  orderedWorkflows: { id: string; name: string }[],
  workflowFilter: string | null,
) {
  const reorderWorkflowItems = useAppStore((state) => state.reorderWorkflowItems);
  const workflows = useAppStore((state) => state.workflows.items);
  const workspaceId = workflows[0]?.workspaceId;
  const sensors = useSensors(useSensor(PointerSensor, WORKFLOW_POINTER_SENSOR_OPTIONS));
  const canSort = !workflowFilter && orderedWorkflows.length > 1;

  const handleDragEnd = useCallback(
    (event: DragEndEvent) => {
      const { active, over } = event;
      if (!over || active.id === over.id) return;
      const oldIndex = orderedWorkflows.findIndex((wf) => wf.id === active.id);
      const newIndex = orderedWorkflows.findIndex((wf) => wf.id === over.id);
      if (oldIndex === -1 || newIndex === -1) return;
      const reordered = arrayMove(orderedWorkflows, oldIndex, newIndex);
      reorderWorkflowItems(reordered.map((wf) => wf.id));
      if (workspaceId) {
        reorderWorkflows(
          workspaceId,
          reordered.map((wf) => wf.id),
        ).catch(() => {});
      }
    },
    [orderedWorkflows, reorderWorkflowItems, workspaceId],
  );

  return { sensors, canSort, handleDragEnd };
}

function getRenderedWorkflows(
  isMobileKanban: boolean,
  focusedWorkflowId: string | null,
  visibleWorkflows: { id: string; name: string }[],
) {
  if (!isMobileKanban || !focusedWorkflowId) return visibleWorkflows;
  return visibleWorkflows.filter((workflow) => workflow.id === focusedWorkflowId);
}

function getContainerClass(isMobileKanban: boolean, isMobile: boolean): string {
  if (isMobileKanban) return "flex flex-1 min-h-0 flex-col overflow-hidden pb-4";
  return `flex-1 min-h-0 space-y-3 overflow-y-auto pt-3 pb-4${isMobile ? "" : " px-4"}`;
}

function shouldHideHeaders(
  isMobile: boolean,
  isMobileKanban: boolean,
  workflowFilter: string | null,
  workflowCount: number,
): boolean {
  if (!isMobile) return false;
  return isMobileKanban || workflowFilter !== null || workflowCount === 1;
}

type WorkflowItemsProps = {
  workflows: { id: string; name: string }[];
  repoFilter: Set<string>;
  searchQuery: string;
  matchesPluginTaskFilters?: (taskId: string) => boolean;
  ViewComponent: ComponentType<ViewContentProps>;
  hideHeaders: boolean;
  fillHeight: boolean;
  isMobileKanban: boolean;
  onToggleStepVisibility: (workflowId: string, stepId: string) => void;
  onToggleAutoHideEmpty: (workflowId: string) => void;
  canSortWorkflows: boolean;
  isCollapsed: (workflowId: string) => boolean;
  toggleCollapse: (workflowId: string) => void;
  containerProps: SwimlaneContainerProps;
  mobileWorkflowNavigation?: MobileWorkflowNavigation;
};

function WorkflowItems({
  workflows,
  repoFilter,
  searchQuery,
  matchesPluginTaskFilters,
  ViewComponent,
  hideHeaders,
  fillHeight,
  isMobileKanban,
  onToggleStepVisibility,
  onToggleAutoHideEmpty,
  canSortWorkflows,
  isCollapsed,
  toggleCollapse,
  containerProps,
  mobileWorkflowNavigation,
}: WorkflowItemsProps) {
  return workflows.map((workflow, index) => {
    const collapsed = isCollapsed(workflow.id);
    return (
      <SortableWorkflowItem
        key={isMobileKanban ? "mobile-active-workflow" : workflow.id}
        wf={workflow}
        repoFilter={repoFilter}
        searchQuery={searchQuery}
        matchesPluginTaskFilters={matchesPluginTaskFilters}
        ViewComponent={ViewComponent}
        hideHeader={hideHeaders}
        fillHeight={fillHeight && !collapsed}
        isSortable={canSortWorkflows && !isMobileKanban}
        isCollapsed={collapsed}
        toggleCollapse={toggleCollapse}
        onPreviewTask={containerProps.onPreviewTask}
        onOpenTask={containerProps.onOpenTask}
        onEditTask={containerProps.onEditTask}
        onDeleteTask={containerProps.onDeleteTask}
        onArchiveTask={containerProps.onArchiveTask}
        onMoveError={containerProps.onMoveError}
        deletingTaskId={containerProps.deletingTaskId}
        archivingTaskId={containerProps.archivingTaskId}
        showMaximizeButton={containerProps.showMaximizeButton}
        selectedIds={containerProps.selectedIds}
        onToggleSelect={containerProps.onToggleSelect}
        onSelectRange={containerProps.onSelectRange}
        isMultiSelectMode={containerProps.isMultiSelectMode}
        onToggleMultiSelect={index === 0 ? containerProps.onToggleMultiSelect : undefined}
        mobileWorkflowNavigation={mobileWorkflowNavigation}
        onToggleStepVisibility={onToggleStepVisibility}
        onToggleAutoHideEmpty={onToggleAutoHideEmpty}
      />
    );
  });
}

/**
 * Publishes which workflow the phone board is showing to the store so the
 * mobile menu drawer's Columns control targets that same workflow instead of
 * re-deriving focus from filters.
 */
function usePublishMobileFocus(focusedWorkflowId: string | null) {
  const setMobileKanbanFocusedWorkflow = useAppStore(
    (state) => state.setMobileKanbanFocusedWorkflow,
  );
  useEffect(() => {
    setMobileKanbanFocusedWorkflow(focusedWorkflowId);
  }, [focusedWorkflowId, setMobileKanbanFocusedWorkflow]);
}

function useMobileWorkflowNavigation(
  isMobileKanban: boolean,
  focusedWorkflowId: string | null,
  workflowOptions: MobileWorkflowNavigation["workflows"],
  onWorkflowChange: SwimlaneContainerProps["onWorkflowChange"],
) {
  return useMemo(
    () =>
      isMobileKanban && focusedWorkflowId && onWorkflowChange
        ? { activeWorkflowId: focusedWorkflowId, workflows: workflowOptions, onWorkflowChange }
        : undefined,
    [focusedWorkflowId, isMobileKanban, onWorkflowChange, workflowOptions],
  );
}

export function SwimlaneContainer(containerProps: SwimlaneContainerProps) {
  const {
    viewMode,
    workflowFilter,
    searchQuery,
    selectedRepositoryIds = EMPTY_SELECTED_REPOSITORY_IDS,
  } = containerProps;
  const { isMobile } = useResponsiveBreakpoint();
  const { onToggleStepVisibility, onToggleAutoHideEmpty } = useKanbanDisplaySettings();
  const { isCollapsed, toggleCollapse } = useSwimlaneCollapse();
  const {
    snapshots,
    isLoading,
    orderedWorkflows,
    workflowOptions,
    getFilteredTasks,
    hasLiveHiddenSteps,
    repoFilter,
  } = useSwimlaneRenderData(
    workflowFilter,
    selectedRepositoryIds,
    searchQuery ?? "",
    containerProps.matchesPluginTaskFilters,
  );
  const {
    sensors: workflowSensors,
    canSort: canSortWorkflows,
    handleDragEnd: handleWorkflowDragEnd,
  } = useWorkflowReorder(orderedWorkflows, workflowFilter);

  const view = getEffectiveView(viewMode, isMobile);
  const isMobileKanban = isMobile && view.id === "kanban";
  const visibleWorkflows = useStableWorkflowList(
    selectVisibleWorkflows({
      workflowFilter,
      orderedWorkflows,
      hasTasks: (workflowId) => getFilteredTasks(workflowId).length > 0,
      hasLiveHiddenSteps,
      showEmptyBoard: isMobileKanban,
    }),
  );
  const focusedWorkflowId = visibleWorkflows[0]?.id ?? null;
  usePublishMobileFocus(isMobileKanban ? focusedWorkflowId : null);
  const renderedWorkflows = useStableWorkflowList(
    getRenderedWorkflows(isMobileKanban, focusedWorkflowId, visibleWorkflows),
  );
  const sortableWorkflowIds = useMemo(
    () => renderedWorkflows.map((workflow) => workflow.id),
    [renderedWorkflows],
  );
  const mobileWorkflowNavigation = useMobileWorkflowNavigation(
    isMobileKanban,
    focusedWorkflowId,
    workflowOptions,
    containerProps.onWorkflowChange,
  );

  const emptyMessage = getEmptyMessage({
    isLoading,
    snapshots,
    orderedWorkflows,
    visibleWorkflows,
    showEmptyBoard: isMobileKanban,
  });
  if (emptyMessage) return renderEmptyState(emptyMessage);

  const hideHeaders = shouldHideHeaders(
    isMobile,
    isMobileKanban,
    workflowFilter,
    orderedWorkflows.length,
  );
  const containerClass = getContainerClass(isMobileKanban, isMobile);

  return (
    <DndContext
      sensors={workflowSensors}
      collisionDetection={closestCenter}
      onDragEnd={handleWorkflowDragEnd}
    >
      <SortableContext items={sortableWorkflowIds} strategy={verticalListSortingStrategy}>
        <div className={containerClass} data-testid="swimlane-container">
          <WorkflowItems
            workflows={renderedWorkflows}
            repoFilter={repoFilter}
            searchQuery={searchQuery ?? ""}
            matchesPluginTaskFilters={containerProps.matchesPluginTaskFilters}
            ViewComponent={view.component}
            hideHeaders={hideHeaders}
            onToggleStepVisibility={onToggleStepVisibility}
            onToggleAutoHideEmpty={onToggleAutoHideEmpty}
            fillHeight={view.id === "kanban"}
            isMobileKanban={isMobileKanban}
            canSortWorkflows={canSortWorkflows}
            isCollapsed={isCollapsed}
            toggleCollapse={toggleCollapse}
            containerProps={containerProps}
            mobileWorkflowNavigation={mobileWorkflowNavigation}
          />
        </div>
      </SortableContext>
    </DndContext>
  );
}
