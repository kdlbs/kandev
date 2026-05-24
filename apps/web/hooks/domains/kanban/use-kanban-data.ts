"use client";

import { useMemo, useSyncExternalStore } from "react";
import { useQuery } from "@tanstack/react-query";
import { useAppStore } from "@/components/state-provider";
import { useWorkflows } from "@/hooks/use-workflows";
import { useWorkflowSnapshot } from "@/hooks/use-workflow-snapshot";
import { useUserDisplaySettings } from "@/hooks/use-user-display-settings";
import { filterTasksByRepositories } from "@/lib/kanban/filters";
import { workflowKanbanQueryOptions } from "@/lib/query/query-options/kanban";
import type { WorkflowStep } from "@/components/kanban-column";

type Repository = { id: string; name?: string; local_path?: string };
type TaskWithRepo = { title: string; description?: string; repositoryId?: string };

function filterTasksByQuery<T extends TaskWithRepo>(
  tasks: T[],
  searchQuery: string,
  repositories: Repository[],
): T[] {
  if (!searchQuery) return tasks;
  const query = searchQuery.toLowerCase();
  return tasks.filter((task) => {
    if (task.title.toLowerCase().includes(query)) return true;
    if (task.description?.toLowerCase().includes(query)) return true;
    if (task.repositoryId) {
      const repo = repositories.find((r) => r.id === task.repositoryId);
      if (repo?.name?.toLowerCase().includes(query)) return true;
      if (repo?.local_path?.toLowerCase().includes(query)) return true;
    }
    return false;
  });
}

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
  // Store selectors — Zustand still owns workspaces/workflows/ui state
  const workspaceState = useAppStore((state) => state.workspaces);
  const workflowsState = useAppStore((state) => state.workflows);
  const enablePreviewOnClick = useAppStore((state) => state.userSettings.enablePreviewOnClick);
  const repositoriesByWorkspace = useAppStore((state) => state.repositories.itemsByWorkspaceId);

  // Keep the old snapshot hooks alive during migration so the Zustand kanban
  // slice remains populated for cross-domain consumers not yet migrated.
  useWorkflows(workspaceState.activeId, true);
  useWorkflowSnapshot(workflowsState.activeId);

  // TanStack Query — single-workflow snapshot derived from multi cache
  const { data: snapshot, isLoading: snapshotLoading } = useQuery({
    ...workflowKanbanQueryOptions(workspaceState.activeId ?? "", workflowsState.activeId ?? ""),
    enabled: !!(workspaceState.activeId && workflowsState.activeId),
  });

  // Fallback to Zustand kanban state while TQ warms up (migration period)
  const zustandKanban = useAppStore((state) => state.kanban);
  const kanban = snapshot
    ? {
        workflowId: snapshot.workflowId,
        steps: snapshot.steps,
        tasks: snapshot.tasks,
      }
    : {
        workflowId: zustandKanban.workflowId,
        steps: zustandKanban.steps,
        tasks: zustandKanban.tasks,
      };

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
    () => false,
  );

  // Derived data
  const steps = useMemo<WorkflowStep[]>(
    () =>
      [...kanban.steps]
        .sort((a, b) => (a.position ?? 0) - (b.position ?? 0))
        .map((step) => ({
          id: step.id,
          title: step.title,
          color: step.color || "bg-neutral-400",
          events: step.events,
        })),
    [kanban.steps],
  );

  const tasks = kanban.tasks.map((task) => ({
    id: task.id,
    title: task.title,
    workflowStepId: task.workflowStepId,
    state: task.state,
    description: task.description,
    position: task.position,
    repositoryId: task.repositoryId,
    repositories: task.repositories,
    primarySessionId: task.primarySessionId,
    sessionCount: task.sessionCount,
    reviewStatus: task.reviewStatus,
    parentTaskId: task.parentTaskId,
    createdAt: task.createdAt,
  }));

  const activeSteps = kanban.workflowId ? steps : [];

  const visibleTasks = useMemo(
    () => filterTasksByRepositories(tasks, selectedRepositoryIds),
    [tasks, selectedRepositoryIds],
  );

  const filteredTasks = useMemo(() => {
    const repositories = workspaceState.activeId
      ? (repositoriesByWorkspace[workspaceState.activeId] ?? [])
      : [];
    return filterTasksByQuery(visibleTasks, searchQuery, repositories);
  }, [visibleTasks, searchQuery, workspaceState.activeId, repositoriesByWorkspace]);

  return {
    // State (kanban data now from TQ, rest from Zustand)
    kanban,
    workspaceState,
    workflowsState,
    enablePreviewOnClick,
    userSettings,
    commitSettings,
    selectedRepositoryIds,
    isMounted,
    isLoading: snapshotLoading,

    // Derived data
    activeSteps,
    filteredTasks,
  };
}
