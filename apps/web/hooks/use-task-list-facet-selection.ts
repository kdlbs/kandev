import { useMemo } from "react";
import type { Task } from "@/lib/types/http";
import {
  DEFAULT_TASKS_LIST_GROUP,
  DEFAULT_TASKS_LIST_SORT,
  isTaskListFacetOption,
  parseTasksListGroup,
  parseTasksListSort,
  sortTasksByFacet,
  type TasksListGroupPreference,
  type TasksListSortPreference,
} from "@/lib/tasks/tasks-list-options";
import type { TaskFacetValues } from "./use-task-list-facets";

type TaskListFacetSelectionParams = {
  facetKeys: readonly string[];
  requestedSort: TasksListSortPreference;
  requestedGroup: TasksListGroupPreference;
  tasks: Task[];
  facetValues: TaskFacetValues;
  onSortChange: (sort: TasksListSortPreference) => void;
  onGroupChange: (group: TasksListGroupPreference) => void;
};

export function useTaskListFacetSelection({
  facetKeys,
  requestedSort,
  requestedGroup,
  tasks,
  facetValues,
  onSortChange,
  onGroupChange,
}: TaskListFacetSelectionParams) {
  // The requested value is what the query and the stored preference keep; the
  // effective value falls back to the built-in default while the facet is not
  // registered (disabled, uninstalled, or still loading) without writing back.
  const facetSortKey =
    isTaskListFacetOption(requestedSort) && facetKeys.includes(requestedSort)
      ? requestedSort
      : null;
  const facetGroupKey =
    isTaskListFacetOption(requestedGroup) && facetKeys.includes(requestedGroup)
      ? requestedGroup
      : null;
  const sort =
    isTaskListFacetOption(requestedSort) && !facetSortKey ? DEFAULT_TASKS_LIST_SORT : requestedSort;
  const group =
    isTaskListFacetOption(requestedGroup) && !facetGroupKey
      ? DEFAULT_TASKS_LIST_GROUP
      : requestedGroup;
  const displayedTasks = useMemo(
    () => (facetSortKey ? sortTasksByFacet(tasks, facetSortKey, facetValues) : tasks),
    [facetSortKey, facetValues, tasks],
  );

  return {
    displayedTasks,
    sort,
    group,
    facetSortKey,
    facetGroupKey,
    selectSort: (value: string) => onSortChange(parseTasksListSort(value)),
    selectGroup: (value: string) => onGroupChange(parseTasksListGroup(value)),
  };
}
