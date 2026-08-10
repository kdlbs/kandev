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
import { reorderWorkflows } from "@/lib/api";
import { SwimlaneSection } from "./swimlane-section";
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

/** Exported for unit testing; not part of the module's public component API. */
export function filterTasks(
  snapshots: Record<string, { tasks: Task[] }>,
  workflowId: string,
  repoFilter: ReturnType<typeof mapSelectedRepositoryIds>,
  searchQuery?: string,
  matchesPluginTaskFilters?: (taskId: string) => boolean,
): Task[] {
  const snapshot = snapshots[workflowId];
  if (!snapshot) return [];
  let tasks = snapshot.tasks as Task[];
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
  ...viewProps
}: Omit<WorkflowItemProps, "isSortable"> & { dragHandleProps?: HTMLAttributes<HTMLDivElement> }) {
  const steps = [...snapshot.steps].sort((a, b) => a.position - b.position);
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

type WorkflowLike = { id: string; name: string; hidden?: boolean };

/**
 * Selects the workflow swimlanes the board renders.
 *
 * - No filter ("All Workflows"): every workflow with a loaded snapshot that is
 *   not hidden (system/office workflows stay off the board by design).
 * - Explicit filter: the selected workflow itself, hidden or not. The display
 *   dropdown offers every workflow in the store — including hidden ones like
 *   Improve Kandev — so an explicit selection must render that board;
 *   resolving against the visible-only list would silently show "No tasks yet"
 *   for a workflow that has tasks.
 */
export function selectWorkflowSwimlanes(
  workflowFilter: string | null | undefined,
  workflows: WorkflowLike[],
  snapshots: Record<string, unknown>,
): WorkflowLike[] {
  if (workflowFilter) {
    const workflow = workflows.find((item) => item.id === workflowFilter && snapshots[item.id]);
    return workflow ? [workflow] : [];
  }
  return workflows.filter((workflow) => !workflow.hidden && snapshots[workflow.id]);
}

/**
 * Selects the workflows the mobile board navigator offers. The navigator is
 * the only workflow switcher on the mobile kanban page (the display menu hides
 * its workflow select there), so hidden workflows with tasks — e.g. Improve
 * Kandev, whose tasks land in a hidden workflow — must be reachable, mirroring
 * the sidebar which already aggregates their snapshots. Empty hidden system
 * workflows stay out.
 */
export function selectMobileNavigatorWorkflows(
  visibleOrdered: WorkflowLike[],
  workflows: WorkflowLike[],
  getFilteredTasks: (workflowId: string) => unknown[],
): Array<{ workflow: WorkflowLike; tasks: unknown[] }> {
  const visibleIds = new Set(visibleOrdered.map((workflow) => workflow.id));
  const entries: Array<{ workflow: WorkflowLike; tasks: unknown[] }> = [];
  for (const workflow of visibleOrdered) {
    entries.push({ workflow, tasks: getFilteredTasks(workflow.id) });
  }
  for (const workflow of workflows) {
    if (!workflow.hidden || visibleIds.has(workflow.id)) continue;
    const tasks = getFilteredTasks(workflow.id);
    if (tasks.length > 0) entries.push({ workflow, tasks });
  }
  return entries;
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
    (wfId: string) =>
      filterTasks(snapshots, wfId, repoFilter, searchQuery, matchesPluginTaskFilters),
    [snapshots, repoFilter, searchQuery, matchesPluginTaskFilters],
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

function getVisibleWorkflows(
  workflowFilter: string | null,
  orderedWorkflows: { id: string; name: string }[],
  getFilteredTasks: (workflowId: string) => Task[],
  showEmptyBoard: boolean,
) {
  if (workflowFilter) return orderedWorkflows;
  const withTasks = orderedWorkflows.filter((workflow) => getFilteredTasks(workflow.id).length > 0);
  if (withTasks.length > 0 || !showEmptyBoard) return withTasks;
  return orderedWorkflows;
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
      />
    );
  });
}

export function SwimlaneContainer(containerProps: SwimlaneContainerProps) {
  const { viewMode, workflowFilter, searchQuery, selectedRepositoryIds = [] } = containerProps;
  const { isMobile } = useResponsiveBreakpoint();
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
  const visibleWorkflows = getVisibleWorkflows(
    workflowFilter,
    orderedWorkflows,
    getFilteredTasks,
    isMobileKanban,
  );
  const focusedWorkflowId = visibleWorkflows[0]?.id ?? null;
  useEffect(() => {
    containerProps.onMobileWorkflowFocusChange?.(isMobileKanban ? focusedWorkflowId : null);
  }, [containerProps.onMobileWorkflowFocusChange, focusedWorkflowId, isMobileKanban]);
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
