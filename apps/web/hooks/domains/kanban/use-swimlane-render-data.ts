"use client";

import { useCallback, useMemo, useRef } from "react";
import { useAppStore } from "@/components/state-provider";
import type { Task } from "@/components/kanban-card";
import { mapSelectedRepositoryIds } from "@/lib/kanban/filters";
import { projectWorkflowTasks } from "@/lib/kanban/task-projections";
import {
  selectMobileNavigatorWorkflows,
  selectWorkflowSwimlanes,
} from "@/lib/kanban/workflow-swimlanes";
import type { WorkflowSnapshotData } from "@/lib/state/slices/kanban/types";
import type { Repository } from "@/lib/types/http";

export const EMPTY_HIDDEN_STEP_IDS: string[] = [];

type TaskProjectionCacheEntry = {
  snapshot: WorkflowSnapshotData | undefined;
  hiddenStepIds: string[] | undefined;
  repoFilter: Set<string>;
  searchQuery: string;
  matchesPluginTaskFilters: ((taskId: string) => boolean) | undefined;
  visibleTasks: Task[];
};

function useStableList<T>(items: T[], itemEqual: (previous: T, next: T) => boolean): T[] {
  const previousRef = useRef(items);
  const previous = previousRef.current;
  const isUnchanged =
    previous.length === items.length &&
    previous.every((previousItem, index) => itemEqual(previousItem, items[index]));
  if (!isUnchanged) previousRef.current = items;
  return isUnchanged ? previous : items;
}

export function useStableWorkflowList<T extends { id: string }>(workflows: T[]): T[] {
  return useStableList(workflows, Object.is);
}

export function useStableStringSet(value: Set<string>): Set<string> {
  const previousRef = useRef(value);
  const previous = previousRef.current;
  const isUnchanged =
    previous.size === value.size && Array.from(previous).every((item) => value.has(item));
  if (!isUnchanged) previousRef.current = value;
  return isUnchanged ? previous : value;
}

function useStableWorkflowOptions(
  options: Array<{ id: string; name: string; taskCount: number }>,
): Array<{ id: string; name: string; taskCount: number }> {
  return useStableList(
    options,
    (previous, next) =>
      previous.id === next.id &&
      previous.name === next.name &&
      previous.taskCount === next.taskCount,
  );
}

export function useSwimlaneRenderData(
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
  const workflowIdsWithAutoHideEmptySteps = useAppStore(
    (state) => state.userSettings.workflowIdsWithAutoHideEmptySteps,
  );

  const repositories = useMemo(
    () => Object.values(repositoriesByWorkspace).flat() as Repository[],
    [repositoriesByWorkspace],
  );
  const repoFilter = useMemo(
    () => mapSelectedRepositoryIds(repositories, selectedRepositoryIds),
    [repositories, selectedRepositoryIds],
  );
  const allOrderedWorkflows = useStableWorkflowList(
    useMemo(() => selectWorkflowSwimlanes(null, workflows, snapshots), [workflows, snapshots]),
  );
  const orderedWorkflows = useStableWorkflowList(
    useMemo(
      () => selectWorkflowSwimlanes(workflowFilter, workflows, snapshots),
      [workflowFilter, workflows, snapshots],
    ),
  );

  const projectionCacheRef = useRef(new Map<string, TaskProjectionCacheEntry>());
  const getFilteredTasks = useCallback(
    (workflowId: string) => {
      const snapshot = snapshots[workflowId];
      const hiddenStepIds = hiddenWorkflowStepIds[workflowId];
      const cached = projectionCacheRef.current.get(workflowId);
      if (
        cached?.snapshot === snapshot &&
        cached.hiddenStepIds === hiddenStepIds &&
        cached.repoFilter === repoFilter &&
        cached.searchQuery === searchQuery &&
        cached.matchesPluginTaskFilters === matchesPluginTaskFilters
      ) {
        return cached.visibleTasks;
      }
      const visibleTasks = projectWorkflowTasks(snapshots, workflowId, repoFilter, {
        searchQuery,
        matchesPluginTaskFilters,
        hiddenStepIds: hiddenStepIds?.length ? new Set(hiddenStepIds) : undefined,
      }).visibleTasks;
      projectionCacheRef.current.set(workflowId, {
        snapshot,
        hiddenStepIds,
        repoFilter,
        searchQuery,
        matchesPluginTaskFilters,
        visibleTasks,
      });
      return visibleTasks;
    },
    [hiddenWorkflowStepIds, matchesPluginTaskFilters, repoFilter, searchQuery, snapshots],
  );

  const hasLiveHiddenSteps = useCallback(
    (workflowId: string) => {
      const hidden = hiddenWorkflowStepIds[workflowId];
      if (workflowIdsWithAutoHideEmptySteps.includes(workflowId)) return true;
      if (!hidden?.length) return false;
      const liveStepIds = new Set((snapshots[workflowId]?.steps ?? []).map((step) => step.id));
      return hidden.some((id) => liveStepIds.has(id));
    },
    [hiddenWorkflowStepIds, snapshots, workflowIdsWithAutoHideEmptySteps],
  );

  const workflowOptions = useStableWorkflowOptions(
    useMemo(
      () =>
        selectMobileNavigatorWorkflows(
          allOrderedWorkflows,
          workflows,
          getFilteredTasks,
          hasLiveHiddenSteps,
        ).map(({ workflow, tasks }) => ({
          id: workflow.id,
          name: workflow.name,
          taskCount: tasks.length,
        })),
      [allOrderedWorkflows, getFilteredTasks, hasLiveHiddenSteps, workflows],
    ),
  );

  return {
    snapshots,
    isLoading,
    orderedWorkflows,
    workflowOptions,
    getFilteredTasks,
    hasLiveHiddenSteps,
    repoFilter,
  };
}

export function useWorkflowSwimlaneData(
  workflowId: string,
  repoFilter: Set<string>,
  searchQuery: string,
  matchesPluginTaskFilters?: (taskId: string) => boolean,
): {
  snapshot: WorkflowSnapshotData | undefined;
  tasks: Task[];
  occupancyTasks: Task[];
  hiddenStepIds: string[];
  hiddenSet: Set<string>;
  autoHideEmpty: boolean;
} {
  const snapshot = useAppStore((state) => state.kanbanMulti.snapshots[workflowId]);
  const hiddenStepIds = useAppStore(
    (state) => state.userSettings.hiddenWorkflowStepIds[workflowId] ?? EMPTY_HIDDEN_STEP_IDS,
  );
  const autoHideEmpty = useAppStore((state) =>
    state.userSettings.workflowIdsWithAutoHideEmptySteps.includes(workflowId),
  );
  const derivedHiddenSet = useMemo(() => {
    if (!snapshot || hiddenStepIds.length === 0) return new Set<string>();
    const liveStepIds = new Set(snapshot.steps.map((step) => step.id));
    return new Set(hiddenStepIds.filter((id) => liveStepIds.has(id)));
  }, [hiddenStepIds, snapshot]);
  const hiddenSet = useStableStringSet(derivedHiddenSet);
  const projection = useMemo(() => {
    if (!snapshot) return { visibleTasks: [], occupancyTasks: [] };
    return projectWorkflowTasks({ [workflowId]: snapshot }, workflowId, repoFilter, {
      searchQuery,
      matchesPluginTaskFilters,
      hiddenStepIds: hiddenSet,
    });
  }, [hiddenSet, matchesPluginTaskFilters, repoFilter, searchQuery, snapshot, workflowId]);

  return {
    snapshot,
    tasks: projection.visibleTasks,
    occupancyTasks: projection.occupancyTasks,
    hiddenStepIds,
    hiddenSet,
    autoHideEmpty,
  };
}
