"use client";

import { type ComponentType, type HTMLAttributes, useCallback, useEffect, useMemo } from "react";
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
import { filterTasksByRepositories, mapSelectedRepositoryIds } from "@/lib/kanban/filters";
import {
  selectMobileNavigatorWorkflows,
  selectWorkflowSwimlanes,
  selectVisibleWorkflows,
} from "@/lib/kanban/workflow-swimlanes";
import { reorderWorkflows } from "@/lib/api";
import { SwimlaneSection } from "./swimlane-section";
import { ColumnsMenu } from "./columns-menu";
import { useKanbanDisplaySettings } from "@/hooks/use-kanban-display-settings";
import {
  getEffectiveView,
  type MobileWorkflowNavigation,
  type ViewContentProps,
} from "@/lib/kanban/view-registry";
import type { Task } from "@/components/kanban-card";
import type { MoveTaskError } from "@/hooks/use-drag-and-drop";
import type { Repository } from "@/lib/types/http";
import type { WorkflowSnapshotData } from "@/lib/state/slices/kanban/types";

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
  onMobileWorkflowFocusChange?: (workflowId: string | null) => void;
};

type EmptyMessageOptions = {
  isLoading: boolean;
  snapshots: Record<string, unknown>;
  orderedWorkflows: { id: string; name: string }[];
  workflowFilter: string | null;
  getFilteredTasks: (id: string) => Task[];
  showEmptyBoard: boolean;
};

function getEmptyMessage({
  isLoading,
  snapshots,
  orderedWorkflows,
  workflowFilter,
  getFilteredTasks,
  showEmptyBoard,
}: EmptyMessageOptions): string | null {
  if (isLoading && Object.keys(snapshots).length === 0) return "Loading...";
  if (orderedWorkflows.length === 0) return "No workflows available yet.";
  const visible = workflowFilter
    ? orderedWorkflows
    : orderedWorkflows.filter((wf) => getFilteredTasks(wf.id).length > 0);
  if (visible.length === 0 && !showEmptyBoard) return "No tasks yet";
  return null;
}

function renderEmptyState(emptyMessage: string) {
  return (
    <div className="flex-1 min-h-0 px-4 pb-4">
      <div className="h-full rounded-lg border border-dashed border-border/60 flex items-center justify-center text-sm text-muted-foreground">
        {emptyMessage}
      </div>
    </div>
  );
}

type FilterTasksOptions = {
  searchQuery?: string;
  matchesPluginTaskFilters?: (taskId: string) => boolean;
  hiddenStepIds?: Set<string>;
};

/** Exported for unit testing; not part of the module's public component API. */
export function filterTasks(
  snapshots: Record<string, { tasks: Task[]; steps: { id: string }[] }>,
  workflowId: string,
  repoFilter: ReturnType<typeof mapSelectedRepositoryIds>,
  options?: FilterTasksOptions,
): Task[] {
  const snapshot = snapshots[workflowId];
  if (!snapshot) return [];
  let tasks = snapshot.tasks as Task[];
  const { hiddenStepIds, searchQuery, matchesPluginTaskFilters } = options ?? {};
  if (hiddenStepIds && hiddenStepIds.size > 0) {
    const liveStepIds = new Set(snapshot.steps.map((s) => s.id));
    const effectiveHidden = new Set([...hiddenStepIds].filter((id) => liveStepIds.has(id)));
    if (effectiveHidden.size > 0) {
      tasks = tasks.filter((t) => !effectiveHidden.has(t.workflowStepId));
    }
  }
  tasks = filterTasksByRepositories(tasks, repoFilter);
  if (searchQuery) {
    const q = searchQuery.toLowerCase();
    tasks = tasks.filter(
      (t) =>
        t.title.toLowerCase().includes(q) ||
        (t.description && t.description.toLowerCase().includes(q)),
    );
  }
  if (matchesPluginTaskFilters) {
    tasks = tasks.filter((t) => matchesPluginTaskFilters(t.id));
  }
  return tasks;
}

/** Stable identity so an unhidden workflow does not remount its menu each render. */
const EMPTY_HIDDEN_IDS: string[] = [];

type WorkflowItemProps = {
  wf: { id: string; name: string };
  snapshot: WorkflowSnapshotData;
  tasks: Task[];
  ViewComponent: ComponentType<ViewContentProps>;
  hideHeader: boolean;
  isSortable: boolean;
  isCollapsed: boolean;
  onToggleCollapse: () => void;
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
};

function SortableWorkflowItem({
  wf,
  hideHeader,
  isSortable,
  fillHeight,
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
  const dragHandleProps = isSortable && !hideHeader ? { ...attributes, ...listeners } : undefined;
  return (
    <div ref={setNodeRef} style={style} className={fillHeight ? "min-h-0 flex-1" : undefined}>
      <WorkflowItemContent
        wf={wf}
        hideHeader={hideHeader}
        fillHeight={fillHeight}
        dragHandleProps={dragHandleProps}
        {...rest}
      />
    </div>
  );
}

function WorkflowItemContent({
  wf,
  snapshot,
  tasks,
  ViewComponent,
  hideHeader,
  isCollapsed,
  onToggleCollapse,
  dragHandleProps,
  onToggleMultiSelect,
  fillHeight,
  onToggleStepVisibility,
  ...viewProps
}: Omit<WorkflowItemProps, "isSortable"> & { dragHandleProps?: HTMLAttributes<HTMLDivElement> }) {
  const hiddenWorkflowStepIds = useAppStore((state) => state.userSettings.hiddenWorkflowStepIds);
  const hiddenSet = useMemo(() => {
    const ids = hiddenWorkflowStepIds[wf.id];
    if (!ids || ids.length === 0) return new Set<string>();
    const liveStepIds = new Set(snapshot.steps.map((s) => s.id));
    return new Set(ids.filter((id) => liveStepIds.has(id)));
  }, [hiddenWorkflowStepIds, wf.id, snapshot.steps]);
  const steps = useMemo(
    () =>
      [...snapshot.steps]
        .filter((step) => !hiddenSet.has(step.id))
        .sort((a, b) => a.position - b.position),
    [snapshot.steps, hiddenSet],
  );
  const content = <ViewComponent workflowId={wf.id} steps={steps} tasks={tasks} {...viewProps} />;

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
      columnsMenu={
        <ColumnsMenu
          workflowId={wf.id}
          workflowName={wf.name}
          steps={snapshot.steps}
          hiddenStepIds={hiddenWorkflowStepIds[wf.id] ?? EMPTY_HIDDEN_IDS}
          onToggle={onToggleStepVisibility}
        />
      }
    >
      {content}
    </SwimlaneSection>
  );
}

function useWorkflowReorder(
  orderedWorkflows: { id: string; name: string }[],
  workflowFilter: string | null,
) {
  const reorderWorkflowItems = useAppStore((state) => state.reorderWorkflowItems);
  const workflows = useAppStore((state) => state.workflows.items);
  const workspaceId = workflows[0]?.workspaceId;
  const sensors = useSensors(useSensor(PointerSensor, { activationConstraint: { distance: 8 } }));
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

function useSwimlaneData(
  workflowFilter: string | null | undefined,
  selectedRepositoryIds: string[],
  searchQuery: string,
  matchesPluginTaskFilters?: (taskId: string) => boolean,
) {
  const snapshots = useAppStore((state) => state.kanbanMulti.snapshots);
  const isLoading = useAppStore((state) => state.kanbanMulti.isLoading);
  const workflows = useAppStore((state) => state.workflows.items);
  const repositoriesByWorkspace = useAppStore((state) => state.repositories.itemsByWorkspaceId);
  const hiddenWorkflowStepIds = useAppStore((state) => state.userSettings.hiddenWorkflowStepIds);

  const repositories = useMemo(
    () => Object.values(repositoriesByWorkspace).flat() as Repository[],
    [repositoriesByWorkspace],
  );
  const repoFilter = useMemo(
    () => mapSelectedRepositoryIds(repositories, selectedRepositoryIds),
    [repositories, selectedRepositoryIds],
  );
  const allOrderedWorkflows = useMemo(
    () => selectWorkflowSwimlanes(null, workflows, snapshots),
    [workflows, snapshots],
  );
  const orderedWorkflows = useMemo(
    () => selectWorkflowSwimlanes(workflowFilter, workflows, snapshots),
    [workflowFilter, workflows, snapshots],
  );

  const getFilteredTasks = useCallback(
    (wfId: string) => {
      const hidden = hiddenWorkflowStepIds[wfId];
      const hiddenSet = hidden && hidden.length > 0 ? new Set(hidden) : undefined;
      return filterTasks(snapshots, wfId, repoFilter, {
        searchQuery,
        matchesPluginTaskFilters,
        hiddenStepIds: hiddenSet,
      });
    },
    [snapshots, repoFilter, searchQuery, matchesPluginTaskFilters, hiddenWorkflowStepIds],
  );

  // The mobile board navigator is the only workflow switcher on the mobile
  // kanban page (the display menu hides its workflow select there), so hidden
  // workflows with tasks are included alongside the visible ones. The selector
  // returns the filtered tasks with each entry so `getFilteredTasks` runs once
  // per workflow and the task count reuses that result.
  const workflowOptions = useMemo(
    () =>
      selectMobileNavigatorWorkflows(allOrderedWorkflows, workflows, getFilteredTasks).map(
        ({ workflow, tasks }) => ({ ...workflow, taskCount: tasks.length }),
      ),
    [allOrderedWorkflows, workflows, getFilteredTasks],
  );

  return {
    snapshots,
    isLoading,
    orderedWorkflows,
    allOrderedWorkflows,
    workflowOptions,
    getFilteredTasks,
  };
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
  return `flex-1 min-h-0 space-y-3 overflow-y-auto pb-4${isMobile ? "" : " px-4"}`;
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
  snapshots: Record<string, WorkflowSnapshotData>;
  getFilteredTasks: (workflowId: string) => Task[];
  ViewComponent: ComponentType<ViewContentProps>;
  hideHeaders: boolean;
  fillHeight: boolean;
  onToggleStepVisibility: (workflowId: string, stepId: string) => void;
  canSortWorkflows: boolean;
  isCollapsed: (workflowId: string) => boolean;
  toggleCollapse: (workflowId: string) => void;
  containerProps: SwimlaneContainerProps;
  mobileWorkflowNavigation?: MobileWorkflowNavigation;
};

function WorkflowItems({
  workflows,
  snapshots,
  getFilteredTasks,
  ViewComponent,
  hideHeaders,
  fillHeight,
  onToggleStepVisibility,
  canSortWorkflows,
  isCollapsed,
  toggleCollapse,
  containerProps,
  mobileWorkflowNavigation,
}: WorkflowItemsProps) {
  return workflows.map((workflow, index) => {
    const snapshot = snapshots[workflow.id];
    if (!snapshot) return null;
    return (
      <SortableWorkflowItem
        key={fillHeight ? "mobile-active-workflow" : workflow.id}
        wf={workflow}
        snapshot={snapshot}
        tasks={getFilteredTasks(workflow.id)}
        ViewComponent={ViewComponent}
        hideHeader={hideHeaders}
        fillHeight={fillHeight}
        isSortable={canSortWorkflows && !fillHeight}
        isCollapsed={isCollapsed(workflow.id)}
        onToggleCollapse={() => toggleCollapse(workflow.id)}
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
      />
    );
  });
}

/**
 * Whether a workflow is holding hidden columns right now — its persisted hidden
 * set intersected with the steps its snapshot actually has. A stale id (step
 * deleted) hides nothing, so it must not report true either.
 */
function useHasLiveHiddenSteps(snapshots: Record<string, WorkflowSnapshotData>) {
  const hiddenWorkflowStepIds = useAppStore((state) => state.userSettings.hiddenWorkflowStepIds);
  return useCallback(
    (workflowId: string) => {
      const hidden = hiddenWorkflowStepIds[workflowId];
      if (!hidden || hidden.length === 0) return false;
      const liveStepIds = new Set((snapshots[workflowId]?.steps ?? []).map((step) => step.id));
      return hidden.some((id) => liveStepIds.has(id));
    },
    [hiddenWorkflowStepIds, snapshots],
  );
}

/**
 * Publishes which workflow the phone board is showing: to the board's own
 * callback, and to the store so the mobile menu drawer's Columns control
 * targets that same workflow instead of re-deriving focus from filters.
 */
function usePublishMobileFocus(
  focusedWorkflowId: string | null,
  onMobileWorkflowFocusChange?: (workflowId: string | null) => void,
) {
  const setMobileKanbanFocusedWorkflow = useAppStore(
    (state) => state.setMobileKanbanFocusedWorkflow,
  );
  useEffect(() => {
    onMobileWorkflowFocusChange?.(focusedWorkflowId);
    setMobileKanbanFocusedWorkflow(focusedWorkflowId);
  }, [focusedWorkflowId, onMobileWorkflowFocusChange, setMobileKanbanFocusedWorkflow]);
}

export function SwimlaneContainer(containerProps: SwimlaneContainerProps) {
  const { viewMode, workflowFilter, searchQuery, selectedRepositoryIds = [] } = containerProps;
  const { isMobile } = useResponsiveBreakpoint();
  const { onToggleStepVisibility } = useKanbanDisplaySettings();
  const { isCollapsed, toggleCollapse } = useSwimlaneCollapse();
  const { snapshots, isLoading, orderedWorkflows, workflowOptions, getFilteredTasks } =
    useSwimlaneData(
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
  const hasLiveHiddenSteps = useHasLiveHiddenSteps(snapshots);
  const visibleWorkflows = selectVisibleWorkflows({
    workflowFilter,
    orderedWorkflows,
    hasTasks: (workflowId) => getFilteredTasks(workflowId).length > 0,
    hasLiveHiddenSteps,
    showEmptyBoard: isMobileKanban,
  });
  const focusedWorkflowId = visibleWorkflows[0]?.id ?? null;
  usePublishMobileFocus(
    isMobileKanban ? focusedWorkflowId : null,
    containerProps.onMobileWorkflowFocusChange,
  );
  const renderedWorkflows = getRenderedWorkflows(
    isMobileKanban,
    focusedWorkflowId,
    visibleWorkflows,
  );
  const mobileWorkflowNavigation =
    isMobileKanban && focusedWorkflowId && containerProps.onWorkflowChange
      ? {
          activeWorkflowId: focusedWorkflowId,
          workflows: workflowOptions,
          onWorkflowChange: containerProps.onWorkflowChange,
        }
      : undefined;

  const emptyMessage = getEmptyMessage({
    isLoading,
    snapshots,
    orderedWorkflows,
    workflowFilter,
    getFilteredTasks,
    showEmptyBoard: isMobileKanban,
  });
  if (emptyMessage) return renderEmptyState(emptyMessage);

  const ViewComponent = view.component;
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
      <SortableContext
        items={renderedWorkflows.map((workflow) => workflow.id)}
        strategy={verticalListSortingStrategy}
      >
        <div className={containerClass} data-testid="swimlane-container">
          <WorkflowItems
            workflows={renderedWorkflows}
            snapshots={snapshots}
            getFilteredTasks={getFilteredTasks}
            ViewComponent={ViewComponent}
            hideHeaders={hideHeaders}
            onToggleStepVisibility={onToggleStepVisibility}
            fillHeight={isMobileKanban}
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
