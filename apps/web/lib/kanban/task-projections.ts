import type { Task } from "@/components/kanban-card";
import { filterTasksByRepositories, mapSelectedRepositoryIds } from "@/lib/kanban/filters";
import { taskMatchesPriorityFilter } from "@/lib/kanban/priority-filter-tokens";
import type { TaskPriority } from "@/lib/types/http";

type FilterTasksOptions = {
  searchQuery?: string;
  matchesPluginTaskFilters?: (taskId: string) => boolean;
  hiddenStepIds?: Set<string>;
  /** A view lens like search: excluded from `occupancyTasks` in `projectWorkflowTasks`. */
  priorityFilterTokens?: TaskPriority[];
};

export function filterTasks(
  snapshots: Record<string, { tasks: Task[]; steps: { id: string }[] }>,
  workflowId: string,
  repoFilter: ReturnType<typeof mapSelectedRepositoryIds>,
  options?: FilterTasksOptions,
): Task[] {
  const snapshot = snapshots[workflowId];
  if (!snapshot) return [];
  let tasks = snapshot.tasks;
  const { hiddenStepIds, searchQuery, matchesPluginTaskFilters, priorityFilterTokens } =
    options ?? {};
  if (hiddenStepIds && hiddenStepIds.size > 0) {
    const liveStepIds = new Set(snapshot.steps.map((step) => step.id));
    const effectiveHidden = new Set([...hiddenStepIds].filter((id) => liveStepIds.has(id)));
    if (effectiveHidden.size > 0) {
      tasks = tasks.filter((task) => !effectiveHidden.has(task.workflowStepId));
    }
  }
  tasks = filterTasksByRepositories(tasks, repoFilter);
  if (searchQuery) {
    const query = searchQuery.toLowerCase();
    tasks = tasks.filter(
      (task) =>
        task.title.toLowerCase().includes(query) ||
        (task.description && task.description.toLowerCase().includes(query)),
    );
  }
  if (matchesPluginTaskFilters) {
    tasks = tasks.filter((task) => matchesPluginTaskFilters(task.id));
  }
  if (priorityFilterTokens && priorityFilterTokens.length > 0) {
    tasks = tasks.filter((task) => taskMatchesPriorityFilter(task.priority, priorityFilterTokens));
  }
  return tasks;
}

type WorkflowTaskProjectionOptions = {
  searchQuery: string;
  matchesPluginTaskFilters?: (taskId: string) => boolean;
  hiddenStepIds?: Set<string>;
  priorityFilterTokens?: TaskPriority[];
};

export function projectWorkflowTasks(
  snapshots: Record<string, { tasks: Task[]; steps: { id: string }[] }>,
  workflowId: string,
  repoFilter: ReturnType<typeof mapSelectedRepositoryIds>,
  options: WorkflowTaskProjectionOptions,
): { visibleTasks: Task[]; occupancyTasks: Task[] } {
  return {
    visibleTasks: filterTasks(snapshots, workflowId, repoFilter, options),
    occupancyTasks: filterTasks(snapshots, workflowId, repoFilter, {
      matchesPluginTaskFilters: options.matchesPluginTaskFilters,
    }),
  };
}
