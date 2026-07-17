"use client";

import {
  type ComponentType,
  type HTMLAttributes,
  useCallback,
  useEffect,
  useMemo,
  useState,
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
import { filterTasksByRepositories, mapSelectedRepositoryIds } from "@/lib/kanban/filters";
import { reorderWorkflows } from "@/lib/api";
import { SwimlaneSection } from "./swimlane-section";
import {
  getViewByStoredValue,
  getDefaultView,
  type ViewContentProps,
} from "@/lib/kanban/view-registry";
import type { Task } from "@/components/kanban-card";
import type { MoveTaskError } from "@/hooks/use-drag-and-drop";
import type { Repository } from "@/lib/types/http";
import type { WorkflowSnapshotData } from "@/lib/state/slices/kanban/types";
import { MobileWorkflowPicker } from "./mobile-workflow-picker";

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
  selectedIds?: Set<string>;
  onToggleSelect?: (taskId: string) => void;
  onSelectRange?: (taskId: string, orderedIds: string[]) => void;
  isMultiSelectMode?: boolean;
  onToggleMultiSelect?: () => void;
};

function getEmptyMessage(
  isLoading: boolean,
  snapshots: Record<string, unknown>,
  orderedWorkflows: { id: string; name: string }[],
  workflowFilter: string | null,
  getFilteredTasks: (id: string) => Task[],
): string | null {
  if (isLoading && Object.keys(snapshots).length === 0) return "Loading...";
  if (orderedWorkflows.length === 0) return "No workflows available yet.";
  const visible = workflowFilter
    ? orderedWorkflows
    : orderedWorkflows.filter((wf) => getFilteredTasks(wf.id).length > 0);
  if (visible.length === 0) return "No tasks yet";
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

function filterTasks(
  snapshots: Record<string, { tasks: Task[] }>,
  workflowId: string,
  repoFilter: ReturnType<typeof mapSelectedRepositoryIds>,
  searchQuery?: string,
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
    return (
      <div key={wf.id} className={fillHeight ? "h-full min-h-0" : undefined}>
        {content}
      </div>
    );
  }

  return (
    <SwimlaneSection
      key={wf.id}
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

function useSwimlaneData(
  workflowFilter: string | null | undefined,
  selectedRepositoryIds: string[],
  searchQuery: string,
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
  const orderedWorkflows = useMemo(() => {
    if (workflowFilter) {
      const snapshot = snapshots[workflowFilter];
      if (!snapshot) return [];
      return [{ id: workflowFilter, name: snapshot.workflowName }];
    }
    return workflows.filter((wf) => snapshots[wf.id]);
  }, [workflowFilter, workflows, snapshots]);

  const getFilteredTasks = useCallback(
    (wfId: string) => filterTasks(snapshots, wfId, repoFilter, searchQuery),
    [snapshots, repoFilter, searchQuery],
  );

  return { snapshots, isLoading, orderedWorkflows, getFilteredTasks };
}

function useMobileWorkflowFocus(visibleWorkflows: { id: string }[]) {
  const [requestedWorkflowId, setRequestedWorkflowId] = useState<string | null>(null);
  const requestedWorkflowIsVisible = visibleWorkflows.some(
    (workflow) => workflow.id === requestedWorkflowId,
  );
  const focusedWorkflowId = requestedWorkflowIsVisible
    ? requestedWorkflowId
    : (visibleWorkflows[0]?.id ?? null);

  useEffect(() => {
    if (requestedWorkflowId !== focusedWorkflowId) {
      setRequestedWorkflowId(focusedWorkflowId);
    }
  }, [focusedWorkflowId, requestedWorkflowId]);

  return { focusedWorkflowId, setFocusedWorkflowId: setRequestedWorkflowId };
}

function getVisibleWorkflows(
  workflowFilter: string | null,
  orderedWorkflows: { id: string; name: string }[],
  getFilteredTasks: (workflowId: string) => Task[],
) {
  if (workflowFilter) return orderedWorkflows;
  return orderedWorkflows.filter((workflow) => getFilteredTasks(workflow.id).length > 0);
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
}: WorkflowItemsProps) {
  return workflows.map((workflow, index) => {
    const snapshot = snapshots[workflow.id];
    if (!snapshot) return null;
    return (
      <SortableWorkflowItem
        key={workflow.id}
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
      />
    );
  });
}

export function SwimlaneContainer(containerProps: SwimlaneContainerProps) {
  const { viewMode, workflowFilter, searchQuery, selectedRepositoryIds = [] } = containerProps;
  const { isMobile } = useResponsiveBreakpoint();
  const { isCollapsed, toggleCollapse } = useSwimlaneCollapse();
  const { snapshots, isLoading, orderedWorkflows, getFilteredTasks } = useSwimlaneData(
    workflowFilter,
    selectedRepositoryIds,
    searchQuery ?? "",
  );
  const {
    sensors: workflowSensors,
    canSort: canSortWorkflows,
    handleDragEnd: handleWorkflowDragEnd,
  } = useWorkflowReorder(orderedWorkflows, workflowFilter);

  const visibleWorkflows = getVisibleWorkflows(workflowFilter, orderedWorkflows, getFilteredTasks);
  const { focusedWorkflowId, setFocusedWorkflowId } = useMobileWorkflowFocus(visibleWorkflows);
  const view = getViewByStoredValue(viewMode) ?? getDefaultView();
  const isMobileKanban = isMobile && view.id === "kanban";
  const renderedWorkflows = getRenderedWorkflows(
    isMobileKanban,
    focusedWorkflowId,
    visibleWorkflows,
  );
  const workflowOptions = visibleWorkflows.map((workflow) => ({
    ...workflow,
    taskCount: getFilteredTasks(workflow.id).length,
  }));

  const emptyMessage = getEmptyMessage(
    isLoading,
    snapshots,
    orderedWorkflows,
    workflowFilter,
    getFilteredTasks,
  );
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
          {isMobileKanban && !workflowFilter && workflowOptions.length > 1 && focusedWorkflowId && (
            <MobileWorkflowPicker
              workflows={workflowOptions}
              activeWorkflowId={focusedWorkflowId}
              onWorkflowChange={setFocusedWorkflowId}
            />
          )}
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
          />
        </div>
      </SortableContext>
    </DndContext>
  );
}
