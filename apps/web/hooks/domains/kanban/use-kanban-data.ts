'use client';

import { useMemo, useState, useSyncExternalStore } from 'react';
import { useAppStore } from '@/components/state-provider';
import { useWorkflows } from '@/hooks/use-workflows';
import { useWorkflowSnapshot } from '@/hooks/use-workflow-snapshot';
import { useUserDisplaySettings } from '@/hooks/use-user-display-settings';
import { filterTasksByRepositories } from '@/lib/kanban/filters';
import type { WorkflowStep } from '@/components/kanban-column';

type KanbanDataOptions = {
  onWorkspaceChange: (workspaceId: string | null) => void;
  onWorkflowChange: (workflowId: string | null) => void;
  searchQuery?: string;
};

export function useKanbanData({ onWorkspaceChange, onWorkflowChange, searchQuery = '' }: KanbanDataOptions) {
  const [taskSessionAvailability, setTaskSessionAvailability] = useState<Record<string, boolean>>({});

  // Store selectors
  const kanban = useAppStore((state) => state.kanban);
  const workspaceState = useAppStore((state) => state.workspaces);
  const workflowsState = useAppStore((state) => state.workflows);
  const enablePreviewOnClick = useAppStore((state) => state.userSettings.enablePreviewOnClick);
  const repositoriesByWorkspace = useAppStore((state) => state.repositories.itemsByWorkspaceId);

  // Data fetching hooks
  useWorkflows(workspaceState.activeId, true);
  useWorkflowSnapshot(workflowsState.activeId);

  // User settings hook
  const {
    settings: userSettings,
    commitSettings,
    selectedRepositoryIds,
  } = useUserDisplaySettings({
    workspaceId: workspaceState.activeId,
    workflowId: workflowsState.activeId,
    onWorkspaceChange,
    onWorkflowChange,
  });

  // SSR safety check
  const isMounted = useSyncExternalStore(
    () => () => {},
    () => true,
    () => false
  );

  // Derived data
  const steps = useMemo<WorkflowStep[]>(
    () =>
      [...kanban.steps]
        .sort((a, b) => (a.position ?? 0) - (b.position ?? 0))
        .map((step) => ({
          id: step.id,
          title: step.title,
          color: step.color || 'bg-neutral-400',
          events: step.events,
        })),
    [kanban.steps]
  );

  const tasks = kanban.tasks.map((task: typeof kanban.tasks[number]) => ({
    id: task.id,
    title: task.title,
    workflowStepId: task.workflowStepId,
    state: task.state,
    description: task.description,
    position: task.position,
    repositoryId: task.repositoryId,
    primarySessionId: task.primarySessionId,
    sessionCount: task.sessionCount,
    reviewStatus: task.reviewStatus,
  }));

  const activeSteps = kanban.workflowId ? steps : [];

  const visibleTasks = useMemo(
    () => filterTasksByRepositories(tasks, selectedRepositoryIds),
    [tasks, selectedRepositoryIds]
  );

  // Apply search filtering
  const filteredTasks = useMemo(() => {
    if (!searchQuery) return visibleTasks;

    // Get repositories for the current workspace for search filtering
    const repositories = workspaceState.activeId
      ? repositoriesByWorkspace[workspaceState.activeId] ?? []
      : [];

    const query = searchQuery.toLowerCase();
    return visibleTasks.filter((task) => {
      // Match task title or description
      if (task.title.toLowerCase().includes(query)) return true;
      if (task.description?.toLowerCase().includes(query)) return true;

      // Match repository name/path
      if (task.repositoryId) {
        const repo = repositories.find((r) => r.id === task.repositoryId);
        if (repo?.name?.toLowerCase().includes(query)) return true;
        if (repo?.local_path?.toLowerCase().includes(query)) return true;
      }

      return false;
    });
  }, [visibleTasks, searchQuery, workspaceState.activeId, repositoriesByWorkspace]);

  const visibleTasksWithSessions = useMemo(
    () =>
      filteredTasks.map((task) => ({
        ...task,
        hasSession: taskSessionAvailability[task.id],
      })),
    [filteredTasks, taskSessionAvailability]
  );

  return {
    // State
    kanban,
    workspaceState,
    workflowsState,
    enablePreviewOnClick,
    userSettings,
    commitSettings,
    selectedRepositoryIds,
    taskSessionAvailability,
    setTaskSessionAvailability,
    isMounted,

    // Derived data
    activeSteps,
    visibleTasksWithSessions,
  };
}
