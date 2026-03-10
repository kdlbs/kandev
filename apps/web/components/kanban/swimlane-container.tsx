"use client";

import { type ComponentType, useMemo } from "react";
import { useAppStore } from "@/components/state-provider";
import { useSwimlaneCollapse } from "@/hooks/domains/kanban/use-swimlane-collapse";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import { filterTasksByRepositories, mapSelectedRepositoryIds } from "@/lib/kanban/filters";
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

export type SwimlaneContainerProps = {
  viewMode: string;
  workflowFilter: string | null;
  onPreviewTask: (task: Task) => void;
  onOpenTask: (task: Task) => void;
  onEditTask: (task: Task) => void;
  onDeleteTask: (task: Task) => void;
  onMoveError?: (error: MoveTaskError) => void;
  deletingTaskId?: string | null;
  showMaximizeButton?: boolean;
  searchQuery?: string;
  selectedRepositoryIds?: string[];
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
  isCollapsed: boolean;
  onToggleCollapse: () => void;
  onPreviewTask: (task: Task) => void;
  onOpenTask: (task: Task) => void;
  onEditTask: (task: Task) => void;
  onDeleteTask: (task: Task) => void;
  onMoveError?: (error: MoveTaskError) => void;
  deletingTaskId?: string | null;
  showMaximizeButton?: boolean;
};

function WorkflowItem({
  wf,
  snapshot,
  tasks,
  ViewComponent,
  hideHeader,
  isCollapsed,
  onToggleCollapse,
  ...viewProps
}: WorkflowItemProps) {
  const steps = [...snapshot.steps].sort((a, b) => a.position - b.position);
  const content = <ViewComponent workflowId={wf.id} steps={steps} tasks={tasks} {...viewProps} />;

  if (hideHeader) return <div key={wf.id}>{content}</div>;

  return (
    <SwimlaneSection
      key={wf.id}
      workflowId={wf.id}
      workflowName={wf.name}
      taskCount={tasks.length}
      isCollapsed={isCollapsed}
      onToggleCollapse={onToggleCollapse}
    >
      {content}
    </SwimlaneSection>
  );
}

export function SwimlaneContainer({
  viewMode,
  workflowFilter,
  onPreviewTask,
  onOpenTask,
  onEditTask,
  onDeleteTask,
  onMoveError,
  deletingTaskId,
  showMaximizeButton,
  searchQuery,
  selectedRepositoryIds = [],
}: SwimlaneContainerProps) {
  const { isMobile } = useResponsiveBreakpoint();
  const snapshots = useAppStore((state) => state.kanbanMulti.snapshots);
  const isLoading = useAppStore((state) => state.kanbanMulti.isLoading);
  const workflows = useAppStore((state) => state.workflows.items);
  const repositoriesByWorkspace = useAppStore((state) => state.repositories.itemsByWorkspaceId);
  const { isCollapsed, toggleCollapse } = useSwimlaneCollapse();

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

  const getFilteredTasks = (wfId: string) => filterTasks(snapshots, wfId, repoFilter, searchQuery);

  const emptyMessage = getEmptyMessage(
    isLoading,
    snapshots,
    orderedWorkflows,
    workflowFilter,
    getFilteredTasks,
  );
  if (emptyMessage) return renderEmptyState(emptyMessage);

  const visibleWorkflows = workflowFilter
    ? orderedWorkflows
    : orderedWorkflows.filter((wf) => getFilteredTasks(wf.id).length > 0);

  const ViewComponent = (getViewByStoredValue(viewMode) ?? getDefaultView()).component;
  const hideHeaders = isMobile && (workflowFilter !== null || orderedWorkflows.length === 1);
  const containerClass = isMobile
    ? "flex-1 min-h-0 overflow-y-auto pb-4 space-y-3"
    : "flex-1 min-h-0 overflow-y-auto px-4 pb-4 space-y-3";

  return (
    <div className={containerClass} data-testid="swimlane-container">
      {visibleWorkflows.map((wf) => {
        const snapshot = snapshots[wf.id];
        if (!snapshot) return null;
        return (
          <WorkflowItem
            key={wf.id}
            wf={wf}
            snapshot={snapshot}
            tasks={getFilteredTasks(wf.id)}
            ViewComponent={ViewComponent}
            hideHeader={hideHeaders}
            isCollapsed={isCollapsed(wf.id)}
            onToggleCollapse={() => toggleCollapse(wf.id)}
            onPreviewTask={onPreviewTask}
            onOpenTask={onOpenTask}
            onEditTask={onEditTask}
            onDeleteTask={onDeleteTask}
            onMoveError={onMoveError}
            deletingTaskId={deletingTaskId}
            showMaximizeButton={showMaximizeButton}
          />
        );
      })}
    </div>
  );
}
