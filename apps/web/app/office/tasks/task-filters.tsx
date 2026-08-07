"use client";

import { IconFilter } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Checkbox } from "@kandev/ui/checkbox";
import { Popover, PopoverContent, PopoverTrigger } from "@kandev/ui/popover";
import { Separator } from "@kandev/ui/separator";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { useAppStore } from "@/components/state-provider";
import type {
  TaskFilterState,
  OfficeTaskStatus,
  OfficeTaskPriority,
} from "@/lib/state/slices/office/types";
import { StatusIcon } from "./status-icon";
import { PRIORITY_LABEL_KEYS, STATUS_LABEL_KEYS } from "../lib/label-keys";
import { useTranslation } from "react-i18next";

// `labelKey`, not `label` — module scope, so a `t()` here would freeze at the
// boot locale; the component resolves at render. The `value`s are the wire
// status/priority ids and stay untranslated.
const FALLBACK_STATUSES: { value: OfficeTaskStatus; labelKey: string }[] = [
  { value: "backlog", labelKey: STATUS_LABEL_KEYS.backlog },
  { value: "todo", labelKey: STATUS_LABEL_KEYS.todo },
  { value: "in_progress", labelKey: STATUS_LABEL_KEYS.in_progress },
  { value: "in_review", labelKey: STATUS_LABEL_KEYS.in_review },
  { value: "blocked", labelKey: STATUS_LABEL_KEYS.blocked },
  { value: "done", labelKey: STATUS_LABEL_KEYS.done },
  { value: "cancelled", labelKey: STATUS_LABEL_KEYS.cancelled },
];

const FALLBACK_PRIORITIES: { value: OfficeTaskPriority; labelKey: string }[] = (
  ["critical", "high", "medium", "low", "none"] as const
).map((value) => ({ value, labelKey: PRIORITY_LABEL_KEYS[value] }));

type IssueFiltersProps = {
  filters: TaskFilterState;
  onFilterChange: (filters: Partial<TaskFilterState>) => void;
};

function toggleInArray<T>(arr: T[], value: T): T[] {
  return arr.includes(value) ? arr.filter((v) => v !== value) : [...arr, value];
}

export function TaskFilters({ filters, onFilterChange }: IssueFiltersProps) {
  const { t } = useTranslation();
  const meta = useAppStore((s) => s.office.meta);
  const STATUSES = meta
    ? meta.statuses.map((s) => ({ value: s.id as OfficeTaskStatus, label: s.label }))
    : FALLBACK_STATUSES.map((s) => ({ value: s.value, label: t(s.labelKey) }));
  const PRIORITIES = meta
    ? meta.priorities.map((p) => ({ value: p.id as OfficeTaskPriority, label: p.label }))
    : FALLBACK_PRIORITIES.map((p) => ({ value: p.value, label: t(p.labelKey) }));

  const activeCount =
    filters.statuses.length +
    filters.priorities.length +
    filters.assigneeIds.length +
    filters.projectIds.length;

  return (
    <Popover>
      <Tooltip>
        <TooltipTrigger asChild>
          <PopoverTrigger asChild>
            <Button
              variant={activeCount > 0 ? "secondary" : "ghost"}
              size="icon-sm"
              className="cursor-pointer"
            >
              <IconFilter className="h-4 w-4" />
              {activeCount > 0 && (
                <span className="absolute -top-1 -right-1 h-4 w-4 rounded-full bg-primary text-primary-foreground text-[10px] flex items-center justify-center">
                  {activeCount}
                </span>
              )}
            </Button>
          </PopoverTrigger>
        </TooltipTrigger>
        <TooltipContent>{t("office:filter")}</TooltipContent>
      </Tooltip>
      <PopoverContent className="w-56 p-3" align="end">
        <p className="text-xs font-medium mb-2">{t("common:status")}</p>
        <div className="flex flex-col gap-1.5">
          {STATUSES.map((s) => (
            <label key={s.value} className="flex items-center gap-2 text-sm cursor-pointer">
              <Checkbox
                checked={filters.statuses.includes(s.value)}
                onCheckedChange={() =>
                  onFilterChange({ statuses: toggleInArray(filters.statuses, s.value) })
                }
                className="cursor-pointer"
              />
              <StatusIcon status={s.value} className="h-3.5 w-3.5" />
              {s.label}
            </label>
          ))}
        </div>
        <Separator className="my-2" />
        <p className="text-xs font-medium mb-2">{t("office:priority")}</p>
        <div className="flex flex-col gap-1.5">
          {PRIORITIES.map((p) => (
            <label key={p.value} className="flex items-center gap-2 text-sm cursor-pointer">
              <Checkbox
                checked={filters.priorities.includes(p.value)}
                onCheckedChange={() =>
                  onFilterChange({ priorities: toggleInArray(filters.priorities, p.value) })
                }
                className="cursor-pointer"
              />
              {p.label}
            </label>
          ))}
        </div>
        {activeCount > 0 && (
          <>
            <Separator className="my-2" />
            <Button
              variant="ghost"
              size="sm"
              className="w-full cursor-pointer"
              onClick={() =>
                onFilterChange({ statuses: [], priorities: [], assigneeIds: [], projectIds: [] })
              }
            >
              {t("office:clearFilters")}
            </Button>
          </>
        )}
      </PopoverContent>
    </Popover>
  );
}
