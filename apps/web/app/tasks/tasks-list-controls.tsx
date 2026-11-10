"use client";

import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
import { useTranslation } from "react-i18next";
import {
  GROUP_OPTION_LABEL_KEYS,
  SORT_OPTION_LABEL_KEYS,
  TASKS_LIST_GROUP_OPTIONS,
  TASKS_LIST_SORT_OPTIONS,
} from "@/lib/tasks/tasks-list-options";
import { controlSizingClassName } from "@kandev/ui/control-sizing";
import { TasksListFilters, type TasksListFilterProps } from "./tasks-list-filters";
import { TasksListArchiveModes } from "./tasks-list-archive-modes";
import type { TasksListArchiveMode } from "@/lib/tasks/tasks-list-query";

export function TasksListControls({
  archiveMode,
  onArchiveModeChange,
  filters,
  total = 0,
  isLoading = false,
  tasksListSort,
  onTasksListSortChange,
  tasksListGroup,
  onTasksListGroupChange,
  facetOptions,
}: {
  archiveMode: TasksListArchiveMode;
  onArchiveModeChange: (mode: TasksListArchiveMode) => void;
  filters?: TasksListFilterProps;
  total?: number;
  isLoading?: boolean;
  tasksListSort: string;
  onTasksListSortChange: (sort: string) => void;
  tasksListGroup: string;
  onTasksListGroupChange: (group: string) => void;
  facetOptions?: ReadonlyArray<{ value: string; label: string }>;
}) {
  const { t } = useTranslation();
  const sortOptions = [
    ...TASKS_LIST_SORT_OPTIONS.map((option) => ({
      value: option.value,
      label: t(SORT_OPTION_LABEL_KEYS[option.value]),
    })),
    ...(facetOptions ?? []),
  ];
  const groupOptions = [
    ...TASKS_LIST_GROUP_OPTIONS.map((option) => ({
      value: option.value,
      label: t(GROUP_OPTION_LABEL_KEYS[option.value]),
    })),
    ...(facetOptions ?? []),
  ];
  const displayOptions = (
    <div className="hidden flex-wrap items-center justify-end gap-3 md:flex">
      <ListOptionSelect
        label={t("tasks:sort")}
        value={tasksListSort}
        options={sortOptions}
        onChange={onTasksListSortChange}
        testId="tasks-list-sort"
      />
      <ListOptionSelect
        label={t("tasks:group")}
        value={tasksListGroup}
        options={groupOptions}
        onChange={onTasksListGroupChange}
        testId="tasks-list-group"
      />
    </div>
  );
  return filters ? (
    <TasksListFilters
      {...filters}
      archiveMode={archiveMode}
      onArchiveModeChange={onArchiveModeChange}
      displayOptions={displayOptions}
      total={total}
      isLoading={isLoading}
    />
  ) : (
    <div className="flex flex-wrap justify-between gap-3">
      <TasksListArchiveModes value={archiveMode} onChange={onArchiveModeChange} />
      {displayOptions}
    </div>
  );
}

function ListOptionSelect<T extends string>({
  label,
  value,
  options,
  onChange,
  testId,
}: {
  label: string;
  value: T;
  options: ReadonlyArray<{ readonly value: T; readonly label: string }>;
  onChange: (value: T) => void;
  testId: string;
}) {
  return (
    <div className="flex items-center gap-2">
      <span className="text-sm text-muted-foreground">{label}</span>
      <Select value={value} onValueChange={(next) => onChange(next as T)}>
        <SelectTrigger
          data-testid={testId}
          className={controlSizingClassName("standard", "w-auto min-w-[150px] cursor-pointer")}
        >
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          {options.map((option) => (
            <SelectItem key={option.value} value={option.value} className="cursor-pointer">
              {option.label}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
    </div>
  );
}
