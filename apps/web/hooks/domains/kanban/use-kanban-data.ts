"use client";

import { useMemo, useSyncExternalStore } from "react";
import { useAppStore } from "@/components/state-provider";
import { useWorkflows } from "@/hooks/use-workflows";
import { useWorkflowSnapshot } from "@/hooks/use-workflow-snapshot";
import { useUserDisplaySettings } from "@/hooks/use-user-display-settings";
import { filterTasksByRepositories } from "@/lib/kanban/filters";
import type { WorkflowStep } from "@/components/kanban-column";
import type { KanbanState } from "@/lib/state/slices";

type KanbanDataOptions = {
  onWorkspaceChange: (workspaceId: string | null) => void;
  onWorkflowChange: (workflowId: string | null) => void;
  searchQuery?: string;
};

export function useKanbanData({
  onWorkspaceChange,
  onWorkflowChange,
  searchQuery = "",
}: KanbanDataOptions) {
  // Store selectors
  const workspaceState = useAppStore((state) => state.workspaces);
  const storeWorkflowsState = useAppStore((state) => state.workflows);

  // Data fetching hooks
  const workflowsQuery = useWorkflows(workspaceState.activeId, true);
  const snapshotQuery = useWorkflowSnapshot(storeWorkflowsState.activeId);

  // User settings hook
  const {
    settings: userSettings,
    commitSettings,
    repositories,
    selectedRepositoryIds,
  } = useUserDisplaySettings({
    workspaceId: workspaceState.activeId,
    workflowId: storeWorkflowsState.activeId,
    onWorkspaceChange,
    onWorkflowChange,
  });
  const enablePreviewOnClick = userSettings.enablePreviewOnClick;
  const boardState = useMemo<KanbanState>(
    () =>
      snapshotQuery.snapshotState ?? {
        workflowId: storeWorkflowsState.activeId,
        steps: [],
        tasks: [],
        isLoading: Boolean(storeWorkflowsState.activeId && snapshotQuery.isFetching),
      },
    [snapshotQuery.isFetching, snapshotQuery.snapshotState, storeWorkflowsState.activeId],
  );
  const workflowsState = useMemo(
    () => ({ ...storeWorkflowsState, items: workflowsQuery.workflows }),
    [storeWorkflowsState, workflowsQuery.workflows],
  );

  // SSR safety check
  const isMounted = useSyncExternalStore(
    () => () => {},
    () => true,
    () => false,
  );

  // Derived data
  const steps = useMemo<WorkflowStep[]>(
    () =>
      [...boardState.steps]
        .sort((a, b) => (a.position ?? 0) - (b.position ?? 0))
        .map((step) => ({
          id: step.id,
          title: step.title,
          color: step.color || "bg-neutral-400",
          events: step.events,
        })),
    [boardState.steps],
  );

  // Memoized so a fresh array of card objects isn't produced on every store
  // change. Without this, `visibleTasks`/`filteredTasks` (which depend on
  // `tasks`) and every downstream card re-render on any unrelated store update,
  // pegging the CPU on large boards.
  const tasks = useMemo(
    () =>
      boardState.tasks.map((task) => ({
        id: task.id,
        title: task.title,
        workflowStepId: task.workflowStepId,
        state: task.state,
        description: task.description,
        position: task.position,
        repositoryId: task.repositoryId,
        repositories: task.repositories,
        primarySessionId: task.primarySessionId,
      })),
    [boardState.tasks],
  );

  const activeSteps = boardState.workflowId ? steps : [];

  const visibleTasks = useMemo(
    () => filterTasksByRepositories(tasks, selectedRepositoryIds),
    [tasks, selectedRepositoryIds],
  );

  // Apply search filtering
  const filteredTasks = useMemo(() => {
    if (!searchQuery) return visibleTasks;

    // Get repositories for the current workspace for search filtering
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
  }, [visibleTasks, searchQuery, repositories]);

  return {
    // State
    boardState,
    workspaceState,
    workflowsState,
    enablePreviewOnClick,
    userSettings,
    commitSettings,
    selectedRepositoryIds,
    isMounted,

    // Derived data
    activeSteps,
    filteredTasks,
  };
}
