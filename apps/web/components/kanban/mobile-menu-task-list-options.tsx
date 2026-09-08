"use client";

import { Checkbox } from "@kandev/ui/checkbox";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
import {
  GROUP_OPTION_LABEL_KEYS,
  SORT_OPTION_LABEL_KEYS,
  TASKS_LIST_GROUP_OPTIONS,
  TASKS_LIST_SORT_OPTIONS,
} from "@/lib/tasks/tasks-list-options";
import { useTranslation } from "react-i18next";

export type TasksListDisplayOptions = {
  showArchived: boolean;
  onShowArchivedChange: (show: boolean) => void;
  sort: string;
  onSortChange: (sort: string) => void;
  group: string;
  onGroupChange: (group: string) => void;
  facetOptions?: ReadonlyArray<{ value: string; label: string }>;
};

const fieldClass = "space-y-1.5";
const fieldLabelClass = "text-xs font-medium text-muted-foreground";

export function MobileTasksListOptions({ options }: { options: TasksListDisplayOptions }) {
  const { t } = useTranslation();
  return (
    <div className={fieldClass}>
      <label className={fieldLabelClass}>{t("kanban:taskList")}</label>
      <p className="text-xs text-muted-foreground">{t("kanban:theseOptionsAffectThisTaskList")}</p>
      <div className="space-y-3">
        <div className={fieldClass}>
          <label id="mobile-tasks-list-sort-label" className={fieldLabelClass}>
            {t("kanban:sort")}
          </label>
          <Select value={options.sort} onValueChange={options.onSortChange}>
            <SelectTrigger
              id="mobile-tasks-list-sort-select"
              aria-labelledby="mobile-tasks-list-sort-label"
              data-testid="mobile-tasks-list-sort"
              className="h-11 w-full cursor-pointer px-3 text-sm"
            >
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {[
                ...TASKS_LIST_SORT_OPTIONS.map((option) => ({
                  value: option.value,
                  label: t(SORT_OPTION_LABEL_KEYS[option.value]),
                })),
                ...(options.facetOptions ?? []),
              ].map((option) => (
                <SelectItem key={option.value} value={option.value} className="cursor-pointer">
                  {option.label}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
          <p className="text-xs text-muted-foreground">{t("kanban:chooseHowTasksAreOrderedIn")}</p>
        </div>
        <div className={fieldClass}>
          <label id="mobile-tasks-list-group-label" className={fieldLabelClass}>
            {t("kanban:group")}
          </label>
          <Select value={options.group} onValueChange={options.onGroupChange}>
            <SelectTrigger
              id="mobile-tasks-list-group-select"
              aria-labelledby="mobile-tasks-list-group-label"
              data-testid="mobile-tasks-list-group"
              className="h-11 w-full cursor-pointer px-3 text-sm"
            >
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {[
                ...TASKS_LIST_GROUP_OPTIONS.map((option) => ({
                  value: option.value,
                  label: t(GROUP_OPTION_LABEL_KEYS[option.value]),
                })),
                ...(options.facetOptions ?? []),
              ].map((option) => (
                <SelectItem key={option.value} value={option.value} className="cursor-pointer">
                  {option.label}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
          <p className="text-xs text-muted-foreground">
            {t("kanban:groupTasksIntoSectionsByState")}
          </p>
        </div>
        <label className="flex min-h-11 cursor-pointer items-center gap-3 rounded-md px-0 text-sm font-medium">
          <Checkbox
            checked={options.showArchived}
            onCheckedChange={(checked) => options.onShowArchivedChange(checked === true)}
            data-testid="mobile-tasks-list-show-archived"
          />
          <span>{t("kanban:showArchived")}</span>
        </label>
      </div>
    </div>
  );
}
