"use client";

import { IconFilter } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Checkbox } from "@kandev/ui/checkbox";
import { Popover, PopoverContent, PopoverTrigger } from "@kandev/ui/popover";
import { Separator } from "@kandev/ui/separator";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import type {
  IssueFilterState,
  OrchestrateIssueStatus,
  OrchestrateIssuePriority,
} from "@/lib/state/slices/orchestrate/types";
import { StatusIcon } from "./status-icon";

const STATUSES: { value: OrchestrateIssueStatus; label: string }[] = [
  { value: "backlog", label: "Backlog" },
  { value: "todo", label: "Todo" },
  { value: "in_progress", label: "In Progress" },
  { value: "in_review", label: "In Review" },
  { value: "blocked", label: "Blocked" },
  { value: "done", label: "Done" },
  { value: "cancelled", label: "Cancelled" },
];

const PRIORITIES: { value: OrchestrateIssuePriority; label: string }[] = [
  { value: "critical", label: "Critical" },
  { value: "high", label: "High" },
  { value: "medium", label: "Medium" },
  { value: "low", label: "Low" },
  { value: "none", label: "None" },
];

type IssueFiltersProps = {
  filters: IssueFilterState;
  onFilterChange: (filters: Partial<IssueFilterState>) => void;
};

function toggleInArray<T>(arr: T[], value: T): T[] {
  return arr.includes(value) ? arr.filter((v) => v !== value) : [...arr, value];
}

export function IssueFilters({ filters, onFilterChange }: IssueFiltersProps) {
  const activeCount =
    filters.statuses.length + filters.priorities.length +
    filters.assigneeIds.length + filters.projectIds.length;

  return (
    <Popover>
      <Tooltip>
        <TooltipTrigger asChild>
          <PopoverTrigger asChild>
            <Button variant={activeCount > 0 ? "secondary" : "ghost"} size="icon-sm" className="cursor-pointer">
              <IconFilter className="h-4 w-4" />
              {activeCount > 0 && (
                <span className="absolute -top-1 -right-1 h-4 w-4 rounded-full bg-primary text-primary-foreground text-[10px] flex items-center justify-center">
                  {activeCount}
                </span>
              )}
            </Button>
          </PopoverTrigger>
        </TooltipTrigger>
        <TooltipContent>Filter</TooltipContent>
      </Tooltip>
      <PopoverContent className="w-56 p-3" align="end">
        <p className="text-xs font-medium mb-2">Status</p>
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
        <p className="text-xs font-medium mb-2">Priority</p>
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
              Clear filters
            </Button>
          </>
        )}
      </PopoverContent>
    </Popover>
  );
}
