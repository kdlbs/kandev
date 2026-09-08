import { useEffect, useMemo, useState } from "react";
import type { Task } from "@/lib/types/http";
import {
  isTaskListFacetOption,
  sortTasksByFacet,
  type TasksListGroup,
  type TasksListSort,
} from "@/lib/tasks/tasks-list-options";
import type { TaskFacetValues } from "./use-task-list-facets";

type TaskListFacetSelectionParams = {
  facetKeys: readonly string[];
  coreSort: TasksListSort;
  coreGroup: TasksListGroup;
  tasks: Task[];
  facetValues: TaskFacetValues;
  onCoreSortChange: (sort: TasksListSort) => void;
  onCoreGroupChange: (group: TasksListGroup) => void;
};

export function useTaskListFacetSelection({
  facetKeys,
  coreSort,
  coreGroup,
  tasks,
  facetValues,
  onCoreSortChange,
  onCoreGroupChange,
}: TaskListFacetSelectionParams) {
  const [facetSort, setFacetSort] = useState<string | null>(null);
  const [facetGroup, setFacetGroup] = useState<string | null>(null);

  useEffect(() => {
    if (facetSort && !facetKeys.includes(facetSort)) setFacetSort(null);
    if (facetGroup && !facetKeys.includes(facetGroup)) setFacetGroup(null);
  }, [facetGroup, facetKeys, facetSort]);

  const displayedTasks = useMemo(
    () => (facetSort ? sortTasksByFacet(tasks, facetSort, facetValues) : tasks),
    [facetSort, facetValues, tasks],
  );
  const selectSort = (value: string) => {
    if (isTaskListFacetOption(value)) {
      setFacetSort(value);
      return;
    }
    setFacetSort(null);
    onCoreSortChange(value as TasksListSort);
  };
  const selectGroup = (value: string) => {
    if (isTaskListFacetOption(value)) {
      setFacetGroup(value);
      return;
    }
    setFacetGroup(null);
    onCoreGroupChange(value as TasksListGroup);
  };

  return {
    displayedTasks,
    sort: facetSort ?? coreSort,
    group: facetGroup ?? coreGroup,
    selectSort,
    selectGroup,
  };
}
