"use client";

import { Checkbox } from "@kandev/ui/checkbox";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
import {
  TASKS_LIST_GROUP_OPTIONS,
  TASKS_LIST_SORT_OPTIONS,
  type TasksListGroup,
  type TasksListSort,
} from "@/lib/tasks/tasks-list-options";

export type TasksListDisplayOptions = {
  showArchived: boolean;
  onShowArchivedChange: (show: boolean) => void;
  sort: TasksListSort;
  onSortChange: (sort: TasksListSort) => void;
  group: TasksListGroup;
  onGroupChange: (group: TasksListGroup) => void;
};

const fieldClass = "space-y-1.5";
const fieldLabelClass = "text-xs font-medium text-muted-foreground";

export function MobileTasksListOptions({ options }: { options: TasksListDisplayOptions }) {
  return (
    <div className={fieldClass}>
      <label className={fieldLabelClass}>Task list</label>
      <p className="text-xs text-muted-foreground">These options affect this task list only.</p>
      <div className="space-y-3">
        <div className={fieldClass}>
          <label id="mobile-tasks-list-sort-label" className={fieldLabelClass}>
            Sort
          </label>
          <Select
            value={options.sort}
            onValueChange={(value) => options.onSortChange(value as TasksListSort)}
          >
            <SelectTrigger
              id="mobile-tasks-list-sort-select"
              aria-labelledby="mobile-tasks-list-sort-label"
              data-testid="mobile-tasks-list-sort"
              className="h-11 w-full cursor-pointer px-3 text-sm"
            >
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {TASKS_LIST_SORT_OPTIONS.map((option) => (
                <SelectItem key={option.value} value={option.value} className="cursor-pointer">
                  {option.label}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
          <p className="text-xs text-muted-foreground">
            Choose how tasks are ordered in this list.
          </p>
        </div>
        <div className={fieldClass}>
          <label id="mobile-tasks-list-group-label" className={fieldLabelClass}>
            Group
          </label>
          <Select
            value={options.group}
            onValueChange={(value) => options.onGroupChange(value as TasksListGroup)}
          >
            <SelectTrigger
              id="mobile-tasks-list-group-select"
              aria-labelledby="mobile-tasks-list-group-label"
              data-testid="mobile-tasks-list-group"
              className="h-11 w-full cursor-pointer px-3 text-sm"
            >
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {TASKS_LIST_GROUP_OPTIONS.map((option) => (
                <SelectItem key={option.value} value={option.value} className="cursor-pointer">
                  {option.label}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
          <p className="text-xs text-muted-foreground">
            Group tasks into sections by state, workflow, repository, or none.
          </p>
        </div>
        <label className="flex min-h-11 cursor-pointer items-center gap-3 rounded-md px-0 text-sm font-medium">
          <Checkbox
            checked={options.showArchived}
            onCheckedChange={(checked) => options.onShowArchivedChange(checked === true)}
            data-testid="mobile-tasks-list-show-archived"
          />
          <span>Show archived</span>
        </label>
      </div>
    </div>
  );
}
