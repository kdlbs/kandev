"use client";

import { Checkbox } from "@kandev/ui/checkbox";
import { Label } from "@kandev/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
import { useTranslation } from "react-i18next";
import {
  GROUP_OPTION_LABEL_KEYS,
  SORT_OPTION_LABEL_KEYS,
  TASKS_LIST_GROUP_OPTIONS,
  TASKS_LIST_SORT_OPTIONS,
} from "@/lib/tasks/tasks-list-options";

export function TasksListControls({
  showArchived,
  onShowArchivedChange,
  tasksListSort,
  onTasksListSortChange,
  tasksListGroup,
  onTasksListGroupChange,
  facetOptions,
}: {
  showArchived: boolean;
  onShowArchivedChange: (show: boolean) => void;
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
  return (
    <div className="hidden min-h-9 flex-wrap items-center justify-end gap-3 sm:flex">
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
      <Label className="flex h-11 items-center gap-2 text-sm text-muted-foreground cursor-pointer select-none lg:h-9">
        <Checkbox
          checked={showArchived}
          onCheckedChange={(checked) => onShowArchivedChange(checked === true)}
          className="cursor-pointer"
        />
        {t("tasks:showArchived")}
      </Label>
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
        <SelectTrigger data-testid={testId} className="h-10 w-[150px] cursor-pointer lg:h-9">
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
