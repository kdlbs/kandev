'use client';

import { useMemo } from 'react';
import { useAppStore } from '@/components/state-provider';
import { useSwimlaneCollapse } from '@/hooks/domains/kanban/use-swimlane-collapse';
import { filterTasksByRepositories, mapSelectedRepositoryIds } from '@/lib/kanban/filters';
import { SwimlaneSection } from './swimlane-section';
import { getViewByStoredValue, getDefaultView } from '@/lib/kanban/view-registry';
import type { Task } from '@/components/kanban-card';
import type { WorkflowAutomation, MoveTaskError } from '@/hooks/use-drag-and-drop';
import type { Repository } from '@/lib/types/http';

export type SwimlaneContainerProps = {
  viewMode: string;
  workflowFilter: string | null;
  onPreviewTask: (task: Task) => void;
  onOpenTask: (task: Task) => void;
  onEditTask: (task: Task) => void;
  onDeleteTask: (task: Task) => void;
  onMoveError?: (error: MoveTaskError) => void;
  onWorkflowAutomation?: (automation: WorkflowAutomation) => void;
  deletingTaskId?: string | null;
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
  if (isLoading && Object.keys(snapshots).length === 0) return 'Loading...';
  if (orderedWorkflows.length === 0) return 'No workflows available yet.';
  const visible = workflowFilter
    ? orderedWorkflows
    : orderedWorkflows.filter((wf) => getFilteredTasks(wf.id).length > 0);
  if (visible.length === 0) return 'No tasks yet';
  return null;
}

export function SwimlaneContainer({
  viewMode,
  workflowFilter,
  onPreviewTask,
  onOpenTask,
  onEditTask,
  onDeleteTask,
  onMoveError,
  onWorkflowAutomation,
  deletingTaskId,
  searchQuery,
  selectedRepositoryIds = [],
}: SwimlaneContainerProps) {
  const snapshots = useAppStore((state) => state.kanbanMulti.snapshots);
  const isLoading = useAppStore((state) => state.kanbanMulti.isLoading);
  const workflows = useAppStore((state) => state.workflows.items);
  const repositoriesByWorkspace = useAppStore((state) => state.repositories.itemsByWorkspaceId);
  const { isCollapsed, toggleCollapse } = useSwimlaneCollapse();

  const repositories = useMemo(
    () => Object.values(repositoriesByWorkspace).flat() as Repository[],
    [repositoriesByWorkspace]
  );

  const repoFilter = useMemo(
    () => mapSelectedRepositoryIds(repositories, selectedRepositoryIds),
    [repositories, selectedRepositoryIds]
  );

  // Build ordered list of workflows to render
  const orderedWorkflows = useMemo(() => {
    if (workflowFilter) {
      // Single workflow mode
      const snapshot = snapshots[workflowFilter];
      if (!snapshot) return [];
      return [{ id: workflowFilter, name: snapshot.workflowName }];
    }
    // All workflows - use workflow order, only show those with snapshots
    return workflows.filter((wf) => snapshots[wf.id]);
  }, [workflowFilter, workflows, snapshots]);

  // Filter tasks per workflow
  const getFilteredTasks = (workflowId: string): Task[] => {
    const snapshot = snapshots[workflowId];
    if (!snapshot) return [];
    let tasks = snapshot.tasks as Task[];

    tasks = filterTasksByRepositories(tasks, repoFilter);

    if (searchQuery) {
      const q = searchQuery.toLowerCase();
      tasks = tasks.filter(
        (t) =>
          t.title.toLowerCase().includes(q) ||
          (t.description && t.description.toLowerCase().includes(q))
      );
    }

    return tasks;
  };

  const emptyMessage = getEmptyMessage(isLoading, snapshots, orderedWorkflows, workflowFilter, getFilteredTasks);
  if (emptyMessage) {
    return (
      <div className="flex-1 min-h-0 px-4 pb-4">
        <div className="h-full rounded-lg border border-dashed border-border/60 flex items-center justify-center text-sm text-muted-foreground">
          {emptyMessage}
        </div>
      </div>
    );
  }

  const visibleWorkflows = workflowFilter
    ? orderedWorkflows
    : orderedWorkflows.filter((wf) => getFilteredTasks(wf.id).length > 0);

  const ViewComponent = (getViewByStoredValue(viewMode) ?? getDefaultView()).component;

  return (
    <div className="flex-1 min-h-0 overflow-y-auto px-4 pb-4 space-y-3">
      {visibleWorkflows.map((wf) => {
        const snapshot = snapshots[wf.id];
        if (!snapshot) return null;
        const tasks = getFilteredTasks(wf.id);
        const steps = [...snapshot.steps].sort((a, b) => a.position - b.position);
        return (
          <SwimlaneSection
            key={wf.id} workflowId={wf.id} workflowName={wf.name} taskCount={tasks.length}
            isCollapsed={isCollapsed(wf.id)} onToggleCollapse={() => toggleCollapse(wf.id)}
          >
            <ViewComponent
              workflowId={wf.id} steps={steps} tasks={tasks} onPreviewTask={onPreviewTask}
              onOpenTask={onOpenTask} onEditTask={onEditTask} onDeleteTask={onDeleteTask}
              onMoveError={onMoveError} onWorkflowAutomation={onWorkflowAutomation} deletingTaskId={deletingTaskId}
            />
          </SwimlaneSection>
        );
      })}
    </div>
  );
}
